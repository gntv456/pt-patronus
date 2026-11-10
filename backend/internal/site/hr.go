// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ParseHR 纯函数：按 def.HR 选择器从文档解析 H&R 列表（仿 ParseSearch，可单测）。
func ParseHR(doc *goquery.Document, def *SiteMetadata, baseURL string) []HRTorrent {
	if doc == nil || def == nil {
		return nil
	}
	f := def.HR.Fields
	if len(f) == 0 {
		return nil
	}
	base, _ := url.Parse(baseURL)
	rows := doc.Find(def.HR.Rows.Selector)
	out := []HRTorrent{}
	rows.Each(func(_ int, row *goquery.Selection) {
		t := HRTorrent{
			Title:        Extract(row, f["title"]),
			Size:         Extract(row, f["size"]),
			SeedTime:     Extract(row, f["seedtime"]),
			Status:       Extract(row, f["status"]),
			DeadlineLeft: Extract(row, f["deadline"]),
			Ratio:        ToFloat64(Extract(row, f["ratio"])),
			TorrentID:    Extract(row, f["id"]),
		}
		t.DetailURL = ResolveURL(base, Extract(row, f["detail"]))
		// 跳过表头/空行：无标题且无状态视为非数据行
		if t.Title == "" && t.Status == "" {
			return
		}
		out = append(out, t)
	})
	return out
}

// ── 时长解析 ──

var (
	// 前导天数：2天 / 2day(s) / 2 days / 2d
	hrDayRe = regexp.MustCompile(`(?i)(\d+)\s*(?:天|day\(?s?\)?|days|d)\b`)
	// 中文零散：3小时 / 20分(钟) / 5秒
	hrCNHourRe = regexp.MustCompile(`(\d+)\s*小时`)
	hrCNMinRe  = regexp.MustCompile(`(\d+)\s*分(?:钟)?`)
	hrCNSecRe  = regexp.MustCompile(`(\d+)\s*秒`)
	// 英文零散：3h / 20m / 5s
	hrENHourRe = regexp.MustCompile(`(?i)(\d+)\s*h(?:ou)?r?s?`)
	hrENMinRe  = regexp.MustCompile(`(?i)(\d+)\s*m(?:in)?(?:ute)?s?`)
	hrENSecRe  = regexp.MustCompile(`(?i)(\d+)\s*s(?:ec)?(?:ond)?s?`)
	// 冒号时段：3:04:05 / 4:05（mkprettytime 的纯时间部分）
	colonTimeRe = regexp.MustCompile(`\d{1,2}(?::\d{1,2}){1,2}`)
)

// ParseHRDuration 把 NexusPHP mkprettytime 风格的剩余时间文本解析为时长。
// 支持形态（源码 include/functions.php mkprettytime）：
//   - "2天03:04:05" / "2day(s)03:04:05"  → 天数 + HH:MM:SS
//   - "3:04:05"                         → HH:MM:SS
//   - "4:05"                            → MM:SS
//   - "2天3小时20分" / "3h 20m 5s"        → 零散中/英文
//
// 解析失败返回 0（视为无可靠剩余时间）。
func ParseHRDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" || s == "---" {
		return 0
	}
	s = strings.ReplaceAll(s, "：", ":") // 全角冒号归一

	var d time.Duration
	// 1) 前导天数
	if m := hrDayRe.FindStringSubmatch(s); len(m) >= 2 {
		if days, err := strconv.Atoi(m[1]); err == nil {
			d += time.Duration(days) * 24 * time.Hour
		}
		s = hrDayRe.ReplaceAllString(s, " ")
	}
	// 2) 冒号时段 H:M:S / M:S（取首个匹配，从右向左为 秒/分/时）
	if m := colonTimeRe.FindString(s); m != "" {
		d += parseColonTime(m)
		s = colonTimeRe.ReplaceAllString(s, " ")
	}
	// 3) 零散中英文
	d += atoiDur(hrCNHourRe, s) * time.Hour
	d += atoiDur(hrENHourRe, s) * time.Hour
	d += atoiDur(hrCNMinRe, s) * time.Minute
	d += atoiDur(hrENMinRe, s) * time.Minute
	d += atoiDur(hrCNSecRe, s) * time.Second
	d += atoiDur(hrENSecRe, s) * time.Second
	return d
}

func atoiDur(re *regexp.Regexp, s string) time.Duration {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return time.Duration(n)
}

func parseColonTime(m string) time.Duration {
	parts := strings.Split(m, ":")
	secs := 0
	mul := []int{1, 60, 3600} // 从右往左：秒、分、时
	for i := 0; i < len(parts) && i < len(mul); i++ {
		n, err := strconv.Atoi(parts[len(parts)-1-i])
		if err != nil {
			continue
		}
		secs += n * mul[i]
	}
	return time.Duration(secs) * time.Second
}

// ── 危险判定 ──

// IsHRDangerous 判定一条 H&R 种子是否有被封号风险（需告警）。
//   - 状态命中「达标/豁免」关键词 → 安全
//   - 状态命中「未达标/失败/超时」关键词 → 危险（已失败，惩罚将至）
//   - 其余（监控中/未知）：剩余考核时间 < warnDeadline → 危险（临期）
//
// warnDeadline 为 0 时不按时间告警（仅按失败状态告警）。
func IsHRDangerous(t HRTorrent, warnDeadline time.Duration) bool {
	st := strings.ToLower(strings.TrimSpace(t.Status))
	if st != "" {
		switch {
		case containsAny(st, "未达标", "unreached", "失败", "fail", "超时", "过期", "warn"):
			return true
		case containsAny(st, "达标", "reached", "完成", "豁免", "pardon", "ignored", "ignore"):
			return false
		}
	}
	if warnDeadline <= 0 {
		return false
	}
	left := ParseHRDuration(t.DeadlineLeft)
	if left <= 0 {
		return false // 无可靠剩余时间 → 不误报
	}
	return left < warnDeadline
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
