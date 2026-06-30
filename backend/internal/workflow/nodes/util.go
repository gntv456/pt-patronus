package nodes

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hxpt/ptpatronus/internal/workflow"
)

// matchResolution 种子标题是否含期望分辨率（大小写不敏感）。
// 复制自 chain.matchResolution（订阅分辨率过滤同款逻辑），避免 import chain 造成循环。
func matchResolution(title, want string) bool {
	if want == "" {
		return true
	}
	w := strings.ToUpper(strings.TrimSpace(want))
	t := strings.ToUpper(title)
	if strings.Contains(t, w) {
		return true
	}
	num := strings.TrimRight(strings.TrimRight(w, "PI"), " ")
	switch {
	case strings.HasPrefix(w, "2160") && strings.Contains(t, "4K"):
		return true
	case strings.HasPrefix(w, "4K") && strings.Contains(t, "2160"):
		return true
	case num != "" && num != w && strings.Contains(t, num):
		return true
	}
	return false
}

// matchAny 标题是否命中 | 分隔的任一关键词（大小写不敏感，空规则=放行）。
func matchAny(title, rules string) bool {
	if strings.TrimSpace(rules) == "" {
		return true
	}
	t := strings.ToLower(title)
	for _, kw := range strings.Split(rules, "|") {
		if k := strings.TrimSpace(kw); k != "" && strings.Contains(t, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// matchNone 标题是否命中 | 分隔的任一关键词（命中即排除；空规则=不排除）。
func matchNone(title, rules string) bool {
	if strings.TrimSpace(rules) == "" {
		return true
	}
	t := strings.ToLower(title)
	for _, kw := range strings.Split(rules, "|") {
		if k := strings.TrimSpace(kw); k != "" && strings.Contains(t, strings.ToLower(k)) {
			return false
		}
	}
	return true
}

// tmplRe 模板占位 {{...}}。
var tmplRe = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// render 渲染模板：{{var.NAME}}→变量、{{torrents.N.field}}→种子字段、{{count}}→资源数、{{content}}→文本载荷。
func render(s string, ecx *workflow.Context) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	return tmplRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(m, "{{"), "}}"))
		switch {
		case strings.HasPrefix(inner, "var."):
			k := strings.TrimPrefix(inner, "var.")
			if ecx.Variables != nil {
				if v, ok := ecx.Variables[k]; ok {
					return fmt.Sprintf("%v", v)
				}
			}
			return ""
		case inner == "count":
			return fmt.Sprintf("%d", len(ecx.Torrents))
		case inner == "content":
			return ecx.Content
		case strings.HasPrefix(inner, "torrents."):
			// torrents.N.field
			rest := strings.TrimPrefix(inner, "torrents.")
			parts := strings.SplitN(rest, ".", 2)
			if len(parts) != 2 {
				return ""
			}
			idx := 0
			fmt.Sscanf(parts[0], "%d", &idx)
			if idx < 0 || idx >= len(ecx.Torrents) {
				return ""
			}
			t := ecx.Torrents[idx]
			switch parts[1] {
			case "title":
				return t.Title
			case "size":
				return t.Size
			case "site", "site_name":
				return t.SiteName
			case "url", "download_url":
				return t.DownloadURL
			case "detail_url":
				return t.DetailURL
			}
			return ""
		}
		return m
	})
}
