// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func Extract(ctx *goquery.Selection, q ElementQuery) string {
	if ctx == nil || ctx.Length() == 0 {
		if q.Text != "" {
			return ApplyFilters(q.Text, q.Filters)
		}
		return ""
	}
	// Remove 在整个上下文上预先移除（与历史行为一致：广告/图标在 Find 前清掉）
	if q.Remove != "" {
		ctx.Find(q.Remove).Remove()
	}
	sels := q.Selectors
	if len(sels) == 0 && q.Selector != "" {
		sels = []string{q.Selector}
	}
	if len(sels) == 0 {
		if q.Text != "" && q.Data == "" {
			return ApplyFilters(q.Text, q.Filters)
		}
		// 无选择器：仅当带过滤链时才在整上下文上取值（"任意位置正则抽取"模式）；
		// 否则无可提取，返回空——避免无配置字段误返整页文本（详情页未映射字段 bug）。
		if len(q.Filters) == 0 {
			return ""
		}
		return readNode(ctx, q)
	}
	for _, sel := range sels {
		sub := findCompat(ctx, sel)
		if sel == ":self" {
			sub = ctx
		}
		if sub.Length() == 0 {
			continue
		}
		switch q.Attr {
		case "count":
			return ApplyFilters(strconv.Itoa(sub.Length()), q.Filters)
		case "text_all":
			return ApplyFilters(sub.Text(), q.Filters)
		case "html_all":
			html, _ := sub.Html()
			return ApplyFilters(html, q.Filters)
		}
		// 多匹配时只取首个（如多个 font.color_bonus，首个才是真魔力/积分）
		if got := readNode(scopeNode(sub.First(), q), q); got != "" {
			return got
		}
	}
	if q.Text != "" {
		return ApplyFilters(q.Text, q.Filters)
	}
	return ""
}

func scopeNode(s *goquery.Selection, q ElementQuery) *goquery.Selection {
	if s == nil || s.Length() == 0 {
		return s
	}
	if q.Ancestor != "" {
		scoped := s.Closest(q.Ancestor)
		if scoped.Length() == 0 {
			return scoped
		}
		s = scoped
	}
	if q.Find != "" {
		return s.Find(q.Find).First()
	}
	return s
}

// readNode 在单个命中节点（已 First）上按 Parent/Attr/Filters 取值。

func readNode(s *goquery.Selection, q ElementQuery) string {
	if s == nil || s.Length() == 0 {
		return ""
	}
	if q.Parent {
		s = s.Parent()
		if s.Length() == 0 {
			return ""
		}
	}
	var raw string
	if q.Data != "" {
		v, ok := s.Attr(dataAttrName(q.Data))
		if !ok {
			return ""
		}
		return ApplyFilters(v, q.Filters)
	}
	switch q.Attr {
	case "", "text":
		raw = s.Text()
	case "html":
		raw, _ = s.Html()
	case "title_or_text":
		raw = firstAttrOrText(s, "title")
	case "datetime_or_title_or_text":
		raw = firstAttrOrText(s, "datetime", "title")
	default:
		v, ok := s.Attr(q.Attr)
		if !ok {
			return ""
		}
		raw = v
	}
	return ApplyFilters(raw, q.Filters)
}

func firstAttrOrText(s *goquery.Selection, attrs ...string) string {
	for _, attr := range attrs {
		if v, ok := s.Attr(attr); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return s.Text()
}

func dataAttrName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "data-") {
		return name
	}
	var b strings.Builder
	b.WriteString("data-")
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		if r == '_' {
			b.WriteByte('-')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ApplyFilters 顺序应用过滤链。
