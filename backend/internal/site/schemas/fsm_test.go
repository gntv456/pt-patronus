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

func TestGenericJSONFSMDefinitionSearchAndUserInfo(t *testing.T) {
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("fsm")
	if !ok {
		t.Fatal("fsm definition missing")
	}
	cred := `{"token":"token-1","passkey":"passkey-1","cookie":"sid=1"}`
	srv := fsmMockServer(t)
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "fsm")
	ts, err := g.Search(context.Background(), def, srv.URL, cred, "movie", "", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ts) != 1 {
		t.Fatalf("Search returned %d torrents, want 1", len(ts))
	}
	got := ts[0]
	if got.Title != "FSM Movie" || got.Category != "Adult" {
		t.Fatalf("torrent title/category = %q/%q", got.Title, got.Category)
	}
	if got.DetailURL != srv.URL+"/Torrents/details?tid=123" {
		t.Fatalf("detail = %q", got.DetailURL)
	}
	if got.Size != "2.00 GiB" || got.Seeders != 8 || got.Leechers != 2 || got.Completed != 30 {
		t.Fatalf("torrent stats = size %q seeders %d leechers %d completed %d", got.Size, got.Seeders, got.Leechers, got.Completed)
	}

	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, cred)
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "alice" || u.Level != "注册成员" || u.LevelID != 2 || u.JoinDate != "2024-01-02" {
		t.Fatalf("identity = id %q name %q level %q level_id %d join %q", u.ID, u.Name, u.Level, u.LevelID, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) || u.SeedingSize != 3*(1<<40) {
		t.Fatalf("sizes = uploaded %d downloaded %d seeding_size %d", u.Uploaded, u.Downloaded, u.SeedingSize)
	}
	if u.Bonus != 12345.6 || u.Seeding != 7 || u.Uploads != 12 {
		t.Fatalf("stats = bonus %g seeding %d uploads %d", u.Bonus, u.Seeding, u.Uploads)
	}
}

func fsmMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("APITOKEN") != "token-1" {
			t.Fatalf("%s APITOKEN = %q, want token-1", r.URL.Path, r.Header.Get("APITOKEN"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/Torrents/listTorrents":
			if r.URL.Query().Get("keyword") != "movie" {
				t.Fatalf("keyword = %q", r.URL.Query().Get("keyword"))
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"list":[{
				"tid":123,
				"title":"FSM Movie",
				"fileRawSize":2147483648,
				"createdTs":1704153600,
				"peers":{"upload":8,"download":2},
				"finish":30,
				"type":{"name":"Adult"},
				"tags":["tag-a","tag-b"]
			}]}}`))
		case "/Users/infos":
			_, _ = w.Write([]byte(`{"success":true,"data":{"uid":42,"username":"alice"}}`))
		case "/Users/profile":
			if r.URL.Query().Get("uid") != "42" {
				t.Fatalf("uid = %q", r.URL.Query().Get("uid"))
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{
				"createdTs":1704153600,
				"upload":2199023255552,
				"download":549755813888,
				"userRank":{"id":2,"name":"注册成员"},
				"point":12345.6,
				"nowActiveUpload":7,
				"nowActiveUploadSize":3298534883328,
				"torrent":12
			}}`))
		default:
			http.NotFound(w, r)
		}
	}))
}
