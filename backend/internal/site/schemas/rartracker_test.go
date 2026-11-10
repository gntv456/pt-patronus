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

func TestGenericHTMLSuperbitsDefinitionSearchAndUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/torrents":
			if r.URL.Query().Get("searchText") != "movie" {
				t.Fatalf("searchText = %q, want movie", r.URL.Query().Get("searchText"))
			}
			if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("section") != "all" {
				t.Fatalf("query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{
				"id": 123,
				"name": "Superbits Movie",
				"size": 2147483648,
				"seeders": 8,
				"leechers": 1,
				"timesCompleted": 22,
				"category": "Movie",
				"added": "2024-01-02T03:04:05Z"
			}]`))
		case "/api/v1/status":
			_, _ = w.Write([]byte(`{"user":{
				"id": 42,
				"username": "alice",
				"uploaded": "2.00 TiB",
				"downloaded": "512.00 GiB",
				"bonuspoang": 12345.6,
				"myseeds": 7,
				"currentGbSeed": 3072,
				"class": 2
			}}`))
		case "/api/v1/users/42":
			_, _ = w.Write([]byte(`{
				"added": "2024-01-02T03:04:05Z",
				"uploaded": "2.00 TiB",
				"downloaded": "512.00 GiB",
				"bonuspoang": 12345.6,
				"peersSeeder": 7
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("superbits")
	if !ok {
		t.Fatal("superbits definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "rartracker")

	ts, err := g.Search(context.Background(), def, srv.URL, "cookie", "movie", "", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ts) != 1 {
		t.Fatalf("Search returned %d torrents, want 1", len(ts))
	}
	if ts[0].Title != "Superbits Movie" || ts[0].Category != "Movie" {
		t.Fatalf("torrent title/category = %q/%q", ts[0].Title, ts[0].Category)
	}
	if ts[0].DetailURL != srv.URL+"/torrent/123" {
		t.Fatalf("detail = %q", ts[0].DetailURL)
	}
	if ts[0].Size != "2.00 GiB" || ts[0].Seeders != 8 || ts[0].Leechers != 1 || ts[0].Completed != 22 {
		t.Fatalf("torrent stats = size %q seeders %d leechers %d completed %d", ts[0].Size, ts[0].Seeders, ts[0].Leechers, ts[0].Completed)
	}

	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "alice" || u.Level != "Trogen medlem" || u.LevelID != 2 || u.JoinDate != "2024-01-02" {
		t.Fatalf("identity = id %q name %q level %q level_id %d join %q", u.ID, u.Name, u.Level, u.LevelID, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Bonus != 12345.6 || u.Seeding != 7 {
		t.Fatalf("stats = bonus %g seeding %d", u.Bonus, u.Seeding)
	}
}
