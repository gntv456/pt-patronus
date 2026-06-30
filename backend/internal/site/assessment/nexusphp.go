package assessment

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// NexusPHPAdapter NexusPHP 站考核：GET userdetails.php → HTML 归一 + 跨行聚合。
type NexusPHPAdapter struct{}

// Platform 返回平台标识。
func (NexusPHPAdapter) Platform() string { return "nexusphp" }

// Supported 默认支持。
func (NexusPHPAdapter) Supported() bool { return true }

// FetchAssessments 抓取并解析考核任务。
func (NexusPHPAdapter) FetchAssessments(ctx context.Context, hc HTTPDoer, def Def, baseURL, token string) ([]Task, error) {
	detail := def.UserInfo.DetailPath
	if detail == "" {
		detail = "userdetails.php"
	}
	resp, err := hc.Get(ctx, joinBase(baseURL, detail), token, true)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || len(resp.Body) < 500 {
		return nil, nil
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, nil
	}
	return parseNexusPHPAssessment(doc.Text()), nil
}

var (
	// normalizeHTML：把块级标签（含每个 </td>）转为换行，再剥剩余标签。
	htmlBlockRe = regexp.MustCompile(`(?is)<(?:script|style|noscript)\b[^>]*>.*?</(?:script|style|noscript)>`)
	htmlBrRe     = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlCloseRe  = regexp.MustCompile(`(?i)</?(?:p|div|li|tr|td|th|h\d)[^>]*>`)
	htmlTagRe    = regexp.MustCompile(`<[^>]+>`)
	// header：名称：X 时间：YYYY-MM-DD HH:MM ~ YYYY-MM-DD HH:MM
	headerRe = regexp.MustCompile(`名称[：:]\s*(.+?)\s+时间[：:]\s*(\d{4}[./-]\d{1,2}[./-]\d{1,2}\s+\d{1,2}:\d{2}(?::\d{2})?)\s*[~～\-]\s*(\d{4}[./-]\d{1,2}[./-]\d{1,2}\s+\d{1,2}:\d{2}(?::\d{2})?)`)
	// 单行完整指标：指标N：名称, 要求:X, 当前:Y, 结果:Z
	metricRe = regexp.MustCompile(`指标\s*(\d+)[：:]\s*([^,，]+?)[,，]\s*要求[：:]\s*(.+?)[,，]\s*当前[：:]\s*(.+?)[,，]\s*结果[：:]\s*(.+)`)
	// 指标行起始（拆行回退用）
	metricStartRe = regexp.MustCompile(`指标\s*(\d+)[：:]\s*(.*)`)
	requireRe     = regexp.MustCompile(`要求[：:]\s*(.+)`)
	currentRe     = regexp.MustCompile(`当前[：:]\s*(.+)`)
	resultRe      = regexp.MustCompile(`结果[：:]\s*(.+)`)
	// 指标字段（用于 line-by-line 回退按名抽取）
	metricNameRe = regexp.MustCompile(`指标\s*\d+[：:]\s*([^,，]+)`)
	// 考核信号
	assessSignalRe = regexp.MustCompile(`(?i)(考核|新手任务|任务进度|assessment|examine)`)
	// 结果完成关键词（须先排除「未达标/未完成」等否定前缀）
	completedResultRe = regexp.MustCompile(`完成|通过|达标|✓|✔|√`)
	failedResultRe    = regexp.MustCompile(`未达标|未完成|未通过|不达标|失败|未达`)
	// metric 黑名单（噪声行）
	metricBlacklistRe = regexp.MustCompile(`火花|佩戴|过期时间|优先级|gntv|加成系数`)
	// 剩余天数
	remainingDaysRe = regexp.MustCompile(`(?:考核.{0,12}?还有|剩余)\s*(\d{1,4})\s*天`)
)

