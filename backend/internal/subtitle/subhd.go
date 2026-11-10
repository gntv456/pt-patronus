// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package subtitle

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// subhdProvider — SubHD（subhd.tv）网页抓取。goquery 解析搜索/详情页。
//
// TODO 联调：选择器基于社区逆向，subhd 有反爬（可能需 cookie/CF）；下载链路需详情页
// 取下载 token。单测用 mock HTML 覆盖搜索解析；真实 cookie + 反爬联调留用户。
type subhdProvider struct {
	cookie string // 可选 anti-bot cookie
	hc     HTTPDoer
	base   string
}

func NewSubHD(cookie string, hc HTTPDoer) *subhdProvider {
	return &subhdProvider{cookie: cookie, hc: hc, base: "https://subhd.tv"}
}

func (p *subhdProvider) Name() string { return "subhd" }

func (p *subhdProvider) Search(ctx context.Context, query string, opts SearchOpts) ([]Subtitle, error) {
	u := fmt.Sprintf("%s/search/%s", p.base, url.PathEscape(query))
	resp, err := p.hc.Do(ctx, "GET", u, nil, nil, p.cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subhd search: HTTP %d", resp.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("subhd parse: %w", err)
	}
	seen := map[string]bool{}
	var out []Subtitle
	doc.Find(`a[href*="/d/"]`).Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		id := extractSubhdID(href)
		if id == "" || seen[id] {
			return
		}
		title := strings.TrimSpace(s.Text())
		if title == "" {
			return
		}
		seen[id] = true
		lang := guessLangFromTitle(title)
		out = append(out, Subtitle{
			Source: "subhd", SourceID: id, Title: title,
			Language: lang, LangCode: normalizeLang(lang),
			FileName: title, Ext: guessExt(title),
			Extra: href,
		})
	})
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

// extractSubhdID 从 /d/abc123 取 id 段。
func extractSubhdID(href string) string {
	i := strings.Index(href, "/d/")
	if i < 0 {
		return ""
	}
	rest := href[i+3:]
	if j := strings.IndexAny(rest, "?#"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

func (p *subhdProvider) Download(ctx context.Context, sub Subtitle) ([]byte, string, error) {
	// TODO 联调：SubHD 下载需详情页的下载 token（反爬），真实流程：
	// 1) GET /d/{id} 取下载表单 token；2) GET/POST 下载端点拿 zip。
	// 此处实现 best-effort：直接 GET /d/{id}.d（部分时期的端点），失败则返回明确错误。
	u := fmt.Sprintf("%s/d/%s.d", p.base, url.PathEscape(sub.SourceID))
	resp, err := p.hc.Do(ctx, "GET", u, nil, nil, p.cookie, true)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("subhd download: HTTP %d（可能需 cookie/下载 token 联调）", resp.StatusCode)
	}
	name := sub.FileName
	if name == "" {
		name = sub.SourceID
	}
	return resp.Body, name, nil
}

// guessLangFromTitle 从标题文本猜语言（简/繁/英/双语）。
func guessLangFromTitle(title string) string {
	t := strings.ToLower(title)
	switch {
	case strings.Contains(t, "双语") || strings.Contains(t, "bilingual") ||
		(strings.Contains(t, "中") && (strings.Contains(t, "英") || strings.Contains(t, "eng"))):
		return "中英双语"
	case strings.Contains(t, "繁") || strings.Contains(t, "cht") || strings.Contains(t, "big5"):
		return "繁體中文"
	case strings.Contains(t, "英") || strings.Contains(t, "eng"):
		return "英文"
	}
	return "简体中文"
}

var _ Provider = (*subhdProvider)(nil)
