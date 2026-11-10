// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// MTeam 详情端点字段名按 API 约定（待真实校准）；fixture 验证映射 + sn 抽取。
func TestMTeamFetchTorrentDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/torrent/detail" {
			t.Errorf("detail path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","data":{
			"id":"12345","name":"Test Anime 2024","smallDescr":"1080p BDRip",
			"description":"<p>简介</p>","category":"2","size":5368709120,
			"status":{"seeders":"8","leechers":"3"},
			"mediaInfo":"General\nComplete name: x.mkv"
		}}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	m := NewMTeam(hc)
	// detailURL 末尾 id=12345 由 mtSNFromDetailURL 抽取为 sn
	d, err := m.FetchTorrentDetail(context.Background(), &site.SiteMetadata{}, srv.URL, "k", "https://kp.m-team.cc/detail/12345")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Test Anime 2024" {
		t.Errorf("title = %q", d.Title)
	}
	if d.SubTitle != "1080p BDRip" {
		t.Errorf("subtitle = %q", d.SubTitle)
	}
	if d.Seeders != 8 || d.Leechers != 3 {
		t.Errorf("seeders/leechers = %d/%d", d.Seeders, d.Leechers)
	}
	if d.Size != "5.00 GiB" {
		t.Errorf("size = %q", d.Size)
	}
	if d.MediaInfo == "" {
		t.Error("expected mediainfo populated")
	}
}

// mtSNFromDetailURL 从 MTeam web 深链 / query 抽 id。
func TestMtSNFromDetailURL(t *testing.T) {
	cases := map[string]string{
		"https://kp.m-team.cc/detail/12345":     "12345", // 路径末尾 id
		"https://kp.m-team.cc/detail/12345/":    "12345", // 带尾斜杠
		"https://api.m-team.cc/detail?id=67890": "67890", // 路径无数字 → 回退 query id
		"https://api.m-team.cc/api?sn=abc":      "abc",   // 回退 query sn
		"https://kp.m-team.cc/detail/abc":       "",      // 路径非数字、无 query
	}
	for in, want := range cases {
		if got := mtSNFromDetailURL(in); got != want {
			t.Errorf("mtSNFromDetailURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// 回归保护：parseMTTorrent 现在会设置 DetailURL（原版遗漏）。
// webDetail 空 → 官方 kp.m-team.cc；站点定义 extra_param.web_detail → 站点自己的
// web 详情前缀（如 ColaPT 的 /torrents/<id>，深链尾部 id 供 mtSNFromDetailURL 抽取）。
func TestParseMTTorrentSetsDetailURL(t *testing.T) {
	tor := parseMTTorrent(map[string]any{
		"id":   "999",
		"name": "X",
		"size": 1024,
	}, "")
	if tor.DetailURL != "https://kp.m-team.cc/detail/999" {
		t.Errorf("detail_url = %q", tor.DetailURL)
	}
	if tor.ID != "999" {
		t.Errorf("id = %q", tor.ID)
	}

	custom := parseMTTorrent(map[string]any{"id": "16340", "name": "Y", "size": 2048},
		"https://www.colapt.com/torrents/")
	if custom.DetailURL != "https://www.colapt.com/torrents/16340" {
		t.Errorf("custom detail_url = %q", custom.DetailURL)
	}
}
