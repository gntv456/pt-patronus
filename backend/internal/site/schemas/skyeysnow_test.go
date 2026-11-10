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

func TestSkyeySnowFetchUserInfoWithSeedAndUploads(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<div class="vwmy"><a href="/home.php?mod=space&uid=9">snow</a></div>`))
		case "/home.php":
			_, _ = w.Write([]byte(`<ul id="psts">
<li>上传量 / 5.00 GiB</li><li>下载量 / 1.00 GiB</li>
<li>实际上传 / 8.00 GiB</li><li>实际下载 / 2.00 GiB</li></ul>
<ul class="bbda"><li>分享率 5.0</li></ul><div id="ratio">1234.5</div>
<a href="home.php?mod=spacecp&ac=usergroup">用户组 Lv.2</a>
<ul id="pbbs"><li>注册时间 2024-05-06 12:00</li><li>最后访问 2024-05-07 08:09</li></ul>`))
		case "/forum.php":
			if r.URL.Query().Get("cat_5up") == "on" {
				switch r.URL.Query().Get("page") {
				case "0":
					_, _ = w.Write([]byte(`<a onclick="ajax_refreash('/torrents_list_ajax.php?mod=torrents&diy=&page=1')">1-2</a>
<table class="torrents"><tbody>
<tr><td>Anime</td><td class="torrent_title"><a href="/forum.php?mod=viewthread&tid=1" title="one">one</a></td><td>2024-01-01 00:00:00</td><td>1.00 GiB</td><td>2</td><td>0</td><td>3</td><td><a href="/download.php?id=1">DL</a></td></tr>
</tbody></table>`))
				case "1":
					_, _ = w.Write([]byte(`<table class="torrents"><tbody>
<tr><td>Anime</td><td class="torrent_title"><a href="/forum.php?mod=viewthread&tid=2" title="two">two</a></td><td>2024-01-02 00:00:00</td><td>2.00 GiB</td><td>4</td><td>1</td><td>5</td><td><a href="/download.php?id=2">DL</a></td></tr>
</tbody></table>`))
				default:
					_, _ = w.Write([]byte(`<table class="torrents"><tbody></tbody></table>`))
				}
				return
			}
			if r.URL.Query().Get("search") == "#9" {
				_, _ = w.Write([]byte(`<a onclick="ajax_refreash('/torrents_list_ajax.php?mod=torrents&diy=&page=3')">1-7</a>`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	def := skyeySnowTestDef()
	u, err := fetchSkyeySnowUserInfo(context.Background(), &BaseHTMLAdapter{hc: hc}, def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "9" || u.Name != "snow" || u.Level != "Lv.2" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if u.TrueUploaded != 8*(1<<30) || u.TrueDownloaded != 2*(1<<30) || u.LastAccessAt != "2024-05-07" {
		t.Fatalf("extras = true_uploaded %d true_downloaded %d last_access_at %q", u.TrueUploaded, u.TrueDownloaded, u.LastAccessAt)
	}
	if u.Seeding != 2 || u.SeedingSize != 3*(1<<30) {
		t.Fatalf("seeding = %d size %d", u.Seeding, u.SeedingSize)
	}
	if u.Uploads != 7 {
		t.Fatalf("uploads = %d, want 7", u.Uploads)
	}
}

func skyeySnowTestDef() *site.SiteMetadata {
	return &site.SiteMetadata{
		ID:     "skyeysnow",
		Schema: "discuz",
		Selectors: site.SiteSelectors{
			Rows: site.ElementQuery{Selector: "table.torrents > tbody > tr"},
			Fields: map[string]site.ElementQuery{
				"title":     {Selector: `td.torrent_title > a[title]`, Attr: "title"},
				"detail":    {Selector: `a[href^="/forum.php?mod=viewthread"]`, Attr: "href"},
				"download":  {Selector: `a[href^="/download.php?id="]`, Attr: "href"},
				"size":      {Selector: "td:nth-child(4)", Filters: []site.Filter{{Name: "parse_size"}}},
				"seeders":   {Selector: "td:nth-child(5)"},
				"leechers":  {Selector: "td:nth-child(6)"},
				"completed": {Selector: "td:nth-child(7)"},
			},
		},
		UserInfo: site.UserInfoDef{Pages: []site.UserInfoPageDef{
			{
				Path: "/",
				Fields: map[string]site.ElementQuery{
					"id":   {Selector: ".vwmy a", Attr: "href", Filters: []site.Filter{{Name: "queryparam", Args: "uid"}}},
					"name": {Selector: ".vwmy a"},
				},
			},
			{
				Path:   "/home.php",
				Params: map[string]string{"mod": "space", "uid": "{id}"},
				Fields: map[string]site.ElementQuery{
					"uploaded":   {Selector: `#psts li:contains("上传")`, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:上传量|上传|uploaded)[^0-9]*([\d.,]+\s*[KMGTPE]?i?B)`}, {Name: "parse_size"}}},
					"downloaded": {Selector: `#psts li:contains("下载")`, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:下载量|下载|downloaded)[^0-9]*([\d.,]+\s*[KMGTPE]?i?B)`}, {Name: "parse_size"}}},
					"true_uploaded": {
						Selector: `#psts li:contains("实际上传")`,
						Filters:  []site.Filter{{Name: "regexp", Args: `(?i)实际上传[^0-9]*([\d.,]+\s*[KMGTPE]?i?B)`}, {Name: "parse_size"}},
					},
					"true_downloaded": {
						Selector: `#psts li:contains("实际下载")`,
						Filters:  []site.Filter{{Name: "regexp", Args: `(?i)实际下载[^/]*\/\s*([\d.,]+\s*[KMGTPE]?i?B)`}, {Name: "parse_size"}},
					},
					"ratio":     {Selector: "ul.bbda", Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:分享率|ratio)[^0-9]*([\d.]+)`}}},
					"bonus":     {Selector: "#ratio", Filters: []site.Filter{{Name: "parse_number"}}},
					"level":     {Selector: `a[href='home.php?mod=spacecp&ac=usergroup']`, Filters: []site.Filter{{Name: "regexp", Args: `用户组\s*(.+)`}}},
					"join_date": {Selector: "#pbbs > li", Filters: []site.Filter{{Name: "regexp", Args: `(\d{4}-\d{2}-\d{2})`}}},
					"last_access_at": {
						Selector: `#pbbs > li:contains('最后访问')`,
						Filters:  []site.Filter{{Name: "regexp", Args: `最后访问\s*(.+)`}, {Name: "parse_time"}},
					},
				},
			},
		}},
	}
}
