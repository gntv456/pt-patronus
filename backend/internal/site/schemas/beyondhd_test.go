// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestGenericJSONBeyondHDDefinitionSearchAndUserInfo(t *testing.T) {
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("beyondhd")
	if !ok {
		t.Fatal("beyondhd definition missing")
	}
	cred := `{"api_key":"api-1","rsskey":"rss-1","cookie":"sid=1"}`
	srv := beyondHDMockServer(t)
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "f3nix")
	ts, err := g.Search(context.Background(), def, srv.URL, cred, "movie", "", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ts) != 1 {
		t.Fatalf("Search returned %d torrents, want 1", len(ts))
	}
	got := ts[0]
	if got.Title != "Beyond Movie" || got.Category != "Movies" {
		t.Fatalf("torrent title/category = %q/%q", got.Title, got.Category)
	}
	if got.DetailURL != srv.URL+"/torrents/beyond-movie.123" {
		t.Fatalf("detail = %q", got.DetailURL)
	}
	if got.DownloadURL != srv.URL+"/download/beyond-movie.123" {
		t.Fatalf("download = %q", got.DownloadURL)
	}
	if got.Size != "2.00 GiB" || got.Seeders != 11 || got.Leechers != 3 || got.Completed != 44 {
		t.Fatalf("torrent stats = size %q seeders %d leechers %d completed %d", got.Size, got.Seeders, got.Leechers, got.Completed)
	}

	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, cred)
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "alice" || u.Level != "Master" || u.JoinDate != "2024-01-02" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Ratio != 4 || u.Bonus != 9876.5 || u.Seeding != 7 || u.Leeching != 3 || u.Uploads != 12 || u.BonusPerHour != 12.5 {
		t.Fatalf("stats = ratio %g bonus %g seeding %d leeching %d uploads %d bph %g", u.Ratio, u.Bonus, u.Seeding, u.Leeching, u.Uploads, u.BonusPerHour)
	}
}

func beyondHDMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/torrents/api-1":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["action"] != "search" || body["search"] != "movie" || body["rsskey"] != "rss-1" || body["page"].(float64) != 2 {
				t.Fatalf("body = %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"status_code":1,"results":[{
				"id":123,
				"name":"Beyond Movie",
				"size":2147483648,
				"category":"Movies",
				"seeders":11,
				"leechers":3,
				"times_completed":44,
				"created_at":"2024-01-02T03:04:05Z",
				"url":"/torrents/beyond-movie.123",
				"download_url":null
			}]}`))
		case "/":
			assertCookie(t, r)
			writeHTML(w, `<meta name="secret-id" content="42">
<img class="beta-image-avatar" title="alice">
<ul id="beta-stats">
  <li><a><i class="fas fa-arrow-up"></i> 2.00 TiB</a></li>
  <li><a><i class="fas fa-arrow-down"></i> 512.00 GiB</a></li>
  <li><a><span><i class="fas fa-sync-alt"></i></span> 4.0</a></li>
  <li><a><i class="fas fa-star"></i> 9,876.5</a></li>
  <li><a><i class="fas fa-seedling"></i> 7</a></li>
</ul>`)
		case "/users/alice.42":
			assertCookie(t, r)
			writeHTML(w, `<div style="margin-left: 1rem"><a class="beta-alert" title="Master"></a></div>
<div class="button-right">
  <div class="bhd-block"><span class="class-highlight">12</span></div>
  <div class="bhd-block"><span class="bhd-tidbit-icon">3</span></div>
</div>
<div class="button-holder"><h5>Member Since: 2024-01-02</h5></div>
<table><tr><td class="bhd-user-left">Active Seed Size</td><td><span class="badge-user">3.00 TiB</span></td></tr></table>`)
		case "/bp":
			assertCookie(t, r)
			writeHTML(w, `<div class="panel-body no-padding"><div class="hd-table"><div><span class="class-bold">12.5</span></div></div></div>`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func assertCookie(t *testing.T, r *http.Request) {
	t.Helper()
	if c, err := r.Cookie("sid"); err != nil || c.Value != "1" {
		t.Fatalf("cookie sid = %#v err=%v", c, err)
	}
}
