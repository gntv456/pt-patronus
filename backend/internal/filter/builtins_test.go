// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filter

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/site"
)

// TestBuiltinRegexCompile 所有 22 内置规则的 include/exclude 必须能在 Go RE2 下编译通过
// （验证 BLU.exclude / CNSUB.include 的 lookahead/lookbehind/空交替改写生效）。
func TestBuiltinRegexCompile(t *testing.T) {
	if len(BuiltinRuleSet) != 22 {
		t.Errorf("内置规则数应为 22，实际 %d", len(BuiltinRuleSet))
	}
	for id, r := range BuiltinRuleSet {
		for _, p := range r.Include {
			if _, err := compilePattern(p); err != nil {
				t.Errorf("规则 %s include 编译失败：%v (pattern=%q)", id, err, p)
			}
		}
		for _, p := range r.Exclude {
			if _, err := compilePattern(p); err != nil {
				t.Errorf("规则 %s exclude 编译失败：%v (pattern=%q)", id, err, p)
			}
		}
	}
}

func TestBuiltinMatches(t *testing.T) {
	rules := map[string]Rule{
		"4K":     BuiltinRuleSet["4K"],
		"1080P":  BuiltinRuleSet["1080P"],
		"H265":   BuiltinRuleSet["H265"],
		"H264":   BuiltinRuleSet["H264"],
		"BLURAY": BuiltinRuleSet["BLURAY"],
		"WEBDL":  BuiltinRuleSet["WEBDL"],
		"CNSUB":  BuiltinRuleSet["CNSUB"],
	}
	cases := []struct {
		rule, title string
		want        bool
	}{
		{"4K", "Movie 2160p UHD", true},
		{"4K", "Movie 1080p", false},
		{"1080P", "Show 1080p x264", true},
		{"H265", "Movie x265 HEVC", true},
		{"H264", "Movie x264 AVC", true},
		{"BLURAY", "Movie BluRay", true},
		{"WEBDL", "Show WEB-DL", true},
		{"WEBDL", "Show BluRay", false},
		// CNSUB 改写后仍命中中字特征。
		{"CNSUB", "Movie 中字 简体", true},
		{"CNSUB", "Movie.chs", true},
		{"CNSUB", "Movie gb", true}, // 独立词 gb 命中改写后的 (?:^|[^a-z0-9])(gb)(?:[^a-z0-9]|$)
	}
	for _, c := range cases {
		got := matchRule(site.Torrent{Title: c.title}, c.rule, nil, rules)
		if got != c.want {
			t.Errorf("rule=%s title=%q match=%v want=%v", c.rule, c.title, got, c.want)
		}
	}
}

// TestBuiltinBLUExclude BLU 原盘规则：含 x264 等编码应被 exclude 排除（不匹配 BLU）。
func TestBuiltinBLUExclude(t *testing.T) {
	rules := map[string]Rule{"BLU": BuiltinRuleSet["BLU"]}
	// 命中 include（UHD+HEVC）但不含 exclude 项 → 匹配。
	if !matchRule(site.Torrent{Title: "Movie 2160p UHD BluRay HEVC"}, "BLU", nil, rules) {
		t.Fatal("2160p UHD BluRay HEVC 应匹配 BLU")
	}
	// 命中 include 但 exclude 命中 x264 → 不匹配。
	// 构造一个 include 命中（含 BD25）但含 x264 的标题。
	if matchRule(site.Torrent{Title: "Movie BD25 x264"}, "BLU", nil, rules) {
		t.Fatal("BD25 + x264 应被 BLU.exclude(x264) 排除")
	}
}
