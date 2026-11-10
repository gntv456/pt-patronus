// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// IsLoggedOut 按多策略（OR）判定响应是否处于「未登录」态。
// 用于检测 cookie 失效 / 被重定向到登录页 / 触发 WAF 登录墙。
func IsLoggedOut(statusCode int, finalURL string, doc *goquery.Document, def NoLoginDef) bool {
	for _, sc := range def.HttpStatus {
		if statusCode == sc {
			return true
		}
	}
	lowerURL := strings.ToLower(finalURL)
	for _, p := range def.URLPattern {
		if p != "" && strings.Contains(lowerURL, strings.ToLower(p)) {
			return true
		}
	}
	if doc != nil && def.Selector != "" {
		if doc.Find(def.Selector).Length() > 0 {
			return true
		}
	}
	if doc != nil && len(def.Content) > 0 {
		// 只匹配 <title>，不扫整页 doc.Text()：已登录页导航/统计常含「登录」字样，
		// 整页匹配会误判未登录（财神 index 就此中招 → 公告/抓取恒空）。
		// 登录页 <title> 含「登录」/「Login」，已登录页 <title> 是站名，按 title 精确判定。
		title := strings.ToLower(doc.Find("title").First().Text())
		for _, c := range def.Content {
			if c != "" && strings.Contains(title, strings.ToLower(c)) {
				return true
			}
		}
	}
	return false
}
