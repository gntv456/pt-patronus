package assessment

import (
	"regexp"
	"strings"
	"time"
)

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
