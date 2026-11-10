// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ParseFreeleech 纯函数：从首页文档解析「全站免费/促销」横幅（仿原版 SiteFreeleechService.parseNotices）。
//
// 候选节点：a.msgalert_text / td[style*=background] / font[color=white]。
// 命中 looksLikeFreeleech 的节点判 Active/Upcoming，其余忽略。distinct 去重（站点+状态+正文）。
// 返回的 SiteFreeleech 仅含 Status/Title/Content/Link/Color；SiteID/SiteName/FetchedAt 由调用方填。
func ParseFreeleech(doc *goquery.Document) []SiteFreeleech {
	if doc == nil {
		return nil
	}
	now := time.Now()
	var out []SiteFreeleech
	seen := map[string]bool{}

	add := func(status FreeleechStatus, content, link, color string) {
		key := string(status) + "|" + content
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, SiteFreeleech{
			Status:    status,
			Title:     FreeleechTitle(status),
			Content:   content,
			Link:      link,
			Color:     color,
			FetchedAt: now,
		})
	}

	for _, sel := range []string{"a.msgalert_text", "td[style*=background]", "font[color=white]"} {
		doc.Find(sel).Each(func(_ int, node *goquery.Selection) {
			text := normalizeSpace(node.Text())
			if text == "" || !looksLikeFreeleech(text) {
				return
			}
			color := findInlineBackground(node)
			status := FreeleechActive
			if isUpcoming(text, color) {
				status = FreeleechUpcoming
			}
			link, exists := node.Attr("href")
			if !exists || link == "" {
				link = "torrents.php"
			}
			add(status, text, link, color)
		})
	}
	return out
}

// FreeleechTitle 返回某状态的中文标题（供适配器构造兜底通知用）。
func FreeleechTitle(status FreeleechStatus) string {
	switch status {
	case FreeleechUpcoming:
		return "即将站免"
	case FreeleechActive:
		return "正在站免"
	case FreeleechNone:
		return "暂无站免"
	case FreeleechFailed:
		return "读取失败"
	}
	return ""
}

// looksLikeFreeleech：正文须同时含「促销类关键词」与「免费类关键词」（原版双列表 AND 逻辑）。
func looksLikeFreeleech(text string) bool {
	l := strings.ToLower(text)
	promo := []string{"全站", "站免", "促销", "promotion", "full site", "free", "2x", "免费", "两倍", "上传"}
	free := []string{"free", "免费", "站免", "促销", "promotion", "全站"}
	hasPromo, hasFree := false, false
	for _, k := range promo {
		if strings.Contains(l, k) {
			hasPromo = true
			break
		}
	}
	for _, k := range free {
		if strings.Contains(l, k) {
			hasFree = true
			break
		}
	}
	return hasPromo && hasFree
}

// isUpcoming：文本含「即将/upcoming/将于」或背景为蓝色（#00f/#0000ff）。
func isUpcoming(text, color string) bool {
	l := strings.ToLower(text)
	cl := strings.ToLower(color)
	switch {
	case strings.Contains(l, "即将"), strings.Contains(l, "upcoming"), strings.Contains(l, "将于"):
		return true
	case strings.Contains(cl, "blue"), strings.Contains(cl, "#00f"), strings.Contains(cl, "#0000ff"):
		return true
	}
	return false
}

var bgColorRe = regexp.MustCompile(`(?i)background(?:-color)?\s*:\s*([^;'"` + "`" + `]+)`)

// findInlineBackground：向上最多 4 层祖先读取 style 的 background[-color]，取首个非空。
func findInlineBackground(node *goquery.Selection) string {
	cur := node
	for i := 0; i < 4 && cur.Length() > 0; i++ {
		style, _ := cur.Attr("style")
		if m := bgColorRe.FindStringSubmatch(style); len(m) > 1 {
			if bg := strings.TrimSpace(m[1]); bg != "" {
				return bg
			}
		}
		cur = cur.Parent()
	}
	return ""
}

func normalizeSpace(s string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

var spaceRe = regexp.MustCompile(`\s+`)
