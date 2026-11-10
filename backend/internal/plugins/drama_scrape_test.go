// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDramaCleanName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"我的短剧.S01E02.1080p.HEVC.mp4", "我的短剧"},
		{"[2024]剧名EP05【完结】", "剧名"},
		{"剧.名_2024", "剧 名 2024"},
		{"  剧名  ", "剧名"},
		{"", ""},
		{"(蓝光)某剧.x264.4K", "某剧"},
	}
	for _, c := range cases {
		if got := dramaCleanName(c.in); got != c.want {
			t.Errorf("cleanName(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestDramaPickMatch(t *testing.T) {
	if got := dramaPickMatch("剧A", nil); got != nil {
		t.Errorf("empty items should return nil, got %+v", got)
	}
	items := []xgItem{{Title: "剧A"}, {Title: "剧B"}}
	if got := dramaPickMatch("剧A", items); got == nil || got.Title != "剧A" {
		t.Errorf("exact match want 剧A, got %+v", got)
	}
	if got := dramaPickMatch("剧", items); got == nil || got.Title != "剧A" {
		t.Errorf("substring match want first(剧A), got %+v", got)
	}
	if got := dramaPickMatch("xyz", items); got == nil || got.Title != "剧A" {
		t.Errorf("fallback want first(剧A), got %+v", got)
	}
}

func TestWriteTVShowNFO(t *testing.T) {
	d := &xgDetailItem{
		xgItem: xgItem{
			ID: 123, Title: "测试剧", AltTitle: "Test Show", Cover: "/img.jpg",
			Description: "简介", ReleaseYear: 2024, Rating: 8.5, Tags: "短剧，甜宠",
			Region: "中国", TotalEpisodes: 20,
		},
		Casts: []xgCast{{Name: "演员A", Role: "主角"}, {Name: "", Role: "跳过空名"}},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "tvshow.nfo")
	if err := writeTVShowNFO(path, d); err != nil {
		t.Fatalf("write nfo: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read nfo: %v", err)
	}
	s := string(raw)
	wantSubs := []string{
		"<?xml version='1.0' encoding='utf-8'?>",
		"<title>测试剧</title>",
		"<originaltitle>Test Show</originaltitle>",
		"<sorttitle>测试剧</sorttitle>",
		"<plot>简介</plot>",
		"<year>2024</year>",
		"<genre>短剧</genre>",
		"<genre>甜宠</genre>",
		"<country>中国</country>",
		"<studio>寸光集</studio>",
		`<rating name="cunguangji" max="10" default="true">`,
		"<value>8.5</value>",
		"<votes>0</votes>",
		`<uniqueid type="cunguangji" default="true">123</uniqueid>`,
		"<name>演员A</name>",
		"<role>主角</role>",
		"<order>0</order>",
		"<season>1</season>",
		"<episode>20</episode>",
	}
	for _, w := range wantSubs {
		if !strings.Contains(s, w) {
			t.Errorf("NFO missing %q\n--- got ---\n%s", w, s)
		}
	}
	if strings.Contains(s, "跳过空名") {
		t.Errorf("empty-name cast should be skipped")
	}
}

func TestResolveDramaURL(t *testing.T) {
	cases := []struct{ base, in, want string }{
		{"https://ptang.top", "", ""},
		{"https://ptang.top", "https://x.com/a.jpg", "https://x.com/a.jpg"},
		{"https://ptang.top", "/img.jpg", "https://ptang.top/img.jpg"},
		{"https://ptang.top/", "/img.jpg", "https://ptang.top/img.jpg"},
		{"https://ptang.top", "rel.jpg", "rel.jpg"},
	}
	for _, c := range cases {
		if got := resolveDramaURL(c.base, c.in); got != c.want {
			t.Errorf("resolveURL(%q,%q)=%q want %q", c.base, c.in, got, c.want)
		}
	}
}

func TestParseVideoExts(t *testing.T) {
	m := parseVideoExts("mp4, .mkv, AVI")
	for _, e := range []string{".mp4", ".mkv", ".avi"} {
		if !m[e] {
			t.Errorf("ext %s should be present", e)
		}
	}
	if m[".ts"] {
		t.Errorf(".ts should not be present")
	}
	m2 := parseVideoExts("")
	if !m2[".mp4"] || !m2[".webm"] {
		t.Errorf("empty should fall back to defaults")
	}
}
