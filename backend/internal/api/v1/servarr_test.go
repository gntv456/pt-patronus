// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

func TestParseTerm(t *testing.T) {
	cases := []struct {
		in      string
		wantSrc string
		wantID  string
	}{
		{"tmdb:123", "tmdb", "123"},
		{"tvdb:456", "tvdb", "456"},
		{"789", "tmdb", "789"}, // 无前缀默认 tmdb
		{"", "", ""},
	}
	for _, c := range cases {
		src, id := parseTerm(c.in)
		if src != c.wantSrc || id != c.wantID {
			t.Errorf("parseTerm(%q)=(%q,%q) want (%q,%q)", c.in, src, id, c.wantSrc, c.wantID)
		}
	}
}

func TestSubtleEqual(t *testing.T) {
	if subtleEqual("abc", "abc") != 1 {
		t.Error("equal strings should return 1")
	}
	if subtleEqual("abc", "abd") != 0 {
		t.Error("differing strings should return 0")
	}
	if subtleEqual("abc", "ab") != 0 {
		t.Error("different length should return 0")
	}
}

func TestParseIDSafe(t *testing.T) {
	if parseIDSafe("42") != 42 {
		t.Error("numeric parse failed")
	}
	if parseIDSafe("abc") != 0 {
		t.Error("non-numeric should return 0")
	}
}

// TestServarrRadarrMapping 验证 Subscription → RadarrMovie 映射 + hasFile 注入。
func TestServarrRadarrMapping(t *testing.T) {
	h := &ServarrHandler{hasFile: func(source, sourceID string) bool {
		return source == "tmdb" && sourceID == "123"
	}}
	sub := &models.Subscription{ID: 7, Source: "tmdb", SourceID: "123", Title: "X", Year: 2024, Type: "movie", Status: "subscribed"}
	m := h.toRadarrMovie(sub)
	if m.ID != 7 || m.Title != "X" || m.Year != 2024 || m.TmdbID != 123 {
		t.Errorf("mapping wrong: %+v", m)
	}
	if !m.Monitored {
		t.Error("active sub should be monitored")
	}
	if !m.HasFile {
		t.Error("hasFile should reflect injection")
	}

	// hasFile=nil 时恒 false。
	h2 := &ServarrHandler{}
	if h2.toRadarrMovie(sub).HasFile {
		t.Error("nil hasFile should be false")
	}
}

func TestServarrSonarrMapping(t *testing.T) {
	h := &ServarrHandler{}
	sub := &models.Subscription{ID: 9, Source: "tmdb", SourceID: "5", Title: "Show", Year: 2023, Type: "tv", Status: "completed"}
	s := h.toSonarrSeries(sub)
	if s.ID != 9 || s.Title != "Show" || s.SeasonCount != 1 || len(s.Seasons) != 1 {
		t.Errorf("series mapping wrong: %+v", s)
	}
	if s.Monitored {
		t.Error("completed sub should not be monitored")
	}
}
