// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import "testing"

// TestRegistryMatchByURL 验证按 host 匹配站点定义（URLs + LegacyURLs），
// 用于 .ptd 恢复时自动挂正确定义。
func TestRegistryMatchByURL(t *testing.T) {
	r := NewRegistry()
	r.AddDefinition(&SiteMetadata{ID: "zhuque", Schema: "tnode", URLs: []string{"https://zhuque.in/"}})
	r.AddDefinition(&SiteMetadata{ID: "hxpt", Schema: "nexusphp",
		URLs: []string{"https://hxpt.org"}, LegacyURLs: []string{"https://www.hxpt.org/"}})
	r.AddDefinition(&SiteMetadata{ID: "mteam", Schema: "mteam",
		URLs: []string{"https://kp.m-team.cc", "https://m-team.cc"}})

	cases := []struct {
		url  string
		want string
		ok   bool
	}{
		{"https://zhuque.in", "zhuque", true},       // 精确 host
		{"https://zhuque.in/index", "zhuque", true}, // 带路径仍按 host 匹配
		{"https://www.hxpt.org/", "hxpt", true},     // 命中 LegacyURLs（子域）
		{"https://hxpt.org", "hxpt", true},          // 命中 URLs（裸域）
		{"https://kp.m-team.cc", "mteam", true},
		{"https://unknown.example", "", false}, // 无匹配 → 回退 platform 推断
		{"", "", false},                        // 空 URL
		{"not-a-url", "", false},               // 无 host
	}
	for _, c := range cases {
		id, ok := r.MatchByURL(c.url)
		if id != c.want || ok != c.ok {
			t.Errorf("MatchByURL(%q) = (%q,%v), want (%q,%v)", c.url, id, ok, c.want, c.ok)
		}
	}
}
