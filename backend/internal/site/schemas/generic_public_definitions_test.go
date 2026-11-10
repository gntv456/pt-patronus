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

func TestGenericHTMLPublicDefinitionsSearch(t *testing.T) {
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "")

	for _, tc := range []struct {
		id         string
		title      string
		category   string
		detailURL  string
		size       string
		seeders    int
		leechers   int
		completed  int
		noDownload bool
	}{
		{id: "acgrip", title: "ACG Movie", detailURL: "/torrent/123", size: "2147483648", seeders: 0, leechers: 0, completed: 0},
		{id: "audiobookbay", title: "AudioBook Title", category: "Audiobook", detailURL: "/audio/123", size: "1610612736", noDownload: true},
		{id: "dmhy", title: "DMHY Anime", category: "動畫", detailURL: "/topics/view/123", size: "1.2GB", seeders: 8, leechers: 2, completed: 30},
		{id: "ehentai", title: "E-Hentai Book", detailURL: "/g/123/abc", size: "2147483648", seeders: 9, leechers: 1, completed: 42},
		{id: "gtnet", title: "GTNet Movie", category: "Movies > HD", detailURL: "/torrentdetails.php?torrentid=abc123", size: "2147483648", seeders: 13, leechers: 4},
		{id: "gtorg", title: "GTOrg Movie", category: "Movies", detailURL: "/details.php?id=abc123", size: "2147483648", seeders: 11, leechers: 3, completed: 44},
		{id: "nekobt", title: "Neko Anime", category: "Anime", detailURL: "/torrents/123", size: "2.00 GiB", seeders: 15, leechers: 2, completed: 9},
		{id: "rutracker", title: "RuTracker Movie", category: "Movies", detailURL: "/forum/viewtopic.php?t=123", size: "2147483648", seeders: 22, leechers: 5, completed: 70},
	} {
		t.Run(tc.id, func(t *testing.T) {
			def, ok := reg.Definition(tc.id)
			if !ok {
				t.Fatalf("%s definition missing", tc.id)
			}
			srv := genericPublicDefinitionSearchServer(t, tc.id)
			defer srv.Close()

			ts, err := g.Search(context.Background(), def, srv.URL, "cookie", "movie", "", 1)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(ts) != 1 {
				t.Fatalf("Search returned %d torrents, want 1: %+v", len(ts), ts)
			}
			got := ts[0]
			if got.Title != tc.title {
				t.Fatalf("title = %q, want %q", got.Title, tc.title)
			}
			if tc.category != "" && got.Category != tc.category {
				t.Fatalf("category = %q, want %q", got.Category, tc.category)
			}
			if got.DetailURL != srv.URL+tc.detailURL {
				t.Fatalf("detail = %q, want %q", got.DetailURL, srv.URL+tc.detailURL)
			}
			if tc.size != "" && got.Size != tc.size {
				t.Fatalf("size = %q, want %q", got.Size, tc.size)
			}
			if got.Seeders != tc.seeders || got.Leechers != tc.leechers || got.Completed != tc.completed {
				t.Fatalf("stats = seeders %d leechers %d completed %d", got.Seeders, got.Leechers, got.Completed)
			}
			if !tc.noDownload && got.DownloadURL == "" {
				t.Fatal("download url is empty")
			}
		})
	}
}

func TestGenericHTMLGTNetDefinitionUserInfo(t *testing.T) {
	def := mustGenericDefinition(t, "gtnet")
	srv := genericPublicDefinitionSearchServer(t, "gtnet")
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := NewGenericHTML(hc, "").FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "alice" || u.Level != "Power User" || u.JoinDate != "2024-12-31" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) {
		t.Fatalf("traffic = uploaded %d downloaded %d", u.Uploaded, u.Downloaded)
	}
	if u.Ratio != 2.5 || u.Bonus != 12345.6 {
		t.Fatalf("stats = ratio %g bonus %g", u.Ratio, u.Bonus)
	}
}

