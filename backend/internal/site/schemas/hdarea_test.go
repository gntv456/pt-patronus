// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestNexusPHPFetchHDAreaUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`
<div id="info_block">
  <a href="userdetails.php?id=42">area-user</a>
  <span>魔力值: 12,345.6</span>
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "42" {
				t.Fatalf("detail id = %q, want 42", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Elite User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2024-06-07 08:09:10</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量: 1.50 TiB 下载量: 512 GiB 分享率: 3.5</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>最终每小时获取 88.5</div><div>做种总积 <b>3.00 GiB</b></div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "42" {
				t.Fatalf("ajax userid = %q, want 42", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div id="ajax_data_meta" data-count="7"></div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div id="ajax_data_meta" data-count="4"></div>`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("hdarea")
	if !ok {
		t.Fatal("hdarea definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)
	u, err := n.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "area-user" || u.Level != "Elite User" || u.JoinDate != "2024-06-07" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if want := int64(1536 * (1 << 30)); u.Uploaded != want {
		t.Fatalf("uploaded = %d, want %d", u.Uploaded, want)
	}
	if want := int64(512 * (1 << 30)); u.Downloaded != want {
		t.Fatalf("downloaded = %d, want %d", u.Downloaded, want)
	}
	if u.Ratio != 3.5 || u.Bonus != 12345.6 || u.BonusPerHour != 88.5 || u.Seeding != 7 || u.Uploads != 4 {
		t.Fatalf("stats = ratio %g bonus %g bph %g seeding %d uploads %d", u.Ratio, u.Bonus, u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(3 * (1 << 30)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestParseHDAreaSearchSubtitleSibling(t *testing.T) {
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("hdarea")
	if !ok {
		t.Fatal("hdarea definition missing")
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
<table class="torrents"><tbody>
  <tr>
    <td><table class="torrentname"><tbody><tr><td>
      <div><a href="details.php?id=9" title="Movie"><b>Movie Title</b></a></div>
      <div>2160p UHD Blu-ray</div>
    </td></tr></tbody></table></td>
    <td>2026-01-02</td><td>1.00 GiB</td><td><a href="#seeders">8</a></td><td><a href="#leechers">1</a></td><td><a href="download.php?id=9">DL</a></td>
  </tr>
</tbody></table>`))
	if err != nil {
		t.Fatalf("parse doc: %v", err)
	}
	ts := site.ParseSearch(doc, site.EnsureDefaults(def), "https://hdarea.club")
	if len(ts) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(ts), ts)
	}
	if ts[0].Title != "Movie Title" || ts[0].SubTitle != "2160p UHD Blu-ray" {
		t.Fatalf("title/subtitle = %q/%q", ts[0].Title, ts[0].SubTitle)
	}
}