// parseNexusPHPAssessment 从 userdetails 纯文本解析考核任务。
func parseNexusPHPAssessment(text string) []Task {
	if !assessSignalRe.MatchString(text) {
		return nil
	}
	lines := normalizeHTML(text)
	merged := mergeAssessmentLines(lines)

	// header 定位（A：直接行匹配；B：邻行组合）
	headerIdx := -1
	var name, startStr, endStr string
	for i, ln := range merged {
		if m := headerRe.FindStringSubmatch(ln); len(m) >= 4 {
			name, startStr, endStr = strings.TrimSpace(m[1]), m[2], m[3]
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		// B：含「名称」短行 + 后续 5 行组合
		for i, ln := range merged {
			if !strings.Contains(ln, "名称") || len([]rune(ln)) > 80 {
				continue
			}
			window := ln
			for j := 1; j <= 5 && i+j < len(merged); j++ {
				window += " " + merged[i+j]
			}
			if m := headerRe.FindStringSubmatch(window); len(m) >= 4 {
				name, startStr, endStr = strings.TrimSpace(m[1]), m[2], m[3]
				headerIdx = i
				break
			}
		}
	}
	if headerIdx < 0 {
		return nil
	}
	if name == "" {
		name = "绩效考核"
	}

	// 指标解析（Phase 1：merged 全行；Phase 2：原始逐行回退）
	metrics := parseMetricsMerged(merged[headerIdx:])
	if len(metrics) == 0 {
		metrics = parseMetricsLineByLine(lines)
	}
	if len(metrics) == 0 {
		return nil
	}

	// 状态
	allMet := true
	for _, m := range metrics {
		if !m.IsCompleted {
			allMet = false
			break
		}
	}
	status := StatusActive
	endMs := parseDateTimeMs(endStr)
	switch {
	case allMet:
		status = StatusCompleted
	case endMs == 0:
		status = StatusUnknown // endTime 不可解析，不伪造
	case time.Now().UnixMilli() > endMs:
		status = StatusFailed
	}
	startMs := parseDateTimeMs(startStr)

	return []Task{{
		Name:      name,
		StartTime: startMs,
		EndTime:   endMs,
		Metrics:   metrics,
		Status:    status,
		FetchedAt: time.Now(),
	}}
}

// normalizeHTML 把块级标签转换行 + 剥标签 + 折叠空行。
func normalizeHTML(html string) []string {
	s := htmlBlockRe.ReplaceAllString(html, "")
	s = htmlBrRe.ReplaceAllString(s, "\n")
	s = htmlCloseRe.ReplaceAllString(s, "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, " ", " ")
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(regexp.MustCompile(`[ \t　]+`).ReplaceAllString(ln, " "))
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

// mergeAssessmentLines 把「指标N：名称,」行与其后续 要求/当前/结果 续行合并。
func mergeAssessmentLines(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		if isMetricStartLine(ln) {
			merged := ln
			for j := i + 1; j < len(lines) && isMetricContLine(lines[j]); j++ {
				merged += " " + lines[j]
				i = j
			}
			out = append(out, merged)
		} else {
			out = append(out, ln)
		}
	}
	return out
}

func isMetricStartLine(ln string) bool {
	return strings.Contains(ln, "指标") && regexp.MustCompile(`指标\s*\d+[：:]`).MatchString(ln) &&
		!strings.Contains(ln, "要求") && !strings.Contains(ln, "结果")
}

func isMetricContLine(ln string) bool {
	if isMetricStartLine(ln) {
		return false
	}
	if strings.HasPrefix(ln, "要求") || strings.HasPrefix(ln, "当前") || strings.HasPrefix(ln, "结果") {
		return true
	}
	if (strings.Contains(ln, "要求") || strings.Contains(ln, "当前") || strings.Contains(ln, "结果")) && len([]rune(ln)) < 60 {
		return true
	}
	return false
}

// parseMetricsMerged Phase 1：在已合并的行上跑完整指标正则。
func parseMetricsMerged(lines []string) []Metric {
	var out []Metric
	for _, ln := range lines {
		if metricBlacklistRe.MatchString(ln) {
			continue
		}
		if !strings.Contains(ln, "指标") || !strings.Contains(ln, "要求") ||
			!strings.Contains(ln, "当前") || !strings.Contains(ln, "结果") {
			continue
		}
		m := metricRe.FindStringSubmatch(ln)
		if len(m) < 6 {
			continue
		}
		name := strings.TrimRight(strings.TrimSpace(m[2]), ",，")
		if name == "" {
			continue
		}
		out = append(out, Metric{
			Label:       name,
			Required:    normMetricValue(m[3], name),
			Current:     normMetricValue(m[4], name),
			Unit:        inferUnit(name),
			IsCompleted: isResultCompleted(m[5]),
		})
	}
	return out
}

// parseMetricsLineByLine Phase 2：逐指标起始行 + 前看 10 行拼字段。
func parseMetricsLineByLine(lines []string) []Metric {
	var out []Metric
	for i := 0; i < len(lines); i++ {
		if !metricStartRe.MatchString(lines[i]) {
			continue
		}
		mStart := metricStartRe.FindStringSubmatch(lines[i])
		name := strings.TrimRight(strings.TrimSpace(mStart[2]), ",，")
		var req, cur, res string
		for j := i + 1; j < len(lines) && j <= i+10; j++ {
			ln := lines[j]
			if metricStartRe.MatchString(ln) {
				break
			}
			if metricBlacklistRe.MatchString(ln) {
				continue
			}
			if req == "" {
				if m := requireRe.FindStringSubmatch(ln); len(m) > 1 {
					req = strings.TrimRight(strings.TrimSpace(m[1]), ",，")
					continue
				}
			}
			if cur == "" {
				if m := currentRe.FindStringSubmatch(ln); len(m) > 1 {
					cur = strings.TrimRight(strings.TrimSpace(m[1]), ",，")
					continue
				}
			}
			if res == "" {
				if m := resultRe.FindStringSubmatch(ln); len(m) > 1 {
					res = strings.TrimSpace(m[1])
				}
			}
		}
		if name == "" || (req == "" && cur == "") {
			continue
		}
		out = append(out, Metric{
			Label: name, Required: normMetricValue(req, name), Current: normMetricValue(cur, name),
			Unit: inferUnit(name), IsCompleted: isResultCompleted(res),
		})
	}
	return out
}

// isResultCompleted 结果串是否达标：先排除「未达标/未完成」否定前缀，再匹配正向关键词。
func isResultCompleted(s string) bool {
	if failedResultRe.MatchString(s) {
		return false
	}
	return completedResultRe.MatchString(s)
}

// normMetricValue 把值串归一（size→GB、time→小时），单位按 metric 名推断。
func normMetricValue(s, name string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	m := regexp.MustCompile(`([\d.]+)\s*([A-Za-z]*)`).FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	num := atofSimple(m[1])
	unit := strings.ToUpper(m[2])
	// 有 size 单位 → GB
	switch unit {
	case "B":
		return num / 1073741824
	case "KB", "KIB":
		return num / 1048576
	case "MB", "MIB":
		return num / 1024
	case "GB", "GIB":
		return num
	case "TB", "TIB":
		return num * 1024
	case "PB", "PIB":
		return num * 1048576
	}
	// 有 time 单位 → 小时
	switch unit {
	case "S", "SEC":
		return num / 3600
	case "MIN":
		return num / 60
	case "H", "HOUR":
		return num
	case "D", "DAY":
		return num * 24
	case "W", "WEEK":
		return num * 168
	}
	// 无单位 → 原值（分享率/计数/积分等）
	return num
}

// inferUnit 按指标名推断默认展示单位。
func inferUnit(name string) string {
	switch {
	case strings.Contains(name, "上传") || strings.Contains(name, "下载"):
		return "GB"
	case strings.Contains(name, "分享") || strings.Contains(name, "比率"):
		return ""
	case strings.Contains(name, "做种时间") || strings.Contains(name, "时长"):
		return "小时"
	case strings.Contains(name, "做种数"):
		return "个"
	}
	return ""
}

// parseDateTimeMs 解析 "2026-01-01 12:00" 为 epoch ms（失败返回 0，不伪造）。
func parseDateTimeMs(s string) int64 {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006.01.02 15:04:05", "2006.01.02 15:04"} {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(s), time.Local); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func joinBase(base, path string) string {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + "/" + path
}

func atofSimple(s string) float64 {
	f := 0.0
	neg := false
	seenDot := false
	div := 1.0
	for i, r := range s {
		switch {
		case r == '-' && i == 0:
			neg = true
		case r >= '0' && r <= '9':
			if seenDot {
				div *= 10
				f += float64(r-'0') / div
			} else {
				f = f*10 + float64(r-'0')
			}
		case r == '.' && !seenDot:
			seenDot = true
		}
	}
	if neg {
		return -f
	}
	return f
}
