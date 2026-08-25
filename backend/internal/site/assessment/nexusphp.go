package assessment

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// NexusPHPAdapter NexusPHP 站考核：GET userdetails.php → HTML 归一 + 跨行聚合。
type NexusPHPAdapter struct{}

// Platform 返回平台标识。
func (NexusPHPAdapter) Platform() string { return "nexusphp" }

// Supported 默认支持。
func (NexusPHPAdapter) Supported() bool { return true }

// FetchAssessments 抓取并解析考核任务。uid 用于拼 userdetails.php?id=<uid>——
// 裸访 userdetails.php（无 id）会被站点视为无效参数（52movie 警告根因），故 uid 空时不抓。
func (NexusPHPAdapter) FetchAssessments(ctx context.Context, hc HTTPDoer, def Def, baseURL, token, uid string) ([]Task, error) {
	detail := def.UserInfo.DetailPath
	if detail == "" {
		detail = "userdetails.php"
	}
	if uid == "" {
		return nil, nil // chain 层负责提供 uid（聚合直传 / 独立路径回退 UserInfo）
	}
	resp, err := hc.Get(ctx, joinBase(baseURL, detail+"?id="+uid), token, true)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("nexusphp assessment: empty response")
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("nexusphp assessment: status %d", resp.StatusCode)
	}
	// cookie 失效：userdetails.php 被重定向到登录页（FollowRedirect 已跟随成 200+HTML），
	// 靠 body 关键词识别；响应过短也视为异常（正常 userdetails 页远大于 500 字节）。
	bodyLow := strings.ToLower(string(resp.Body))
	if strings.Contains(bodyLow, "takelogin") || strings.Contains(bodyLow, "login.php") ||
		strings.Contains(bodyLow, "请登录") || len(resp.Body) < 500 {
		return nil, site.ErrNotLoggedIn
	}
	// 喂原始 HTML 而非 doc.Text()：normalizeHTML 靠块级闭合标签（</div>/</tr>）切行，
	// doc.Text() 已剥标签且部分皮肤源码单行压缩（憨憨考核块整块一行），指标会粘连错位。
	return parseNexusPHPAssessment(string(resp.Body)), nil
}

var (
	// normalizeHTML：把块级标签（含每个 </td>）转为换行，再剥剩余标签。
	htmlBlockRe = regexp.MustCompile(`(?is)<(?:script|style|noscript)\b[^>]*>.*?</(?:script|style|noscript)>`)
	htmlBrRe    = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlCloseRe = regexp.MustCompile(`(?i)</?(?:p|div|li|tr|td|th|h\d)[^>]*>`)
	htmlTagRe   = regexp.MustCompile(`<[^>]+>`)
	// header：名称：X 时间：YYYY-MM-DD HH:MM ~ YYYY-MM-DD HH:MM
	headerRe = regexp.MustCompile(`名称[：:]\s*(.+?)\s+时间[：:]\s*(\d{4}[./-]\d{1,2}[./-]\d{1,2}\s+\d{1,2}:\d{2}(?::\d{2})?)\s*[~～\-]\s*(\d{4}[./-]\d{1,2}[./-]\d{1,2}\s+\d{1,2}:\d{2}(?::\d{2})?)`)
	// 憨憨式 header：标题行「新手考核:」+ 独立行「考核时间：YYYY-MM-DD HH:MM:SS ~ …」（无名称段）
	headerTimeRe = regexp.MustCompile(`考核时间[：:]\s*(\d{4}[./-]\d{1,2}[./-]\d{1,2}\s+\d{1,2}:\d{2}(?::\d{2})?)\s*[~～\-]\s*(\d{4}[./-]\d{1,2}[./-]\d{1,2}\s+\d{1,2}:\d{2}(?::\d{2})?)`)
	// 单行完整指标：指标N：名称, 要求:X, 当前:Y, 结果:Z
	metricRe = regexp.MustCompile(`指标\s*(\d+)[：:]\s*([^,，]+?)[,，]\s*要求[：:]\s*(.+?)[,，]\s*当前[：:]\s*(.+?)[,，]\s*结果[：:]\s*(.+)`)
	// 憨憨式单行指标：考核指标N：名称, 要求:X, 当前:Y, 结果:Z（前缀多个「考核」二字）
	metricHhanRe = regexp.MustCompile(`(?:考核)?指标\s*(\d+)[：:]\s*(.+?)\s*[,，]\s*要求[：:]\s*(.+?)\s*[,，]\s*当前[：:]\s*(.+?)\s*[,，]\s*结果[：:]\s*(.+)`)
	// 指标行起始（拆行回退用）
	metricStartRe = regexp.MustCompile(`(?:考核)?指标\s*(\d+)[：:]\s*(.*)`)
	requireRe     = regexp.MustCompile(`要求[：:]\s*(.+)`)
	currentRe     = regexp.MustCompile(`当前[：:]\s*(.+)`)
	resultRe      = regexp.MustCompile(`结果[：:]\s*(.+)`)
	// 考核信号
	assessSignalRe = regexp.MustCompile(`(?i)(考核|新手任务|任务进度|assessment|examine)`)
	// 结果完成关键词（须先排除「未达标/未完成」等否定前缀）
	completedResultRe = regexp.MustCompile(`完成|通过|达标|✓|✔|√`)
	failedResultRe    = regexp.MustCompile(`未达标|未完成|未通过|不达标|失败|未达`)
	// metric 黑名单（噪声行）
	metricBlacklistRe = regexp.MustCompile(`火花|佩戴|过期时间|优先级|gntv|加成系数`)
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
		// C：憨憨式——独立「考核时间：A ~ B」行，名称取其前最近的短标题行（如「新手考核:」）
		for i, ln := range merged {
			m := headerTimeRe.FindStringSubmatch(ln)
			if len(m) < 3 {
				continue
			}
			startStr, endStr = m[1], m[2]
			headerIdx = i
			for j := i - 1; j >= 0 && i-j <= 3; j-- {
				t := strings.TrimSpace(strings.Trim(merged[j], "：: "))
				if t != "" && !strings.Contains(merged[j], "时间") && len([]rune(t)) <= 20 {
					name = t
					break
				}
			}
			break
		}
	}
	if headerIdx < 0 {
		return nil
	}
	if name == "" {
		name = "新手考核"
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
