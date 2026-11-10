// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import "testing"

func TestLoadSaveEpisodePriority(t *testing.T) {
	orig := `{"S01E02":80,"S01E03":95}`
	m := loadEpisodePriority(orig)
	if m["S01E02"] != 80 || m["S01E03"] != 95 {
		t.Fatalf("load wrong: %v", m)
	}
	out := saveEpisodePriority(m)
	m2 := loadEpisodePriority(out)
	if m2["S01E02"] != 80 || m2["S01E03"] != 95 {
		t.Fatalf("round-trip wrong: %v", m2)
	}
	if saveEpisodePriority(map[string]int{}) != "" {
		t.Error("empty map should serialize to empty string")
	}
	if len(loadEpisodePriority("not json")) != 0 {
		t.Error("dirty data should yield empty map")
	}
}

func TestEpisodeKeys(t *testing.T) {
	keys, pack := episodeKeys("Show.Name.S01E02.1080p.WEB-GROUP")
	if pack || len(keys) != 1 || keys[0] != "S01E02" {
		t.Errorf("single-ep parse wrong: keys=%v pack=%v", keys, pack)
	}
	keys, pack = episodeKeys("Show.Name.S01.1080p.BluRay-GROUP")
	if !pack || len(keys) != 0 {
		t.Errorf("season-pack parse wrong: keys=%v pack=%v", keys, pack)
	}
	keys, pack = episodeKeys("Some.Movie.2024.1080p.mkv")
	if pack || len(keys) != 0 {
		t.Errorf("movie parse wrong: keys=%v pack=%v", keys, pack)
	}
}

func TestInterestedEpisode(t *testing.T) {
	ep := map[string]int{"S01E01": 70, "S01E02": 90}
	// 单集：未更优 → 不感兴趣
	if interestedEpisode(ep, []string{"S01E02"}, false, 70) {
		t.Error("equal/lower pri should not be interested")
	}
	// 单集：更优 → 感兴趣
	if !interestedEpisode(ep, []string{"S01E02"}, false, 95) {
		t.Error("higher pri should be interested")
	}
	// 单集：新集（首次）→ 感兴趣
	if !interestedEpisode(ep, []string{"S01E03"}, false, 60) {
		t.Error("first-time ep should be interested")
	}
	// 整季包：pri 高于最弱集 → 感兴趣
	if !interestedEpisode(ep, nil, true, 80) {
		t.Error("season pack above weakest should be interested")
	}
	// 整季包：pri 不高于任何 → 不感兴趣
	if interestedEpisode(ep, nil, true, 70) {
		t.Error("season pack at/below all should not be interested")
	}
	// 空 map + 整季包 → 感兴趣（首次建立基线）
	if !interestedEpisode(map[string]int{}, nil, true, 50) {
		t.Error("first season pack should be interested")
	}
}

func TestUpdateEpisodePriority(t *testing.T) {
	ep := map[string]int{"S01E01": 70}
	// 单集更优 → 更新
	updateEpisodePriority(ep, []string{"S01E01"}, false, 90)
	if ep["S01E01"] != 90 {
		t.Fatalf("should update to 90, got %d", ep["S01E01"])
	}
	// 单集未更优 → 保持（单调不降）
	updateEpisodePriority(ep, []string{"S01E01"}, false, 50)
	if ep["S01E01"] != 90 {
		t.Fatalf("should not downgrade, got %d", ep["S01E01"])
	}
	// 整季包：提升所有已见集
	ep = map[string]int{"S01E01": 70, "S01E02": 80}
	updateEpisodePriority(ep, nil, true, 95)
	if ep["S01E01"] != 95 || ep["S01E02"] != 95 {
		t.Fatalf("season pack should bump all seen eps, got %v", ep)
	}
}

// isSeasonPackTitle 整季包识别（BestVersionFull 整季包优先排序用）。
func TestIsSeasonPackTitle(t *testing.T) {
	cases := []struct {
		title string
		pack  bool
		why   string
	}{
		{"Show.Name.S01.1080p.BluRay-GROUP", true, "S01 纯季号=整季包"},
		{"Show.Name.S01-S03.1080p", true, "仅季号=整季包"},
		{"Show.Name.S01E02.1080p.WEB-GROUP", false, "单集非整季包"},
		{"Some.Movie.2024.1080p.mkv", false, "电影非整季包"},
		{"Show.Name.Complete.S01.2160p", true, "含 S01=整季包"},
	}
	for _, c := range cases {
		got := isSeasonPackTitle(c.title)
		if got != c.pack {
			t.Errorf("isSeasonPackTitle(%q)=%v want %v (%s)", c.title, got, c.pack, c.why)
		}
	}
}

// interestedMovie 电影洗版门控：无画质信息（pri<=0，即未配过滤规则组）时只放行首版建立基线。
// 旧实现是 `pri <= CurrentPriority → continue`，新订阅 CurrentPriority=0、候选 pri=0 →
// 勾了「优先最佳版本」却没配规则组的电影订阅一条都不下（功能静默失效）。
func TestInterestedMovie(t *testing.T) {
	cases := []struct {
		name     string
		cur, pri int
		baseline bool
		want     bool
	}{
		{"无画质信息+未推过→放行首版", 0, 0, false, true},
		{"无画质信息+已有基线→不再放行（防多站重复下）", 0, 0, true, false},
		{"无画质信息+已推过（即便记到更高优先级）→不放行", 90, 0, true, false},
		{"有画质信息+未记录过→放行", 0, 20, true, true},
		{"有画质信息+同级→洗版不成立", 80, 80, true, false},
		{"有画质信息+更低→不洗版", 80, 70, true, false},
		{"有画质信息+更高→洗版放行", 80, 95, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := interestedMovie(tc.cur, tc.pri, tc.baseline); got != tc.want {
				t.Errorf("interestedMovie(cur=%d, pri=%d, baseline=%v)=%v want %v", tc.cur, tc.pri, tc.baseline, got, tc.want)
			}
		})
	}
}
