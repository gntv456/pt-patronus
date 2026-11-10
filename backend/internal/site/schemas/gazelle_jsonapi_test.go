// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// gazelleAJAXHandler 按 action 参数返回 Gazelle ajax.php JSON。
func gazelleAJAXHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("action") {
		case "browse":
			_, _ = w.Write([]byte(`{"status":"success","response":{"results":[
			  {"groupId":1,"groupName":"Album","artist":"Bob","groupYear":2020,"releaseType":"Album","torrents":[
			    {"torrentId":10,"size":16106127360,"seeders":5,"leechers":2,"snatches":40,"isFreeleech":true},
			    {"torrentId":11,"size":8589934592,"seeders":3,"leechers":1,"snatches":20,"isFreeleech":false}
			  ]},
			  {"groupId":2,"groupName":"Single","torrentId":20,"size":1073741824,"seeders":1,"leechers":0,"snatches":5,"category":"music"}
			]}}`))
		case "index":
			_, _ = w.Write([]byte(`{"status":"success","response":{"username":"u","id":7,"authkey":"AK","passkey":"PK",
			  "userstats":{"uploaded":1000000000000,"downloaded":500000000000,"ratio":2,"class":"VIP","bonusPoints":100,
			    "seedingBonusPointsPerHour":6.5,"seedingCount":9,"leechingCount":2,"seedingSize":3221225472,"joinedDate":"2019-12-31"}}}`))
		case "user":
			_, _ = w.Write([]byte(`{"status":"success","response":{"stats":{"joinedDate":"2020-01-01"},
			  "community":{"seeding":8,"uploaded":3}}}`))
		default:
			http.NotFound(w, r)
		}
	})
}

// TestGazelleJSONAPISearch：group(2 子) + flat → 3 torrent；Free 标记；下载链含 authkey/torrent_pass。
func TestGazelleJSONAPISearch(t *testing.T) {
	srv := httptest.NewServer(gazelleAJAXHandler(t))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)
	def := &site.SiteMetadata{ID: "red", Schema: "gazelle_jsonapi"}

	ts, err := g.Search(context.Background(), def, srv.URL, "", "kw", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 3 {
		t.Fatalf("expected 3 torrents (2 group + 1 flat), got %d", len(ts))
	}
	if !strings.Contains(ts[0].Title, "Album") || !strings.Contains(ts[0].Title, "Bob") {
		t.Errorf("ts[0].Title = %q, want group title 'Bob - Album ...'", ts[0].Title)
	}
	if !ts[0].Free {
		t.Errorf("ts[0] should be Free (isFreeleech)")
	}
	if ts[0].ID != "10" || ts[0].Seeders != 5 {
		t.Errorf("ts[0]: ID=%q Seeders=%d", ts[0].ID, ts[0].Seeders)
	}
	dl := ts[0].DownloadURL
	if !strings.Contains(dl, "id=10") || !strings.Contains(dl, "authkey=AK") || !strings.Contains(dl, "torrent_pass=PK") {
		t.Errorf("ts[0].DownloadURL = %q, want id=10 + authkey + torrent_pass", dl)
	}
	// flat torrent
	if ts[2].ID != "20" || ts[2].Title != "Single" {
		t.Errorf("ts[2] (flat): ID=%q Title=%q", ts[2].ID, ts[2].Title)
	}
}

