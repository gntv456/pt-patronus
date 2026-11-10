// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"net/url"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// matchSiteByHost 把 IYUU 站点 host 匹配到本地 Site（按 base_url host 精确，回退后缀）。
func matchSiteByHost(sites []models.Site, iyuuHost string) *models.Site {
	want := hostOf(iyuuHost)
	if want == "" {
		return nil
	}
	for i := range sites {
		if hostOf(sites[i].BaseURL) == want {
			return &sites[i]
		}
	}
	// 回退：子域差异（如 IYUU 下载域 vs 本地配置域）后缀包含。
	for i := range sites {
		h := hostOf(sites[i].BaseURL)
		if h != "" && sameOrSubdomain(h, want) {
			return &sites[i]
		}
	}
	return nil
}

func sameOrSubdomain(a, b string) bool {
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

// hostOf 从（可能含 scheme 的）URL 取小写 host，规整端口和常见 www 前缀。
func hostOf(rawURL string) string {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	if u, err := url.Parse(s); err == nil {
		return normalizeHost(u.Hostname())
	}
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s[i+1:], "]") {
		s = s[:i]
	}
	return normalizeHost(s)
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "."))
	return strings.TrimPrefix(host, "www.")
}
