// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/downloader"
)

// downloader.go 纯函数层单测：clampQueryInt / torrentStateGroup /
// containsString / filterTorrents / cmp* / sortTorrents——Torrents 端点的
// 查询语义（分页边界、状态归并、过滤组合、排序稳定性）全部在此收口。

func queryCtx(t *testing.T, qs string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req, _ := http.NewRequest(http.MethodGet, qs, nil)
	c.Request = req
	return c
}

// TestClampQueryInt 查询参数夹取：缺省/越界/合法三态。
func TestClampQueryInt(t *testing.T) {
	cases := []struct {
		query  string
		def    int
		lo, hi int
		want   int
	}{
		{"", 20, 1, 100, 20},           // 缺省
		{"limit=0", 20, 1, 100, 1},     // 下界
		{"limit=999", 20, 1, 100, 100}, // 上界
		{"limit=50", 20, 1, 100, 50},   // 合法
		{"limit=abc", 20, 1, 100, 20},  // 非数字回缺省
	}
	for _, tc := range cases {
		c := queryCtx(t, "/x?"+tc.query)
		if got := clampQueryInt(c, "limit", tc.def, tc.lo, tc.hi); got != tc.want {
			t.Errorf("clampQueryInt(%q) = %d, want %d", tc.query, got, tc.want)
		}
	}
}

// TestTorrentStateGroup 各下载器原始 state 归并到前端分组。
func TestTorrentStateGroup(t *testing.T) {
	cases := map[string]string{
		"error":       "error",
		"errorPAUSED": "error", // error 优先级最高
		"uploading":   "seeding",
		"Seeding":     "seeding",
		"paused":      "paused",
		"stopped":     "paused",
		"downloading": "downloading",
		"active":      "other",
		"":            "other",
	}
	for in, want := range cases {
		if got := torrentStateGroup(in); got != want {
			t.Errorf("torrentStateGroup(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestContainsString 精确匹配（非子串）。
func TestContainsString(t *testing.T) {
	ss := []string{"ptp", "keep"}
	if !containsString(ss, "ptp") {
		t.Error("应命中精确项")
	}
	if containsString(ss, "pt") {
		t.Error("不得子串误命中")
	}
	if containsString(nil, "x") {
		t.Error("nil 切片应 false")
	}
}

// TestFilterTorrents 组合过滤：category/state/tag/q 与全命中直通。
func TestFilterTorrents(t *testing.T) {
	ts := []downloader.Torrent{
		{Name: "Ubuntu 24.04 ISO", Category: "linux", State: "downloading", Tags: []string{"iso"}},
		{Name: "Debian 12 ISO", Category: "linux", State: "uploading", Tags: []string{"iso", "seed"}},
		{Name: "Movie 2023 4K", Category: "movie", State: "paused", Tags: []string{}},
	}

	// 无过滤条件：原样直通（同底层数组）
	if got := filterTorrents(ts, queryCtx(t, "/t")); len(got) != 3 {
		t.Fatalf("无过滤应直通 3 条，got %d", len(got))
	}
	// state=all 等价直通
	if got := filterTorrents(ts, queryCtx(t, "/t?state=all")); len(got) != 3 {
		t.Fatalf("state=all 应直通，got %d", len(got))
	}

	// 单条件
	if got := filterTorrents(ts, queryCtx(t, "/t?category=movie")); len(got) != 1 || got[0].Name != "Movie 2023 4K" {
		t.Fatalf("category 过滤错误: %+v", got)
	}
	// state 归并过滤：uploading → seeding 组
	if got := filterTorrents(ts, queryCtx(t, "/t?state=seeding")); len(got) != 1 || got[0].Name != "Debian 12 ISO" {
		t.Fatalf("state=seeding 过滤错误: %+v", got)
	}
	// tag
	if got := filterTorrents(ts, queryCtx(t, "/t?tag=seed")); len(got) != 1 {
		t.Fatalf("tag 过滤错误: %+v", got)
	}
	// q 大小写不敏感
	if got := filterTorrents(ts, queryCtx(t, "/t?q=ubuntu")); len(got) != 1 {
		t.Fatalf("q 过滤错误: %+v", got)
	}
	// 组合（AND）
	if got := filterTorrents(ts, queryCtx(t, "/t?category=linux&tag=iso&state=downloading")); len(got) != 1 || got[0].Name != "Ubuntu 24.04 ISO" {
		t.Fatalf("组合过滤错误: %+v", got)
	}
	// 零命中
	if got := filterTorrents(ts, queryCtx(t, "/t?q=nope")); len(got) != 0 {
		t.Fatalf("零命中应空，got %d", len(got))
	}
}

// TestSortTorrents 各排序键 + 方向（默认 added_at desc）。
func TestSortTorrents(t *testing.T) {
	base := time.Now()
	ts := []downloader.Torrent{
		{Name: "b", Size: 300, Progress: 0.5, Ratio: 2, DLRate: 10, UPRate: 1, AddedAt: base.Add(-time.Hour)},
		{Name: "a", Size: 100, Progress: 0.9, Ratio: 1, DLRate: 30, UPRate: 5, AddedAt: base},
		{Name: "c", Size: 200, Progress: 0.1, Ratio: 3, DLRate: 20, UPRate: 9, AddedAt: base.Add(-2 * time.Hour)},
	}

	sorted := func(q string) []string {
		cp := append([]downloader.Torrent(nil), ts...)
		sortTorrents(cp, queryCtx(t, "/t?"+q))
		out := make([]string, len(cp))
		for i, x := range cp {
			out[i] = x.Name
		}
		return out
	}

	if got := sorted(""); !equal(got, []string{"a", "b", "c"}) {
		t.Errorf("默认 added_at desc: %v", got)
	}
	if got := sorted("sort=name&order=asc"); !equal(got, []string{"a", "b", "c"}) {
		t.Errorf("name asc: %v", got)
	}
	if got := sorted("sort=size"); !equal(got, []string{"b", "c", "a"}) {
		t.Errorf("size desc: %v", got)
	}
	if got := sorted("sort=progress&order=asc"); !equal(got, []string{"c", "b", "a"}) {
		t.Errorf("progress asc: %v", got)
	}
	if got := sorted("sort=ratio&order=asc"); !equal(got, []string{"a", "b", "c"}) {
		t.Errorf("ratio asc: %v", got)
	}
	if got := sorted("sort=dl_rate&order=asc"); !equal(got, []string{"b", "c", "a"}) {
		t.Errorf("dl_rate asc: %v", got)
	}
	if got := sorted("sort=up_rate&order=asc"); !equal(got, []string{"b", "a", "c"}) {
		t.Errorf("up_rate asc: %v", got)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
