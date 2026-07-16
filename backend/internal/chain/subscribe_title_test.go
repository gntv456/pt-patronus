package chain

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

func titles(ts []site.Torrent) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Title
	}
	return out
}

func setOf(ss ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}

func runTitleCases(t *testing.T, sub *models.Subscription, accept map[string]struct{}, cases []struct{ seed string; keep bool; why string }) {
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
	runTitleCases(t, sub, accept, []struct{ seed string; keep bool; why string }{
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
	runTitleCases(t, sub, accept, []struct{ seed string; keep bool; why string }{
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
	runTitleCases(t, sub, accept, []struct{ seed string; keep bool; why string }{
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