func TestGenericHTMLGTOrgDefinitionUserInfo(t *testing.T) {
	def := mustGenericDefinition(t, "gtorg")
	srv := genericPublicDefinitionSearchServer(t, "gtorg")
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := NewGenericHTML(hc, "").FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "alice" || u.Level != "Poweruser" || u.JoinDate != "2024-01-02" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) {
		t.Fatalf("traffic = uploaded %d downloaded %d", u.Uploaded, u.Downloaded)
	}
	if u.Ratio != 4 || u.Bonus != 9876.5 || u.Seeding != 7 {
		t.Fatalf("stats = ratio %g bonus %g seeding %d", u.Ratio, u.Bonus, u.Seeding)
	}
}

func mustGenericDefinition(t *testing.T, id string) *site.SiteMetadata {
	t.Helper()
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition(id)
	if !ok {
		t.Fatalf("%s definition missing", id)
	}
	return def
}

func genericPublicDefinitionSearchServer(t *testing.T, id string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch id {
		case "acgrip":
			if r.URL.Path != "/" || r.URL.Query().Get("term") != "movie" {
				t.Fatalf("acgrip request = %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			writeHTML(w, `<table class="post-index"><tbody><tr>
<td class="date"><time datetime="2024-01-02T03:04:05Z"></time></td>
<td class="title"><span class="title"><a href="/torrent/123">ACG Movie</a></span></td>
<td class="size">2.00 GiB</td>
<td class="action"><a href="/download/123.torrent">DL</a></td>
</tr></tbody></table>`)
		case "audiobookbay":
			if r.URL.Path != "/" || r.URL.Query().Get("s") != "movie" || r.URL.Query().Get("tt") != "1" {
				t.Fatalf("audiobookbay request = %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			writeHTML(w, `<div class="post">
<div class="postTitle"><h2><a href="/audio/123">AudioBook Title</a></h2></div>
<div class="postInfo">Category: Audiobook Language: English</div>
<div class="postContent">Posted: 2 Jan 2024 File Size: 1.50 GiBs</div>
</div>`)
		case "dmhy":
			if r.URL.Path != "/topics/list" || r.URL.Query().Get("keyword") != "movie" {
				t.Fatalf("dmhy request = %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			writeHTML(w, `<table><tbody><tr>
<td><span>2024/01/02 03:04</span></td>
<td><a>動畫</a></td>
<td><a href="/topics/view/123">DMHY Anime</a></td>
<td><a href="magnet:?xt=urn:btih:dmhy">Magnet</a></td>
<td>1.2GB</td><td>8</td><td>2</td><td>30</td>
</tr></tbody></table>`)
		case "ehentai":
			if r.URL.Path != "/torrents.php" || r.URL.Query().Get("search") != "movie" {
				t.Fatalf("ehentai request = %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			writeHTML(w, `<table class="itg"><tbody><tr>
<td>2024-01-02 03:04</td>
<td><a href="/g/123/abc">Gallery</a><a href="/gallerytorrents.php?gid=123&t=abc">E-Hentai Book</a></td>
<td>ignored</td><td>2.00 GiB</td><td>9</td><td>1</td><td>42</td>
</tr></tbody></table>`)
		case "gtnet":
			handleGTNetMock(t, w, r)
		case "gtorg":
			handleGTOrgMock(t, w, r)
		case "nekobt":
			if r.URL.Path != "/api/v1/torrents/search" || r.URL.Query().Get("query") != "movie" {
				t.Fatalf("nekobt request = %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"results":[{
				"id":123,"title":"Neko Anime","filesize":2147483648,
				"magnet":"magnet:?xt=urn:btih:neko","uploaded_at":"2024-01-02T03:04:05Z",
				"seeders":15,"leechers":2,"completed":9
			}]}}`))
		case "rutracker":
			if r.URL.Path != "/forum/tracker.php" || r.URL.Query().Get("search") != "movie" {
				t.Fatalf("rutracker request = %s?%s", r.URL.Path, r.URL.RawQuery)
			}
			writeHTML(w, `<table id="tor-tbl"><tbody><tr>
<td class="f-name-col"><div class="f-name"><a>Movies</a></div></td>
<td class="t-title-col"><div class="t-title"><a class="tLink" href="/forum/viewtopic.php?t=123">RuTracker Movie</a></div></td>
<td>c3</td><td>c4</td><td>c5</td><td class="tor-size" data-ts_text="2.00 GiB"><a class="tr-dl" href="/forum/dl.php?id=123">DL</a></td>
<td data-ts_text="22">22</td><td class="leechmed">5</td><td>70</td><td data-ts_text="2024-01-02 03:04">date</td>
</tr></tbody></table>`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func handleGTNetMock(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	switch r.URL.Path {
	case "/torrentslist.php":
		writeHTML(w, `<script>var SECURITYTOKEN = "token-1";</script>`)
	case "/search.php":
		if r.Method != http.MethodPost {
			t.Fatalf("gtnet search method = %s, want POST", r.Method)
		}
		if r.URL.Query().Get("do") != "process" {
			t.Fatalf("gtnet search query = %s", r.URL.RawQuery)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.PostForm.Get("query") != "movie" || r.PostForm.Get("securitytoken") != "token-1" {
			t.Fatalf("gtnet form = %s", r.PostForm.Encode())
		}
		writeHTML(w, `<div id="display"><ul class="TorrentList">
<li class="TorrentList1"><a href="/torrentslist.php?type=Movies%2FHD">cat</a></li>
<li class="TorrentList2"><a href="/torrentdetails.php?torrentid=abc123">GTNet Movie</a></li>
<li class="TorrentList3">2.00 GiB</li>
<li class="TorrentList4"><a href="/download.php?id=abc123">DL</a></li>
<li class="TorrentList5"></li><li class="TorrentList6">13</li><li class="TorrentList7">4</li>
<li class="TorrentList8">12:34 31-Dec-2024</li>
</ul></div>`)
	case "/member.php":
		if r.URL.RawQuery == "" {
			writeHTML(w, `<ul class="isuser"><li class="welcomelink"><a href="/member.php?42">alice</a></li></ul>`)
			return
		}
		if r.URL.RawQuery != "42-alice" {
			t.Fatalf("gtnet profile query = %q", r.URL.RawQuery)
		}
		writeHTML(w, `<div id="userinfo"><span class="usertitle">Power User</span></div>
<dl class="stats"><dt>Uploaded</dt><dd>2.00 TiB</dd></dl>
<dl class="stats"><dt>Downloaded</dt><dd>512.00 GiB</dd></dl>
<dl class="stats"><dt>Ratio</dt><dd>2.5</dd></dl>
<dl class="stats"><dt>Juices</dt><dd>12,345.6</dd></dl>
<dl class="stats"><dt>Join Date</dt><dd>12:34 31-Dec-2024</dd></dl>`)
	default:
		http.NotFound(w, r)
	}
}

func handleGTOrgMock(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	switch r.URL.Path {
	case "/torrents_beta.php":
		if r.URL.Query().Get("search") != "movie" {
			t.Fatalf("gtorg query = %s", r.URL.RawQuery)
		}
		writeHTML(w, `<div class="torrentsContainer"><div class="torrent">
<div class="categoryNew"><a>Movies</a></div>
<a class="torrent_link" href="/details.php?id=abc123">GTOrg Movie</a>
<a class="downloadLink" href="/download.php?id=abc123">DL</a>
<div class="date"><div>on 12:34 31-Dec-2024</div></div>
<div class="size">2.00 GiB</div>
<div class="downloadPeers"><a class="green">11</a><a class="red">3</a></div>
<div class="downloadTimes"><a>44 times</a></div>
</div></div>`)
	case "/index.php":
		writeHTML(w, `<a href="/usercp.php?uid=42">Profile</a><a href="/seedbonus.php">9,876.5</a><span title="Seed">7</span>`)
	case "/usercp.php":
		if r.URL.Query().Get("uid") != "42" {
			t.Fatalf("gtorg uid = %q", r.URL.Query().Get("uid"))
		}
		writeHTML(w, `<table>
<tr><td>User</td><td>alice</td></tr>
<tr><td>Rank</td><td>Poweruser</td></tr>
<tr><td>Downloaded</td><td>512.00 GiB</td></tr>
<tr><td>Uploaded</td><td>2.00 TiB</td></tr>
<tr><td>Ratio</td><td>4.0</td></tr>
<tr><td>Joined on</td><td>2024-01-02 03:04</td></tr>
</table>`)
	default:
		http.NotFound(w, r)
	}
}

func writeHTML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<html><body>" + strings.TrimSpace(body) + "</body></html>"))
}
