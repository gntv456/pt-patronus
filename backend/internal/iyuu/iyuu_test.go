// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package iyuu

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 起 IYUU 模拟服务：按 ?service= 分发 App.Api.Hash / App.Api.Sites 的钉死响应，
// 并校验请求 multipart 表单含 sign(=token) 与 hash(JSON 数组)。
func newFakeIYUU(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.FormValue("sign"); got != "TOKEN" {
			t.Errorf("sign field = %q, want TOKEN", got)
		}
		switch r.URL.Query().Get("service") {
		case "App.Api.Hash":
			if h := r.FormValue("hash"); !strings.Contains(h, "H1") {
				t.Errorf("hash field = %q, want JSON array containing H1", h)
			}
			io.WriteString(w, `{"ret":200,"data":[{"hash":"H1","torrent":[{"sid":7,"torrent_id":123,"info_hash":"IH123"}]}],"msg":""}`)
		case "App.Api.Sites":
			io.WriteString(w, `{"ret":200,"data":{"sites":[
				{"id":7,"site":"m-team","base_url":"xp.m-team.cc","download_page":"download.php?id={}","is_https":1},
				{"id":11,"site":"ourbits","base_url":"ourbits.org","download_page":"download.php?id={}","is_https":1}
			]},"msg":""}`)
		default:
			http.Error(w, "unknown service", 400)
		}
	}))
}

func TestHashIndex(t *testing.T) {
	ts := newFakeIYUU(t)
	defer ts.Close()
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	c := New(hc, ts.URL)

	got, err := c.HashIndex(context.Background(), "TOKEN", []string{"H1"})
	if err != nil {
		t.Fatalf("HashIndex: %v", err)
	}
	if len(got) != 1 || got[0].Hash != "H1" {
		t.Fatalf("got %+v", got)
	}
	if len(got[0].Torrents) != 1 || got[0].Torrents[0].Sid != 7 || got[0].Torrents[0].TorrentID != 123 {
		t.Errorf("match parsed wrong: %+v", got[0].Torrents)
	}

	// 空 hashes 不发请求。
	if got, err := c.HashIndex(context.Background(), "TOKEN", nil); err != nil || got != nil {
		t.Errorf("empty hashes: got=%v err=%v", got, err)
	}
	// 空 token 报错。
	if _, err := c.HashIndex(context.Background(), "", []string{"H1"}); err == nil {
		t.Error("empty token should error")
	}
}

func TestSites(t *testing.T) {
	ts := newFakeIYUU(t)
	defer ts.Close()
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	c := New(hc, ts.URL)

	sites, err := c.Sites(context.Background(), "TOKEN")
	if err != nil {
		t.Fatalf("Sites: %v", err)
	}
	if len(sites) != 2 || sites[0].ID != 7 || sites[0].Site != "m-team" {
		t.Errorf("sites parsed wrong: %+v", sites)
	}
}

func TestBuildDownloadURL(t *testing.T) {
	cases := []struct {
		name string
		site SiteInfo
		want string
	}{
		{
			name: "default nexusphp https",
			site: SiteInfo{Site: "ourbits", BaseURL: "ourbits.org", DownloadPage: "download.php?id={}", IsHTTPS: 1},
			want: "https://ourbits.org/download.php?id=5&passkey=pk",
		},
		{
			name: "m-team",
			site: SiteInfo{Site: "m-team", BaseURL: "xp.m-team.cc", DownloadPage: "download.php?id={}", IsHTTPS: 1},
			want: "https://xp.m-team.cc/download.php?id=5&passkey=pk&https=1",
		},
		{
			name: "ttg",
			site: SiteInfo{Site: "ttg", BaseURL: "totheglory.im", DownloadPage: "dl.php?id={}", IsHTTPS: 1},
			want: "https://totheglory.im/dl.php?id=5/pk",
		},
		{
			name: "ccfbits passkey placeholder",
			site: SiteInfo{Site: "ccfbits", BaseURL: "ccfbits.org", DownloadPage: "download.php?torrent={}&passkey={passkey}", IsHTTPS: 1},
			want: "https://ccfbits.org/download.php?torrent=5&passkey=pk",
		},
		{
			name: "http scheme",
			site: SiteInfo{Site: "somesite", BaseURL: "x.org", DownloadPage: "dl.php?id={}", IsHTTPS: 0},
			want: "http://x.org/dl.php?id=5&passkey=pk",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildDownloadURL(tc.site, 5, "pk"); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
