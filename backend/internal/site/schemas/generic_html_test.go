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

// TestGenericHTMLSearch：canned HTML 表（nyaa 形）→ ParseSearch。
func TestGenericHTMLSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<table class="torrent-list"><tbody>
		<tr><td><a href="/view/123">Movie 2024</a></td>
		    <td>1.5 GB</td><td>10</td><td>2</td>
		    <td><a href="/download/123">dl</a></td></tr>
		<tr><td>header</td></tr>
		</tbody></table>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "")
	def := &site.SiteMetadata{
		Schema: "", SearchEntry: "torrents.php", SearchParam: "search",
		Selectors: site.SiteSelectors{
			Rows: site.ElementQuery{Selector: "table.torrent-list > tbody > tr"},
			Fields: map[string]site.ElementQuery{
				"title":    {Selector: `a[href*="/view/"]`},
				"detail":   {Selector: `a[href*="/view/"]`, Attr: "href"},
				"download": {Selector: `a[href*="/download/"]`, Attr: "href"},
				"size":     {Selector: "td:nth-child(2)"},
				"seeders":  {Selector: "td:nth-child(3)"},
			},
		},
	}
	ts, err := g.Search(context.Background(), def, srv.URL, "", "test", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent, got %d", len(ts))
	}
	if ts[0].Title != "Movie 2024" || ts[0].Seeders != 10 {
		t.Errorf("unexpected: %+v", ts[0])
	}
	if ts[0].DownloadURL != srv.URL+"/download/123" {
		t.Errorf("download url = %q", ts[0].DownloadURL)
	}
}

// TestGenericHTMLJSONSniff：schema "" + Format="" + JSON body → 嗅探走 ParseJSONSearch。
func TestGenericHTMLJSONSniff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":1,"file_name":"X","size":1000,"seeders":5,"leechers":1}]}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "")
	def := &site.SiteMetadata{
		Schema: "", SearchEntry: "/api/torrents", SearchParam: "search",
		Selectors: site.SiteSelectors{
			Rows: site.ElementQuery{Selector: "data"},
			Fields: map[string]site.ElementQuery{
				"title":   {Selector: "file_name"},
				"seeders": {Selector: "seeders"},
			},
		},
	}
	ts, err := g.Search(context.Background(), def, srv.URL, "", "test", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 || ts[0].Title != "X" || ts[0].Seeders != 5 {
		t.Errorf("JSON 嗅探解析错: %+v", ts)
	}
}

// TestGenericHTMLUserInfoStub：schema "" → stub 不报错（不抓 EnsureDefaults 注入的 usercp.php）。
func TestGenericHTMLUserInfoStub(t *testing.T) {
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "")
	ui, err := g.FetchUserInfo(context.Background(), &site.SiteMetadata{Schema: ""}, "https://x.com", "")
	if err != nil {
		t.Errorf("不应报错: %v", err)
	}
	if ui == nil {
		t.Errorf("应返 stub UserInfo")
	}
}
