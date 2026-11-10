// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

func setOf(ss ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}

func runTitleCases(t *testing.T, sub *models.Subscription, accept map[string]struct{}, cases []struct {
	seed string
	keep bool
	why  string
}) {
	t.Helper()
	var ts []site.Torrent
	for _, c := range cases {
		ts = append(ts, site.Torrent{Title: c.seed})
	}
	got := filterByTitleMatch(ts, sub, accept)
	gotSet := map[string]struct{}{}
	for _, x := range got {
		gotSet[x.Title] = struct{}{}
	}
	for _, c := range cases {
		_, kept := gotSet[c.seed]
		if kept != c.keep {
			t.Errorf("seed %q: keep=%v want %v (%s)", c.seed, kept, c.keep, c.why)
		}
	}
}

// 无别名（mediaCh 未装配/媒体无 aka）：同语言精确 + 跨语言放行（不误杀）。
func TestFilterByTitleMatchNoAlias(t *testing.T) {
	sub := &models.Subscription{Title: "特立独行", Year: 2026, Type: "movie"}
	accept := setOf(normalizeTitle("特立独行"))
	runTitleCases(t, sub, accept, []struct {
		seed string
		keep bool
		why  string
	}{
		{"特立独行.2026.1080p.x264-A", true, "精确命中"},
		{"一只特立独行的猪.2026.mkv", false, "同语言不命中→过滤（误命中）"},
		{"A Maverick Pig.2026.mkv", true, "接受集无英文别名→跨语言放行不误杀"},
		{"特立独行.2010.mkv", false, "年份差>1"},
		{"特立独行.S01E01.mkv", false, "TV vs movie 类型不符"},
	})
}

// 有英文别名（豆瓣 aka/TMDB alt 含英文名）：跨语言也精确匹配。
func TestFilterByTitleMatchWithEnAlias(t *testing.T) {
	sub := &models.Subscription{Title: "让子弹飞", Year: 2010, Type: "movie"}
	accept := setOf(normalizeTitle("让子弹飞"), normalizeTitle("Let the Bullets Fly"))
	runTitleCases(t, sub, accept, []struct {
		seed string
		keep bool
		why  string
	}{
		{"Let.the.Bullets.Fly.2010.Bluray.1080p-PTer", true, "英文别名精确命中"},
		{"让子弹飞.2010.1080p.mkv", true, "中文精确命中"},
		{"让子弹飞.2010.1080p.简繁中字-CMCT风潇潇", true, "clean 含噪声但拆词含让子弹飞→兜底命中"},
		{"让子弹飞2.2010.mkv", false, "拆词为让子弹飞2≠让子弹飞+中文不命中→过滤"},
		{"一只特立独行的猪.2010.mkv", false, "连续中文不拆→拆词不含让子弹飞+中文不命中→过滤"},
		{"A.Maverick.Pig.2010.mkv", false, "英文不命中+接受集有英文→跨语言也过滤"},
		{"Let.the.Bullets.Fly.2000.mkv", false, "别名命中但年份差>1→过滤"},
	})
}

// 中文订阅 + 接受集无英文（豆瓣国产片 aka 仅中文）：英文种子放行（不误杀）。
func TestFilterByTitleMatchNoEnAliasCrossLangKeep(t *testing.T) {
	sub := &models.Subscription{Title: "让子弹飞", Year: 2010, Type: "movie"}
	accept := setOf(normalizeTitle("让子弹飞"))
	runTitleCases(t, sub, accept, []struct {
		seed string
		keep bool
		why  string
	}{
		{"Let.the.Bullets.Fly.2010-PTer", true, "接受集无英文→放行不误杀"},
		{"让子弹飞.2010.mkv", true, "中文命中"},
		{"让子弹飞2.2010.mkv", false, "中文不命中→过滤"},
	})
}

