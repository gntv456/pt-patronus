// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package media

import "testing"

// TestTypeMatches 锁定 filter → 允许类型的映射。「影视」默认 video 只放行 movie+tv，
// 排除音乐——这是「搜电影不再出全是音乐」的核心。
func TestTypeMatches(t *testing.T) {
	cases := []struct {
		t      MediaType
		filter string
		want   bool
	}{
		// filter="" 全放行（MatchPoster 内部用，保留音乐）
		{TypeMovie, "", true},
		{TypeMusic, "", true},
		// video：movie+tv 放行，music 拒绝
		{TypeMovie, "video", true},
		{TypeTV, "video", true},
		{TypeMusic, "video", false},
		// 精确
		{TypeMovie, "movie", true},
		{TypeTV, "movie", false},
		{TypeMusic, "music", true},
		{TypeMovie, "music", false},
		// 未知 filter 保守放行
		{TypeMusic, "???", true},
	}
	for _, c := range cases {
		if got := TypeMatches(c.t, c.filter); got != c.want {
			t.Errorf("TypeMatches(%q,%q)=%v want %v", c.t, c.filter, got, c.want)
		}
	}
}

// TestSourceMatches 锁定「源产物是否落在 filter 内」——Chain 据此跳过必无用的源
// （如 video 搜时跳过只产音乐的 MusicBrainz，省一次无用且常慢的海外请求）。
func TestSourceMatches(t *testing.T) {
	mb := []MediaType{TypeMusic}            // MusicBrainz
	video := []MediaType{TypeMovie, TypeTV} // TMDB/豆瓣
	if SourceMatches(mb, "") != true {
		t.Error("filter=空应放行所有源")
	}
	if SourceMatches(mb, "video") != false {
		t.Error("MusicBrainz(video 搜)应被跳过")
	}
	if SourceMatches(mb, "music") != true {
		t.Error("MusicBrainz(music 搜)应保留")
	}
	if SourceMatches(video, "video") != true {
		t.Error("TMDB/豆瓣(video 搜)应保留")
	}
	if SourceMatches(video, "music") != false {
		t.Error("TMDB/豆瓣(music 搜)应被跳过")
	}
}
