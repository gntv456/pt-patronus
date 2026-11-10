// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestGenericJSONFetchUserInfoPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":7,"name":"jsoner","levelName":"Lv2","uploaded":1099511627776,"downloaded":536870912,"bonus":12.5,"bonusPerHour":6.25,"joinTime":1704067200,"seeding":3,"seedingSize":2147483648,"uploadCount":4}}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	u, err := g.FetchUserInfo(context.Background(), &site.SiteMetadata{
		ID:     "json",
		Schema: "tnode",
		UserInfo: site.UserInfoDef{Pages: []site.UserInfoPageDef{{
			Path: "/api/me",
			Fields: map[string]site.ElementQuery{
				"id":             {Selector: "data.id"},
				"name":           {Selector: "data.name"},
				"level":          {Selector: "data.levelName"},
				"uploaded":       {Selector: "data.uploaded"},
				"downloaded":     {Selector: "data.downloaded"},
				"bonus":          {Selector: "data.bonus"},
				"bonus_per_hour": {Selector: "data.bonusPerHour"},
				"join_date":      {Selector: "data.joinTime", Filters: []site.Filter{{Name: "unix_seconds"}}},
				"seeding":        {Selector: "data.seeding"},
				"seeding_size":   {Selector: "data.seedingSize"},
				"uploads":        {Selector: "data.uploadCount"},
			},
		}}},
	}, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "7" || u.Name != "jsoner" || u.Level != "Lv2" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if u.JoinDate != "2024-01-01" {
		t.Fatalf("join_date = %q, want 2024-01-01", u.JoinDate)
	}
	if u.Uploaded != 1099511627776 || u.Downloaded != 536870912 || u.SeedingSize != 2147483648 {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Seeding != 3 || u.Uploads != 4 || u.Bonus != 12.5 {
		t.Fatalf("stats = seeding %d uploads %d bonus %g", u.Seeding, u.Uploads, u.Bonus)
	}
	if u.BonusPerHour != 6.25 {
		t.Fatalf("bonus_per_hour = %g, want 6.25", u.BonusPerHour)
	}
}

func TestGenericJSONFetchUserInfoPagePostBody(t *testing.T) {
	var gotMethod string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		gotBody = buf.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":6}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "yemapt")
	u, err := g.FetchUserInfo(context.Background(), &site.SiteMetadata{
		ID: "yemapt", Schema: "yemapt",
		UserInfo: site.UserInfoDef{Pages: []site.UserInfoPageDef{{
			Path:   "/api/torrent/fetchSelfTorrentCount",
			Method: "POST",
			Body:   "{}",
			Format: "json",
			Fields: map[string]site.ElementQuery{"uploads": {Selector: "data"}},
		}}},
	}, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if gotMethod != http.MethodPost || gotBody != "{}" {
		t.Fatalf("request = %s %q, want POST {}", gotMethod, gotBody)
	}
	if u.Uploads != 6 {
		t.Fatalf("uploads = %d, want 6", u.Uploads)
	}
}

func TestGenericJSONFetchYemaPTUserInfoFromMigratedDefinition(t *testing.T) {
	seen := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token-1" {
			t.Fatalf("%s authorization = %q, want token-1", r.URL.Path, got)
		}
		seen[r.URL.Path] = true
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/consumer/fetchSelfDetail":
			if r.Method != http.MethodGet {
				t.Fatalf("self detail method = %s, want GET", r.Method)
			}
			_, _ = w.Write([]byte(`{"data":{"id":42,"name":"yema-user","level":4,"registerTime":"2024-02-03T04:05:06.000+08:00","promotionUploadSize":"2.00 TiB","promotionDownloadSize":"512 GiB","uploadSize":"1.50 TiB","downloadSize":"384 GiB","bonus":123.5}}`))
		case "/api/torrent/fetchSelfTorrentCount":
			assertJSONPostBody(t, r, "{}")
			_, _ = w.Write([]byte(`{"data":7}`))
		case "/api/userTorrent/fetchSeedTorrentInfo":
			assertJSONPostBody(t, r, `{"pageNum":1,"pageSize":1}`)
			_, _ = w.Write([]byte(`{"data":{"num":9,"fileSize":"3.25 TiB"}}`))
		case "/api/consumer/fetchUserPointAcc":
			if r.Method != http.MethodGet {
				t.Fatalf("point method = %s, want GET", r.Method)
			}
			_, _ = w.Write([]byte(`{"data":{"hourPoint":3.25,"hourBasePoint":1.5,"hourOwnerPoint":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("yemapt")
	if !ok {
		t.Fatal("yemapt definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := NewGenericJSON(hc, "yemapt").FetchUserInfo(context.Background(), def, srv.URL, "token-1")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	for _, path := range []string{
		"/api/consumer/fetchSelfDetail",
		"/api/torrent/fetchSelfTorrentCount",
		"/api/userTorrent/fetchSeedTorrentInfo",
		"/api/consumer/fetchUserPointAcc",
	} {
		if !seen[path] {
			t.Fatalf("endpoint %s was not requested", path)
		}
	}
	if u.ID != "42" || u.Name != "yema-user" || u.Level != "修职郎" || u.LevelID != 4 || u.JoinDate != "2024-02-03" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != int64(3.25*(1<<40)) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.TrueUploaded != int64(1.5*(1<<40)) || u.TrueDownloaded != 384*(1<<30) {
		t.Fatalf("true sizes = uploaded %d downloaded %d", u.TrueUploaded, u.TrueDownloaded)
	}
	if u.Uploads != 7 || u.Seeding != 9 || u.Bonus != 123.5 || u.BonusPerHour != 6.75 {
		t.Fatalf("stats = uploads %d seeding %d bonus %g bph %g", u.Uploads, u.Seeding, u.Bonus, u.BonusPerHour)
	}
}

func assertJSONPostBody(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Fatalf("%s method = %s, want POST", r.URL.Path, r.Method)
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r.Body)
	got := strings.ReplaceAll(buf.String(), " ", "")
	if got != want {
		t.Fatalf("%s body = %q, want %q", r.URL.Path, got, want)
	}
}
