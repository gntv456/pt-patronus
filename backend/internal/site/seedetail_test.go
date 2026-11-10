// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"strings"
	"testing"
)

// 代表性 details.php 页面 fixture（DOM 按 DefaultNexusPHPDetail 选择器构造）。
// 接真实站点后用 POST /api/v1/sites/inspect 对真实详情页校准选择器，再钉值。
const detailFixture = `<html><body>
  <h1><a href="details.php?id=100" title="Movie 2024 1080p BluRay x265">Movie</a></h1>
  <div class="torrent-info"><img src="pic/cover.jpg"></div>
  <div id="kdescr">
    <p>一部很棒的电影。剧情简介……</p>
    <img src="pic/shot1.jpg" class="bbcode2">
    <img src="pic/shot2.jpg" class="bbcode2">
    <a href="https://www.imdb.com/title/tt1234567/">IMDB</a>
    <a href="https://movie.douban.com/subject/1292052/">豆瓣</a>
  </div>
  <a href="download.php?id=100&amp;passkey=xx">下载</a>
</body></html>`

func TestParseTorrentDetail(t *testing.T) {
	doc := docFrom(t, detailFixture)
	// EnsureDefaults 填入 DefaultNexusPHPDetail（模拟适配器/链的实际调用路径）
	def := EnsureDefaults(&SiteMetadata{})

	const detailURL = "https://demo.pt/details.php?id=100"
	d := ParseTorrentDetail(doc, def, "https://demo.pt", detailURL)
	if d == nil {
		t.Fatal("nil detail")
	}
	if d.Title != "Movie 2024 1080p BluRay x265" {
		t.Errorf("title = %q", d.Title)
	}
	if d.DetailURL != detailURL {
		t.Errorf("detail_url = %q", d.DetailURL)
	}
	if d.IMDBID != "tt1234567" {
		t.Errorf("imdb_id = %q", d.IMDBID)
	}
	if d.DoubanID != "1292052" {
		t.Errorf("douban_id = %q", d.DoubanID)
	}
	if d.DownloadURL != "https://demo.pt/download.php?id=100&passkey=xx" {
		t.Errorf("download_url = %q", d.DownloadURL)
	}
	if d.PosterURL != "https://demo.pt/pic/cover.jpg" {
		t.Errorf("poster_url = %q", d.PosterURL)
	}
	if len(d.Screenshots) != 2 ||
		d.Screenshots[0] != "https://demo.pt/pic/shot1.jpg" ||
		d.Screenshots[1] != "https://demo.pt/pic/shot2.jpg" {
		t.Errorf("screenshots = %v", d.Screenshots)
	}
	if !strings.Contains(d.Description, "剧情简介") {
		t.Errorf("description missing body: %q", d.Description)
	}
}

// 无选择器/空 def 时返回 nil（健壮性，不 panic）。
func TestParseTorrentDetailNilSafe(t *testing.T) {
	if got := ParseTorrentDetail(nil, &SiteMetadata{}, "https://x", ""); got != nil {
		t.Errorf("nil doc should return nil, got %+v", got)
	}
}