// TestGazelleJSONAPIUserInfo：action=index（基础）+ action=user（扩展）。
func TestGazelleJSONAPIUserInfo(t *testing.T) {
	srv := httptest.NewServer(gazelleAJAXHandler(t))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)
	def := &site.SiteMetadata{ID: "red", Schema: "gazelle_jsonapi"}

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.Name != "u" || ui.Level != "VIP" {
		t.Errorf("Name/Level = %q/%q", ui.Name, ui.Level)
	}
	if ui.Uploaded != 1000000000000 {
		t.Errorf("Uploaded = %d", ui.Uploaded)
	}
	if ui.Bonus != 100 || ui.BonusPerHour != 6.5 {
		t.Errorf("Bonus/BonusPerHour = %v/%v", ui.Bonus, ui.BonusPerHour)
	}
	if !strings.Contains(ui.JoinDate, "2019") {
		t.Errorf("JoinDate = %q", ui.JoinDate)
	}
	if ui.Seeding != 9 || ui.Leeching != 2 || ui.SeedingSize != 3221225472 || ui.Uploads != 3 {
		t.Errorf("Seeding/Leeching/SeedingSize/Uploads = %d/%d/%d/%d", ui.Seeding, ui.Leeching, ui.SeedingSize, ui.Uploads)
	}
}

func TestGazelleJSONAPIStandardDefinitionsUserInfo(t *testing.T) {
	srv := httptest.NewServer(gazelleAJAXHandler(t))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)
	for _, id := range []string{"alpharatio", "desigaane", "greatposterwall", "ianon", "oppaiti", "snakepop", "sugoimusic", "uhdbits"} {
		t.Run(id, func(t *testing.T) {
			def, ok := reg.Definition(id)
			if !ok {
				t.Fatalf("%s definition missing", id)
			}
			ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
			if err != nil {
				t.Fatalf("FetchUserInfo: %v", err)
			}
			if ui.ID != "7" || ui.Name != "u" || ui.Level != "VIP" {
				t.Fatalf("identity = id %q name %q level %q", ui.ID, ui.Name, ui.Level)
			}
			if ui.Seeding != 9 || ui.Leeching != 2 || ui.SeedingSize != 3221225472 || ui.Uploads != 3 {
				t.Fatalf("stats = seeding %d leeching %d seeding_size %d uploads %d", ui.Seeding, ui.Leeching, ui.SeedingSize, ui.Uploads)
			}
		})
	}
}