// buildSubscribeAcceptSet：mediaCh 未装配时仅含订阅 Title（回退不误杀）。
func TestBuildSubscribeAcceptSetNilMedia(t *testing.T) {
	sub := &models.Subscription{Title: "让子弹飞", Source: "douban", SourceID: "movie:1"}
	accept := buildSubscribeAcceptSet(context.Background(), nil, sub)
	if _, ok := accept[normalizeTitle("让子弹飞")]; !ok {
		t.Error("missing subscribe title in accept set")
	}
	if len(accept) != 1 {
		t.Errorf("nil mediaCh: want accept set of 1, got %d", len(accept))
	}
}

func TestNormalizeTitleAndCJK(t *testing.T) {
	if normalizeTitle("Let the Bullets Fly!") != "letthebulletsfly" {
		t.Errorf("normalize en: %q", normalizeTitle("Let the Bullets Fly!"))
	}
	if normalizeTitle("让子弹飞（2010）") != "让子弹飞2010" {
		t.Errorf("normalize cn: %q", normalizeTitle("让子弹飞（2010）"))
	}
	if !containsCJK("让子弹飞") {
		t.Error("containsCJK should be true for Chinese")
	}
	if containsCJK("Let the Bullets Fly") {
		t.Error("containsCJK should be false for English")
	}
}

// matchResolution 数字匹配须带词边界：标题中非分辨率上下文的数字不应误命中。
// 修复缺陷：12160.x 不应命中 2160p（12160 是连续数字，非独立分辨率词）。
func TestMatchResolutionNumBoundary(t *testing.T) {
	cases := []struct {
		title string
		want  string
		ok    bool
		why   string
	}{
		{"Movie.2160p.BluRay", "2160p", true, "完整 2160p 命中"},
		{"Movie.2160.BluRay", "2160p", true, "2160 独立数字词命中"},
		{"Movie.4K.HDR", "2160p", true, "4K↔2160 互认"},
		{"Movie.4K.HDR", "4K", true, "4K 直命中"},
		{"Movie.2160p", "4K", true, "2160↔4K 互认"},
		// 词边界修复的关键 case：
		{"Movie.12160p.x264", "2160p", false, "12160 前字符 1 是数字→非独立词，不应命中 2160p"},
		{"Movie.Size12160.Bytes", "2160p", false, "12160 嵌在更长数字中→不应命中"},
		{"Movie.1080p.WEB", "1080p", true, "1080p 正常命中"},
		{"Movie.1080.WEB", "1080p", true, "1080 独立词命中"},
		{"Movie.x1080y", "1080p", true, "1080 前后是字母非数字→词边界通过→命中（字母不挡，仅挡数字粘连）"},
		{"Movie.720p.WEB", "1080p", false, "720≠1080 不命中"},
	}
	for _, c := range cases {
		got := matchResolution(c.title, c.want)
		if got != c.ok {
			t.Errorf("matchResolution(%q,%q)=%v want %v (%s)", c.title, c.want, got, c.ok, c.why)
		}
	}
}

// containsNumWord 独立数字词判断（前后非 ASCII 数字或边界）。
func TestContainsNumWord(t *testing.T) {
	cases := []struct {
		t, num string
		ok     bool
	}{
		{"2160", "2160", true},
		{".2160.", "2160", true},
		{"MOVIE.2160P", "2160", true},
		{"12160", "2160", false},   // 前字符 1 是数字
		{"21601", "2160", false},   // 后字符 1 是数字
		{"A12160B", "2160", false}, // 前后都是数字（1 和 0）→ 嵌在 12160 中
		{"4K.2160", "2160", true},
		{"", "2160", false},
		{"2160", "", false},
	}
	for _, c := range cases {
		if got := containsNumWord(c.t, c.num); got != c.ok {
			t.Errorf("containsNumWord(%q,%q)=%v want %v", c.t, c.num, got, c.ok)
		}
	}
}

