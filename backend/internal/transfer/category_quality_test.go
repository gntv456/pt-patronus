// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package transfer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hxpt/ptpatronus/internal/media"
)

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestQualityScore(t *testing.T) {
	cases := []struct {
		name string
		want int
	}{
		{"Movie.2024.2160p.UHD.BluRay.HDR.HEVC.Atmos-GROUP", 100}, // 90+8+5+3=106→100
		{"Movie.2024.2160p.WEB-DL.DDP5.1.H.266-GROUP", 93},        // 90+3=93
		{"Movie.2024.1080p.BluRay.x265.10bit-GROUP", 81},          // 70+8+3=81
		{"Movie.2024.1080p.WEB-DL.DDP5.01.H.264-GROUP", 74},       // 70+3+1=74
		{"Movie.2024.720p.WEB-DL.x264-GROUP", 54},                 // 50+3+1=54
		{"Movie.2024.480p.SDTV-GROUP", 32},
		{"random.mkv", 30},
	}
	for _, c := range cases {
		got := qualityScore(c.name)
		if got != c.want {
			t.Errorf("qualityScore(%q)=%d want %d", c.name, got, c.want)
		}
	}
}

func TestQualityScoreOrdering(t *testing.T) {
	// 洗版判定核心：评分必须单调反映「更高画质」。
	high := qualityScore("S01E01.2160p.UHD.BluRay.DV.HEVC-GROUP")
	mid := qualityScore("S01E01.1080p.BluRay.x265-GROUP")
	low := qualityScore("S01E01.720p.WEB-DL.x264-GROUP")
	if !(high > mid && mid > low) {
		t.Fatalf("ordering broken: high=%d mid=%d low=%d", high, mid, low)
	}
}

func TestFindEpisodeInDir(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "Show.S01E01.1080p.WEB-GROUP.mkv", "x")
	mustWrite(t, dir, "Show.S01E02.1080p.WEB-GROUP.mkv", "x")
	mustWrite(t, dir, "poster.jpg", "x")
	got := findEpisodeInDir(dir, "S01E02")
	if got == "" {
		t.Fatal("expected to find S01E02")
	}
	if findEpisodeInDir(dir, "S01E09") != "" {
		t.Fatal("should not find missing S01E09")
	}
}

func TestFirstVideoInDir(t *testing.T) {
	dir := t.TempDir()
	if firstVideoInDir(dir) != "" {
		t.Fatal("empty dir should return empty")
	}
	mustWrite(t, dir, "poster.jpg", "x")
	if firstVideoInDir(dir) != "" {
		t.Fatal("non-video only should return empty")
	}
	mustWrite(t, dir, "Movie.2024.1080p.mkv", "x")
	if firstVideoInDir(dir) == "" {
		t.Fatal("should find video")
	}
	// 样本文件应被忽略
	dir2 := t.TempDir()
	mustWrite(t, dir2, "Movie.sample.mkv", "x")
	if firstVideoInDir(dir2) != "" {
		t.Fatal("sample should be ignored")
	}
}

func TestResolveCategory(t *testing.T) {
	cfg := DefaultCategoryConfig()
	cases := []struct {
		mi   *media.MediaInfo
		want string
	}{
		{&media.MediaInfo{Type: media.TypeMovie, Genres: []string{"Animation"}, Countries: []string{"JP"}}, "动漫"},
		{&media.MediaInfo{Type: media.TypeMovie, Countries: []string{"CN"}}, "华语"},
		{&media.MediaInfo{Type: media.TypeTV, Countries: []string{"KR"}}, "韩剧"},
		{&media.MediaInfo{Type: media.TypeTV, Countries: []string{"US"}}, "欧美剧"},
		{&media.MediaInfo{Type: media.TypeMovie, Countries: []string{"FR"}}, ""}, // 无匹配
	}
	for _, c := range cases {
		got := ResolveCategory(cfg, c.mi)
		if got != c.want {
			t.Errorf("ResolveCategory(type=%v countries=%v)=%q want %q", c.mi.Type, c.mi.Countries, got, c.want)
		}
	}
}

func TestResolveCategoryFirstMatchWins(t *testing.T) {
	// 自定义规则保序：动漫(genre)须优先于日剧(country)，否则日本动画被分进日剧。
	cfg := CategoryConfig{
		TV: []NamedRule{
			{Name: "动漫", Rule: CategoryRule{Genres: []string{"Animation"}}},
			{Name: "日剧", Rule: CategoryRule{Countries: []string{"JP"}}},
		},
	}
	mi := &media.MediaInfo{Type: media.TypeTV, Genres: []string{"Animation"}, Countries: []string{"JP"}}
	if got := ResolveCategory(cfg, mi); got != "动漫" {
		t.Fatalf("first-match-wins failed: got %q want 动漫", got)
	}
}

func TestMatchYearRange(t *testing.T) {
	if !matchYear("2020-2024", 2022) {
		t.Error("2022 should be in 2020-2024")
	}
	if matchYear("2020-2024", 2019) {
		t.Error("2019 should not be in 2020-2024")
	}
	if !matchYear("2024", 2024) {
		t.Error("single year match failed")
	}
	if matchYear("2024", 2023) {
		t.Error("single year mismatch failed")
	}
}

func TestCategoryExclude(t *testing.T) {
	// genres "!Animation" 排除动画；国家 JP 命中但被 genre 排除 → 不满足
	r := CategoryRule{Countries: []string{"JP"}, Genres: []string{"!Animation"}}
	mi := &media.MediaInfo{Type: media.TypeTV, Countries: []string{"JP"}, Genres: []string{"Animation"}}
	if matchCategory(r, mi) {
		t.Fatal("should not match: Animation excluded")
	}
	// 非动画 + JP → 满足
	mi2 := &media.MediaInfo{Type: media.TypeTV, Countries: []string{"JP"}, Genres: []string{"Drama"}}
	if !matchCategory(r, mi2) {
		t.Fatal("should match: JP drama")
	}
}
