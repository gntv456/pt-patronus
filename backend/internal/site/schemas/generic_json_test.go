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

// TestGenericJSONSearch：canned JSON（zhuque 形 rows=data.torrents）→ ParseJSONSearch。
func TestGenericJSONSearch(t *testing.T) {
	var gotParam string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotParam = r.URL.Query().Get("keyword")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"torrents":[{"id":1,"title":"T","size":2000,"seeding":3,"leeching":1,"complete":9}]}}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{
		Schema: "tnode", SearchEntry: "/api/torrent/advancedSearch", SearchParam: "keyword",
		Selectors: site.SiteSelectors{
			Rows: site.ElementQuery{Selector: "data.torrents"},
			Fields: map[string]site.ElementQuery{
				"title":   {Selector: "title"},
				"size":    {Selector: "size"},
				"seeders": {Selector: "seeding"},
			},
		},
	}
	ts, err := g.Search(context.Background(), def, srv.URL, "", "kw", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if gotParam != "kw" {
		t.Errorf("keyword param = %q", gotParam)
	}
	if len(ts) != 1 || ts[0].Title != "T" || ts[0].Size != "1.95 KiB" || ts[0].Seeders != 3 {
		t.Errorf("解析错: %+v", ts)
	}
}

func TestGenericJSONSearchYemaPTUsesPostBody(t *testing.T) {
	var gotMethod string
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/torrent/fetchOpenTorrentList" {
			t.Fatalf("unexpected path %s", r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":9,"showName":"Y","seedNum":5,"leechNum":1,"fileSize":1024,"listingTime":"2026-01-02T03:04:05.000+08:00"}]}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "yemapt")
	def := &site.SiteMetadata{
		ID: "yemapt", Schema: "yemapt", SearchEntry: "/api/torrent/fetchOpenTorrentList", SearchParam: "keyword",
		Selectors: site.SiteSelectors{
			Rows: site.ElementQuery{Selector: "data"},
			Fields: map[string]site.ElementQuery{
				"title":    {Selector: "showName"},
				"seeders":  {Selector: "seedNum"},
				"leechers": {Selector: "leechNum"},
				"size":     {Selector: "fileSize"},
			},
		},
	}
	ts, err := g.Search(context.Background(), def, srv.URL, "token-1", "abc", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s, want POST", gotMethod)
	}
	if gotAuth != "token-1" {
		t.Fatalf("authorization = %q, want token-1", gotAuth)
	}
	if gotBody["keyword"] != "abc" {
		t.Fatalf("keyword body = %#v", gotBody["keyword"])
	}
	pageParam, _ := gotBody["pageParam"].(map[string]any)
	if pageParam["current"].(float64) != 2 || pageParam["pageSize"].(float64) != 40 {
		t.Fatalf("pageParam = %#v", pageParam)
	}
	if len(ts) != 1 || ts[0].Title != "Y" || ts[0].Size != "1.00 KiB" || ts[0].Seeders != 5 {
		t.Fatalf("parsed torrents = %+v", ts)
	}
}