func TestGazelleJSONAPISecretCinemaDefinitionRatioless(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ajax.php" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("action") {
		case "index":
			_, _ = w.Write([]byte(`{"status":"success","response":{"username":"sc","id":71,"authkey":"AK","passkey":"PK",
			  "userstats":{"uploaded":2199023255552,"downloaded":549755813888,"ratio":99,"class":"Director",
			    "bonusPoints":12345.5,"bonusPointsPerHour":4.25,"seedingCount":7,"leechingCount":1,"seedingSize":3298534883328,
			    "joinedDate":"2024-05-06"}}}`))
		case "user":
			_, _ = w.Write([]byte(`{"status":"success","response":{"community":{"uploaded":8}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("secretcinema")
	if !ok {
		t.Fatal("secretcinema definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.ID != "71" || ui.Name != "sc" || ui.Level != "Director" || ui.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", ui.ID, ui.Name, ui.Level, ui.JoinDate)
	}
	if ui.Uploaded != 2*(1<<40) || ui.Downloaded != 512*(1<<30) || ui.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", ui.Uploaded, ui.Downloaded, ui.SeedingSize)
	}
	if ui.Ratio != 0 || ui.Seeding != 7 || ui.Leeching != 1 || ui.Uploads != 8 || ui.Bonus != 12345.5 || ui.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d leeching %d uploads %d bonus %g bph %g", ui.Ratio, ui.Seeding, ui.Leeching, ui.Uploads, ui.Bonus, ui.BonusPerHour)
	}
}

func TestGazelleJSONAPIGreatPosterWallDefinitionFieldsOverrideInfiniteRatio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ajax.php" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("action") {
		case "index":
			_, _ = w.Write([]byte(`{"status":"success","response":{"username":"gpw","id":72,"authkey":"AK","passkey":"PK",
			  "userstats":{"uploaded":2199023255552,"downloaded":0,"ratio":"-1","class":"Power User",
			    "bonusPoints":12345.5,"seedingBonusPointsPerHour":4.25,"seedingCount":7,"leechingCount":1,
			    "seedingSize":3298534883328,"joinedDate":"2024-05-06"}}}`))
		case "user":
			if r.URL.Query().Get("id") != "72" {
				t.Fatalf("id = %q, want 72", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`{"status":"success","response":{"avatar":"https://img.example/gpw.png","personal":{"donor":true},"stats":{"joinedDate":"2024-05-06"},
			  "community":{"seeding":7,"uploaded":8,"groups":6,"perfectFlacs":5,"invited":4}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("greatposterwall")
	if !ok {
		t.Fatal("greatposterwall definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.ID != "72" || ui.Name != "gpw" || ui.Level != "Power User" || ui.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", ui.ID, ui.Name, ui.Level, ui.JoinDate)
	}
	if ui.Uploaded != 2*(1<<40) || ui.Downloaded != 0 || ui.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", ui.Uploaded, ui.Downloaded, ui.SeedingSize)
	}
	if ui.Ratio != 9999 || ui.Seeding != 7 || ui.Leeching != 1 || ui.Uploads != 8 || ui.Bonus != 12345.5 || ui.BonusPerHour != 4.25 {
		t.Fatalf("stats = ratio %g seeding %d leeching %d uploads %d bonus %g bph %g", ui.Ratio, ui.Seeding, ui.Leeching, ui.Uploads, ui.Bonus, ui.BonusPerHour)
	}
	if ui.UniqueGroups != 6 || ui.PerfectFLAC != 5 || ui.Invited != 4 {
		t.Fatalf("community extras = unique_groups %d perfect_flac %d invited %d", ui.UniqueGroups, ui.PerfectFLAC, ui.Invited)
	}
	if ui.Avatar != "https://img.example/gpw.png" || !ui.IsDonor {
		t.Fatalf("profile = avatar %q is_donor %v", ui.Avatar, ui.IsDonor)
	}
}

func TestGazelleJSONAPIOrpheusDefinitionBonusPageSeedingSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ajax.php":
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Query().Get("action") {
			case "index":
				_, _ = w.Write([]byte(`{"status":"success","response":{"username":"ops","id":77,"authkey":"AK","passkey":"PK",
				  "userstats":{"uploaded":1000,"downloaded":500,"ratio":2,"class":"Elite","bonusPoints":50}}}`))
			case "user":
				_, _ = w.Write([]byte(`{"status":"success","response":{"stats":{"joinedDate":"2020-01-01"},
				  "community":{"seeding":4,"uploaded":6}}}`))
			default:
				http.NotFound(w, r)
			}
		case "/bonus.php":
			if r.URL.Query().Get("action") != "bprates" {
				t.Fatalf("bonus query = %s, want action=bprates", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`<table><tbody><tr><td>Seeding size</td><td>2.00 TiB</td></tr></tbody></table>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("orpheus")
	if !ok {
		t.Fatal("orpheus definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.ID != "77" || ui.Name != "ops" || ui.Level != "Elite" {
		t.Fatalf("identity = id %q name %q level %q", ui.ID, ui.Name, ui.Level)
	}
	if ui.Seeding != 4 || ui.Uploads != 6 {
		t.Fatalf("community = seeding %d uploads %d", ui.Seeding, ui.Uploads)
	}
	if want := int64(2 * (1 << 40)); ui.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", ui.SeedingSize, want)
	}
}

func TestGazelleJSONAPIDICMusicDefinitionBonusPageStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ajax.php":
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Query().Get("action") {
			case "index":
				_, _ = w.Write([]byte(`{"status":"success","response":{"username":"dic","id":88,"authkey":"AK","passkey":"PK",
				  "userstats":{"uploaded":1000,"downloaded":500,"ratio":2,"class":"Power TM"}}}`))
			case "user":
				_, _ = w.Write([]byte(`{"status":"success","response":{"stats":{"joinedDate":"2020-01-01"},
				  "community":{"seeding":5,"uploaded":7}}}`))
			default:
				http.NotFound(w, r)
			}
		case "/bonus.php":
			if r.URL.Query().Get("action") != "bprates" {
				t.Fatalf("bonus query = %s, want action=bprates", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`<div id="content"><div class="header"><h3>Bonus: 9,876.5</h3></div></div>
			  <table id="bprates_overview"><tbody><tr><td>total</td><td>12.5 / 1.50 TiB</td></tr></tbody></table>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("dicmusic")
	if !ok {
		t.Fatal("dicmusic definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.ID != "88" || ui.Name != "dic" || ui.Level != "Power TM" {
		t.Fatalf("identity = id %q name %q level %q", ui.ID, ui.Name, ui.Level)
	}
	if ui.Bonus != 9876.5 || ui.BonusPerHour != 12.5 {
		t.Fatalf("bonus = %g bph = %g", ui.Bonus, ui.BonusPerHour)
	}
	if want := int64(1536 * (1 << 30)); ui.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", ui.SeedingSize, want)
	}
}

func TestGazelleJSONAPIRedactedDefinitionCommunityStatsSeedingSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ajax.php" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("action") {
		case "index":
			_, _ = w.Write([]byte(`{"status":"success","response":{"username":"red","id":99,"authkey":"AK","passkey":"PK",
			  "userstats":{"uploaded":1000,"downloaded":500,"ratio":2,"class":"Elite"}}}`))
		case "user":
			_, _ = w.Write([]byte(`{"status":"success","response":{"stats":{"joinedDate":"2020-01-01"},
			  "community":{"seeding":8,"uploaded":9}}}`))
		case "community_stats":
			if r.URL.Query().Get("userid") != "99" {
				t.Fatalf("userid = %q, want 99", r.URL.Query().Get("userid"))
			}
			_, _ = w.Write([]byte(`{"status":"success","response":{"seedingsize":"4.00 TiB"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("redacted")
	if !ok {
		t.Fatal("redacted definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.ID != "99" || ui.Name != "red" || ui.Level != "Elite" {
		t.Fatalf("identity = id %q name %q level %q", ui.ID, ui.Name, ui.Level)
	}
	if want := int64(4 * (1 << 40)); ui.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", ui.SeedingSize, want)
	}
}

// TestGazelleJSONAPIAuthFailure：status:failure + 鉴权类 error → ErrNotLoggedIn（改进 PT-depiler 静默返空）。
func TestGazelleJSONAPIAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failure","error":"bad credentials"}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)
	def := &site.SiteMetadata{ID: "red", Schema: "gazelle_jsonapi"}

	_, err := g.Search(context.Background(), def, srv.URL, "", "kw", "", 0)
	if err != site.ErrNotLoggedIn {
		t.Errorf("expected ErrNotLoggedIn, got %v", err)
	}
}

func TestGazelleGamesSearchAndUserInfo(t *testing.T) {
	const token = "GGN-TOKEN"
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.php" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("X-API-Key"); got != token {
			t.Fatalf("X-API-Key = %q, want %q", got, token)
		}
		w.Header().Set("Content-Type", "application/json")
		req := r.URL.Query().Get("request")
		seen[req]++
		switch req {
		case "search":
			if r.URL.Query().Get("search_type") != "torrents" ||
				r.URL.Query().Get("order_by") != "time" ||
				r.URL.Query().Get("order_way") != "desc" {
				t.Fatalf("search query = %s", r.URL.RawQuery)
			}
			if got := r.URL.Query().Get("searchstr"); got != "Half Life" {
				t.Fatalf("searchstr = %q, want Half Life", got)
			}
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Fatalf("page = %q, want 2", got)
			}
			_, _ = w.Write([]byte(`{"status":"success","response":{
			  "42":{"Artists":[{"name":"Windows"}],"Year":"2024","Torrents":{
			    "321":{"ID":"321","GroupID":"42","CategoryID":"1","Language":"English","Format":"FLAC","Encoding":"Lossless",
			      "Region":"US","Remastered":"1","RemasterYear":"2023","RemasterTitle":"Deluxe Edition","Scene":"1",
			      "ReleaseTitle":"Half-Life Complete","GameDOXType":"","Size":"2147483648","Time":"2026-07-06 12:34:56",
			      "Miscellaneous":"Bonus","FreeTorrent":"1","Seeders":17,"Leechers":3,"Snatched":99}
			  }}
			}}`))
		case "quick_user":
			_, _ = w.Write([]byte(`{"status":"success","response":{"username":"ggn","id":777,"authkey":"AUTH","passkey":"PASS",
			  "userstats":{"uploaded":1099511627776,"downloaded":536870912000,"ratio":2.5,"class":"Gamer"}}}`))
		case "user":
			if r.URL.Query().Get("id") != "777" {
				t.Fatalf("user id = %q, want 777", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`{"status":"success","response":{"stats":{"joinedDate":"2021-02-03","gold":12345.5},
			  "community":{"seeding":44,"uploaded":12,"hourlyGold":7.25,"seedSize":3298534883328}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)
	def := &site.SiteMetadata{ID: "gazellegames", Schema: "gazelle_jsonapi"}

	ts, err := g.Search(context.Background(), def, srv.URL, token, "Half.Life", "", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ts) != 1 {
		t.Fatalf("got %d torrents, want 1", len(ts))
	}
	if ts[0].ID != "321" || ts[0].Title != "Half-Life Complete" || ts[0].Category != "Windows" {
		t.Fatalf("torrent identity = id %q title %q category %q", ts[0].ID, ts[0].Title, ts[0].Category)
	}
	if !strings.Contains(ts[0].SubTitle, "2023 / Deluxe Edition / English / US / FLAC / Lossless / Bonus / Scene") {
		t.Fatalf("subtitle = %q", ts[0].SubTitle)
	}
	if ts[0].DetailURL != srv.URL+"/torrents.php?id=42&torrentid=321" {
		t.Fatalf("detail_url = %q", ts[0].DetailURL)
	}
	if !strings.Contains(ts[0].DownloadURL, "id=321") ||
		!strings.Contains(ts[0].DownloadURL, "authkey=AUTH") ||
		!strings.Contains(ts[0].DownloadURL, "torrent_pass=PASS") {
		t.Fatalf("download_url = %q", ts[0].DownloadURL)
	}
	if ts[0].Size != "2.00 GiB" || ts[0].Seeders != 17 || ts[0].Leechers != 3 || ts[0].Completed != 99 || !ts[0].Free {
		t.Fatalf("torrent stats = size %q seed/leech/done/free %d/%d/%d/%v", ts[0].Size, ts[0].Seeders, ts[0].Leechers, ts[0].Completed, ts[0].Free)
	}
	if ts[0].Time != "2026-07-06" {
		t.Fatalf("time = %q, want 2026-07-06", ts[0].Time)
	}

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, token)
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.ID != "777" || ui.Name != "ggn" || ui.Level != "Gamer" {
		t.Fatalf("identity = id %q name %q level %q", ui.ID, ui.Name, ui.Level)
	}
	if ui.Uploaded != 1099511627776 || ui.Downloaded != 536870912000 || ui.Ratio != 2.5 {
		t.Fatalf("traffic = uploaded %d downloaded %d ratio %g", ui.Uploaded, ui.Downloaded, ui.Ratio)
	}
	if ui.JoinDate != "2021-02-03" || ui.Seeding != 44 || ui.Uploads != 12 {
		t.Fatalf("activity = join %q seeding %d uploads %d", ui.JoinDate, ui.Seeding, ui.Uploads)
	}
	if ui.Bonus != 12345.5 || ui.BonusPerHour != 7.25 || ui.SeedingSize != 3298534883328 {
		t.Fatalf("bonus/seed_size = %g/%g/%d", ui.Bonus, ui.BonusPerHour, ui.SeedingSize)
	}
	if seen["search"] != 1 || seen["quick_user"] < 1 || seen["user"] != 1 {
		t.Fatalf("seen requests = %#v", seen)
	}
}
