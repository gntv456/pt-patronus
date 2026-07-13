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
