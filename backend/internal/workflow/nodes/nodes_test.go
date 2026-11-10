// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

func torrent(title string, seeders int, free bool, size string) site.Torrent {
	return site.Torrent{Title: title, Seeders: seeders, Free: free, Size: size}
}

func TestSortTorrents(t *testing.T) {
	ecx := &workflow.Context{Torrents: []site.Torrent{
		torrent("a", 5, false, ""),
		torrent("b", 50, false, ""),
		torrent("c", 10, false, ""),
	}}
	n := NewSortTorrentsNode()
	if err := n.Execute(context.Background(), map[string]any{"field": "seeders", "order": "desc"}, ecx); err != nil {
		t.Fatal(err)
	}
	if ecx.Torrents[0].Title != "b" || ecx.Torrents[1].Title != "c" || ecx.Torrents[2].Title != "a" {
		t.Fatalf("desc by seeders wrong order: %+v", ecx.Torrents)
	}
	// 升序
	ecx = &workflow.Context{Torrents: []site.Torrent{torrent("a", 5, false, ""), torrent("b", 50, false, "")}}
	if err := n.Execute(context.Background(), map[string]any{"field": "seeders", "order": "asc"}, ecx); err != nil {
		t.Fatal(err)
	}
	if ecx.Torrents[0].Title != "a" {
		t.Fatalf("asc by seeders wrong order: %+v", ecx.Torrents)
	}
}

func TestDedupeTorrents(t *testing.T) {
	// 同标题（归一化后）保留做种最多者
	ecx := &workflow.Context{Torrents: []site.Torrent{
		torrent("Movie 2024", 5, false, ""),
		torrent("Movie.2024", 50, false, ""), // 归一化同为 "movie 2024"
		torrent("Other", 1, false, ""),
	}}
	n := NewDedupeTorrentsNode()
	if err := n.Execute(context.Background(), map[string]any{"keep": "seeders"}, ecx); err != nil {
		t.Fatal(err)
	}
	if len(ecx.Torrents) != 2 {
		t.Fatalf("expected 2 after dedupe, got %d: %+v", len(ecx.Torrents), ecx.Torrents)
	}
	// 保留的 movie 组应是 seeders=50
	var movie site.Torrent
	for _, x := range ecx.Torrents {
		if strings.HasPrefix(x.Title, "Movie") {
			movie = x
		}
	}
	if movie.Seeders != 50 {
		t.Fatalf("dedupe kept wrong keeper: %+v", movie)
	}
}

func TestPickBest(t *testing.T) {
	ecx := &workflow.Context{Torrents: []site.Torrent{
		torrent("X 1080p", 100, false, ""),
		torrent("X 2160p", 5, false, ""), // 分辨率更高，应排前
		torrent("Y 1080p", 1, true, ""),  // 免费，分值最高
	}}
	n := NewPickBestNode()
	if err := n.Execute(context.Background(), map[string]any{"count": 1}, ecx); err != nil {
		t.Fatal(err)
	}
	if len(ecx.Torrents) != 1 || !ecx.Torrents[0].Free {
		t.Fatalf("pick_best 应选免费项，got %+v", ecx.Torrents)
	}
}

func TestRandomPick(t *testing.T) {
	ts := make([]site.Torrent, 10)
	for i := range ts {
		ts[i] = torrent("t", i, false, "")
	}
	ecx := &workflow.Context{Torrents: ts}
	n := NewRandomPickNode()
	if err := n.Execute(context.Background(), map[string]any{"count": 3}, ecx); err != nil {
		t.Fatal(err)
	}
	if len(ecx.Torrents) != 3 {
		t.Fatalf("expected 3, got %d", len(ecx.Torrents))
	}
}

func TestStopFlow(t *testing.T) {
	ecx := &workflow.Context{}
	n := NewStopFlowNode()
	if err := n.Execute(context.Background(), map[string]any{}, ecx); err != nil {
		t.Fatal(err)
	}
	if !ecx.Stop {
		t.Fatal("ecx.Stop 未置 true")
	}
}