// 短词订阅拆词兜底门槛：长度<=2 的非 CJK token 须作首 token 且第二 token 是年份才命中，
// 避免 "Up" 订阅误命中 "Up.In.The.Air"。
func TestTokenHitsAcceptShortTokenGate(t *testing.T) {
	cases := []struct {
		name   string
		title  string
		accept map[string]struct{}
		hit    bool
		why    string
	}{
		{
			"短词作品本身：Up.2009.BluRay",
			"Up.2009.BluRay-GROUP",
			setOf("up"),
			true, "首 token=up 且第二 token=2009 年份→命中",
		},
		{
			"短词误命中：Up.In.The.Air",
			"Up.In.The.Air.2009.1080p-GROUP",
			setOf("up"),
			false, "首 token=up 但第二=in 非年份→是另一多词作品，不命中",
		},
		{
			"短词误命中：It.Follows",
			"It.Follows.2014",
			setOf("it"),
			false, "首 token=it 但第二=follows 非年份→不命中",
		},
		{
			"短词作品：It.2017",
			"It.2017.BluRay.1080p-GROUP",
			setOf("it"),
			true, "首 token=it 第二=2017 年份→命中",
		},
		{
			"短词唯一 token：Up",
			"Up",
			setOf("up"),
			true, "唯一 token=up→命中（无第二 token 干扰）",
		},
		{
			"长词不受门槛：让子弹飞.简繁中字.CMCT",
			"让子弹飞.简繁中字.CMCT",
			setOf("让子弹飞"),
			true, "CJK 长词走全量 token 集，拆词含让子弹飞→命中",
		},
		{
			"长词英文：让子弹飞.Let.the.Bullets.Fly",
			"Let.the.Bullets.Fly.2010.Bluray.1080p-PTer",
			setOf("letthebulletsfly"),
			false, "clean 后长词 letthebulletsfly 不在拆词集（拆出 let/the/bullets/fly）→不命中",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tokens := splitTitleTokens(c.title)
			if got := tokenHitsAccept(tokens, c.accept); got != c.hit {
				t.Errorf("tokenHitsAccept(%v)=%v want %v (%s)", tokens, got, c.hit, c.why)
			}
		})
	}
}

// 纯英文别名订阅不应放过同年中文无关种子（缺陷 6）。
// sub.Title 非 CJK、acceptSet 全英文、种子 CJK 且未精确/拆词命中 → 过滤。
func TestFilterByTitleMatchEnSubCJKSeedFiltered(t *testing.T) {
	sub := &models.Subscription{Title: "Maverick", Year: 2026, Type: "movie"}
	accept := setOf(normalizeTitle("Maverick")) // 纯英文接受集
	runTitleCases(t, sub, accept, []struct {
		seed string
		keep bool
		why  string
	}{
		{"Maverick.2026.1080p-GROUP", true, "英文精确命中"},
		{"Maverick.2025.mkv", true, "英文精确+年份门控通过（差<=1）"},
		// 关键缺陷 6 case：英文订阅 + 中文种子未精确命中 → 过滤（旧逻辑会放行）
		{"独行侠.2026.mkv", false, "英文订阅+中文种子未精确命中→过滤（缺陷6修复）"},
		{"特立独行.2026.mkv", false, "英文订阅+中文种子（含 maverick 语义但 normalize 后不等）→过滤"},
		{"Maverick.2026.S01E01.mkv", false, "TV vs movie 类型不符"},
	})
}

// CJK 订阅 + 英文种子仍放行（不误杀），缺陷 6 修复不应破坏此跨语言放行。
func TestFilterByTitleMatchCJKSubEnSeedStillKeeps(t *testing.T) {
	sub := &models.Subscription{Title: "让子弹飞", Year: 2010, Type: "movie"}
	accept := setOf(normalizeTitle("让子弹飞")) // 仅中文接受集
	runTitleCases(t, sub, accept, []struct {
		seed string
		keep bool
		why  string
	}{
		{"Let.the.Bullets.Fly.2010-PTer", true, "CJK 订阅+英文种子→放行不误杀（缺陷6不影响反向）"},
		{"让子弹飞.2010.mkv", true, "中文精确命中"},
	})
}