func TestFormatSize(t *testing.T) {
	ecx := &workflow.Context{Variables: map[string]any{"uploaded": int64(16106127360)}} // 15 GiB
	n := NewFormatSizeNode()
	if err := n.Execute(context.Background(), map[string]any{"value": "{{var.uploaded}}", "output_var": "h"}, ecx); err != nil {
		t.Fatal(err)
	}
	if ecx.Variables["h"] != "15.00 GiB" {
		t.Fatalf("format wrong: %v", ecx.Variables["h"])
	}
	if humanBytes(500) != "500 B" || humanBytes(2048) != "2.00 KiB" {
		t.Fatalf("humanBytes edge wrong: %s / %s", humanBytes(500), humanBytes(2048))
	}
}

func TestParseJSON(t *testing.T) {
	ecx := &workflow.Context{Content: `{"name":"x","items":[{"t":"a"},{"t":"b"}],"nested":{"k":42}}`}
	n := NewParseJSONNode()
	fields := map[string]any{
		"name":  "name",
		"item0": "items.0.t",
		"miss":  "nope",
		"num":   "nested.k",
	}
	if err := n.Execute(context.Background(), map[string]any{"fields": fields}, ecx); err != nil {
		t.Fatal(err)
	}
	if ecx.Variables["name"] != "x" {
		t.Fatalf("name=%v", ecx.Variables["name"])
	}
	if ecx.Variables["item0"] != "a" {
		t.Fatalf("item0=%v", ecx.Variables["item0"])
	}
	if _, ok := ecx.Variables["miss"]; ok {
		t.Fatal("miss should be absent")
	}
	if ecx.Variables["num"] == nil {
		t.Fatal("num missing")
	}
}

func TestRegexExtract(t *testing.T) {
	ecx := &workflow.Context{Content: "grab https://a/x.torrent and https://b/y.torrent ok"}
	n := NewRegexExtractNode()
	if err := n.Execute(context.Background(), map[string]any{
		"pattern":    `(https?://\S+\.torrent)`,
		"output_var": "urls",
	}, ecx); err != nil {
		t.Fatal(err)
	}
	got := ecx.Variables["urls"].(string)
	if strings.Count(got, "\n")+1 != 2 {
		t.Fatalf("expected 2 urls, got %q", got)
	}
	if ecx.Variables["urls_count"] != 2 {
		t.Fatalf("count=%v", ecx.Variables["urls_count"])
	}
}

func TestResolveHashes(t *testing.T) {
	// 显式 hashes（含空白/换行/模板渲染）
	ecx := &workflow.Context{Variables: map[string]any{"h": "zzz"}}
	got := resolveHashes(map[string]any{"hashes": "aa, bb\n cc {{var.h}}"}, ecx)
	want := []string{"aa", "bb", "cc", "zzz"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("idx %d got %v want %v", i, got, want)
		}
	}
	// 空 hashes 回退 torrent_hashes 变量
	ecx2 := &workflow.Context{Variables: map[string]any{"torrent_hashes": "h1,h2"}}
	got2 := resolveHashes(map[string]any{"hashes": ""}, ecx2)
	if len(got2) != 2 || got2[0] != "h1" || got2[1] != "h2" {
		t.Fatalf("fallback wrong: %v", got2)
	}
	// 全空
	if got3 := resolveHashes(map[string]any{"hashes": ""}, &workflow.Context{}); len(got3) != 0 {
		t.Fatalf("expected empty, got %v", got3)
	}
}

func TestNormalizeTitleAndScore(t *testing.T) {
	if normalizeTitle("Movie.Name.2024") != normalizeTitle("movie name 2024") {
		t.Fatal("normalizeTitle 不一致")
	}
	if resolutionRank("2160p") != 4 || resolutionRank("1080p") != 3 || resolutionRank("720p") != 2 {
		t.Fatal("resolutionRank 错误")
	}
	free := torrent("X 1080p", 0, true, "")
	notFree := torrent("X 2160p", 9999, false, "")
	if torrentScore(free) <= torrentScore(notFree) {
		t.Fatal("免费分值应高于非免费")
	}
}
