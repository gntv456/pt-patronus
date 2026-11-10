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

func TestGenericHTMLFetchUserInfoPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a class="me" href="/user/42">Ada</a>`))
		case "/profile":
			if r.URL.Query().Get("uid") != "42" {
				t.Fatalf("uid = %q, want 42", r.URL.Query().Get("uid"))
			}
			_, _ = w.Write([]byte(`<div class="level">Power</div><div class="uploaded">1.25 TiB</div>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "")
	u, err := g.FetchUserInfo(context.Background(), &site.SiteMetadata{
		ID: "pages",
		UserInfo: site.UserInfoDef{
			Pages: []site.UserInfoPageDef{
				{
					Path: "/",
					Fields: map[string]site.ElementQuery{
						"id":   {Selector: "a.me", Attr: "href", Filters: []site.Filter{{Name: "regexp", Args: `/user/(\d+)`}}},
						"name": {Selector: "a.me"},
					},
				},
				{
					Path:   "/profile",
					Params: map[string]string{"uid": "{id}"},
					Fields: map[string]site.ElementQuery{
						"level":    {Selector: ".level"},
						"uploaded": {Selector: ".uploaded"},
					},
				},
			},
		},
	}, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "Ada" || u.Level != "Power" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if want := int64(1.25 * float64(1<<40)); u.Uploaded != want {
		t.Fatalf("uploaded = %d, want %d", u.Uploaded, want)
	}
}

func TestGenericHTMLFetchUserInfoJSONPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":11,"displayName":"milk","uploaded":1099511627776,"downloaded":536870912,"createdAt":"2023-04-05T06:07:08Z"}}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "")
	u, err := g.FetchUserInfo(context.Background(), &site.SiteMetadata{
		ID: "json-page",
		UserInfo: site.UserInfoDef{Pages: []site.UserInfoPageDef{{
			Path:   "/api/me",
			Format: "json",
			Fields: map[string]site.ElementQuery{
				"id":         {Selector: "user.id"},
				"name":       {Selector: "user.displayName"},
				"uploaded":   {Selector: "user.uploaded"},
				"downloaded": {Selector: "user.downloaded"},
				"join_date":  {Selector: "user.createdAt", Filters: []site.Filter{{Name: "parse_time"}}},
			},
		}}},
	}, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "11" || u.Name != "milk" || u.JoinDate != "2023-04-05" {
		t.Fatalf("identity = id %q name %q join %q", u.ID, u.Name, u.JoinDate)
	}
	if u.Uploaded != 1099511627776 || u.Downloaded != 536870912 {
		t.Fatalf("traffic = uploaded %d downloaded %d", u.Uploaded, u.Downloaded)
	}
}

func TestGenericHTMLFetchSkyeySnowPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<div class="vwmy"><a href="home.php?mod=space&uid=9">snow</a></div>`))
		case "/home.php":
			if r.URL.Query().Get("uid") != "9" || r.URL.Query().Get("mod") != "space" {
				t.Fatalf("query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`<ul id="psts">
<li>上传量 / 5.00 GiB</li>
<li>下载量 / 1.00 GiB</li>
<li>实际上传 / 8.00 GiB</li>
<li>实际下载 / 2.00 GiB</li>
</ul>
<ul class="bbda"><li>分享率 5.0</li></ul>
<div id="ratio">1234.5</div>
<a href="home.php?mod=spacecp&ac=usergroup">用户组 Lv.2</a>
<ul id="pbbs"><li>注册时间 2024-05-06 07:08</li><li>最后访问 2024-05-07 08:09</li></ul>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "discuz")
	u, err := g.FetchUserInfo(context.Background(), &site.SiteMetadata{
		ID:     "skyeysnow",
		Schema: "discuz",
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
					"uploaded":   {Selector: `#psts li:contains("上传"), #psts li:contains("Uploaded")`, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:上传量|上传|uploaded)[^0-9]*([\d.,]+\s*[KMGTPE]?i?B)`}, {Name: "parse_size"}}},
					"downloaded": {Selector: `#psts li:contains("下载"), #psts li:contains("Downloaded")`, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:下载量|下载|downloaded)[^0-9]*([\d.,]+\s*[KMGTPE]?i?B)`}, {Name: "parse_size"}}},
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
	}, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "9" || u.Name != "snow" || u.Level != "Lv.2" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if u.Uploaded != 5*(1<<30) || u.Downloaded != 1*(1<<30) || u.Ratio != 5 {
		t.Fatalf("traffic = uploaded %d downloaded %d ratio %g", u.Uploaded, u.Downloaded, u.Ratio)
	}
	if u.Bonus != 1234.5 || u.JoinDate != "2024-05-06" {
		t.Fatalf("bonus/join = %g/%q", u.Bonus, u.JoinDate)
	}
	if u.TrueUploaded != 8*(1<<30) || u.TrueDownloaded != 2*(1<<30) || u.LastAccessAt != "2024-05-07" {
		t.Fatalf("extras = true_uploaded %d true_downloaded %d last_access_at %q", u.TrueUploaded, u.TrueDownloaded, u.LastAccessAt)
	}
}

func TestGenericHTMLFetchSportsCultSeedingSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			switch r.URL.Query().Get("page") {
			case "":
				_, _ = w.Write([]byte(`<a href="index.php?page=usercp&uid=42">profile</a>`))
			case "usercp":
				if r.URL.Query().Get("uid") != "42" {
					t.Fatalf("uid = %q, want 42", r.URL.Query().Get("uid"))
				}
				_, _ = w.Write([]byte(`<div id="mcol"><div class="b-content">
				<table class="lista"><tbody><tr><td>ignore</td></tr></tbody></table>
				<table class="lista"><tbody><tr><td>ignore</td></tr></tbody></table>
				<table class="lista"><tbody>
				  <tr><th>Name</th><th>Size</th></tr>
				  <tr><td colspan="2">Active seeding torrents</td></tr>
				  <tr><td>Match A</td><td>1.50 GiB</td></tr>
				  <tr><td>Match B</td><td>512 MiB</td></tr>
				</tbody></table>
				</div></div>
				<table>
				  <tr><td class="header">User</td><td>sporty</td></tr>
				  <tr><td class="header">Last access</td><td>2024-05-07</td></tr>
				  <tr><td class="header">Active Torrents</td><td>2</td></tr>
				</table>`))
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "xbtit")
	u, err := g.FetchUserInfo(context.Background(), &site.SiteMetadata{
		ID:     "sportscult",
		Schema: "xbtit",
		UserInfo: site.UserInfoDef{Pages: []site.UserInfoPageDef{
			{
				Path: "/index.php",
				Fields: map[string]site.ElementQuery{
					"id": {Selector: `a[href*='index.php?page=usercp']`, Attr: "href", Filters: []site.Filter{{Name: "queryparam", Args: "uid"}}},
				},
			},
			{
				Path:   "/index.php",
				Params: map[string]string{"page": "usercp", "uid": "{id}"},
				Fields: map[string]site.ElementQuery{
					"name":           {Selector: `td.header:contains('User') + td`},
					"last_access_at": {Selector: `td.header:contains('Last access') + td`, Filters: []site.Filter{{Name: "parse_time"}}},
					"seeding":        {Selector: `td.header:contains('Active Torrents') + td`, Filters: []site.Filter{{Name: "parse_number"}}},
					"seeding_size":   {Selector: `#mcol div.b-content table.lista:nth-of-type(3) tbody tr:not(:first-child):not(:nth-child(2)) td:nth-child(2)`, Attr: "text_all", Filters: []site.Filter{{Name: "sum_size"}}},
				},
			},
		}},
	}, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "sporty" {
		t.Fatalf("identity = id %q name %q", u.ID, u.Name)
	}
	if u.LastAccessAt != "2024-05-07" {
		t.Fatalf("last_access_at = %q, want 2024-05-07", u.LastAccessAt)
	}
	if u.Seeding != 2 {
		t.Fatalf("seeding = %d, want 2", u.Seeding)
	}
	if want := int64(2 * (1 << 30)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestGenericHTMLFetchSmallHorseUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="user.php?id=42">small-user</a>`))
		case "/user.php":
			if r.URL.Query().Get("id") != "42" {
				t.Fatalf("id = %q, want 42", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<a href="user.php?id=42">small-user</a>
<ul class="stats nobullet"><li>ignored</li></ul>
<ul class="stats nobullet">
  <li>Joined: <span>2024-05-06 07:08:09</span></li>
  <li>Ignored</li>
  <li>Uploaded: 1.50 TiB</li>
  <li>Downloaded: 512 GiB</li>
  <li>Ratio: <span>3.0</span></li>
  <li>Bonus: 12,345.5</li>
</ul>
<ul class="stats nobullet"><li>ignored</li></ul>
<ul class="stats nobullet"><li>Class: Power User</li></ul>
<ul class="stats nobullet">
  <li>0</li><li>1</li><li>2</li><li>3</li><li>4</li><li>5</li>
  <li>Leeching: [2]</li>
</ul>`))
		case "/torrents.php":
			if r.URL.Query().Get("type") != "seeding" || r.URL.Query().Get("userid") != "42" {
				t.Fatalf("query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`
<table id="torrent_table">
  <tr><th>Name</th><th>2</th><th>3</th><th>4</th><th>5</th><th>Size</th><th>7</th><th>Seeders</th></tr>
  <tr><td>A</td><td></td><td></td><td></td><td></td><td>1.50 GiB</td><td></td><td>4</td></tr>
  <tr><td>B</td><td></td><td></td><td></td><td></td><td>512 MiB</td><td></td><td>2</td></tr>
</table>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "smallhorse")
	def := site.EnsureDefaults(&site.SiteMetadata{ID: "smallhorse-demo", Schema: "smallhorse"})
	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "small-user" || u.Level != "Power User" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if want := int64(1536 * (1 << 30)); u.Uploaded != want {
		t.Fatalf("uploaded = %d, want %d", u.Uploaded, want)
	}
	if want := int64(512 * (1 << 30)); u.Downloaded != want {
		t.Fatalf("downloaded = %d, want %d", u.Downloaded, want)
	}
	if u.Ratio != 3 || u.Bonus != 12345.5 || u.JoinDate != "2024-05-06" || u.Leeching != 2 {
		t.Fatalf("stats = ratio %g bonus %g join %q leeching %d", u.Ratio, u.Bonus, u.JoinDate, u.Leeching)
	}
	if u.Seeding != 2 {
		t.Fatalf("seeding = %d, want 2", u.Seeding)
	}
	if want := int64(2 * (1 << 30)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestGenericHTMLFetchTCGUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/main.php":
			_, _ = w.Write([]byte(`<a href="userdetails.php?id=42">tcg-user</a>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "42" {
				t.Fatalf("id = %q, want 42", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table class="embedded">
  <tr><td>Join date</td><td>2024-01-02 03:04:05 (1 year ago)</td></tr>
  <tr><td>Last seen</td><td>2024-02-03 04:05:06 (2 days ago)</td></tr>
</table>
<table>
  <tr><td class="rowhead">Uploaded</td><td>2.00 TiB (real)</td></tr>
  <tr><td class="rowhead">Downloaded</td><td>512 GiB (real)</td></tr>
  <tr><td class="rowhead">Class</td><td>Power User</td></tr>
  <tr><td class="statuslink"><span>Ratio:</span><span>3.5</span></td></tr>
</table>
<img title="seeders"><span>4</span>
<div id="kd1"><table class="details"><tbody>
  <tr><td class="colbrowsehead">Name</td><td class="colbrowsehead">2</td><td class="colbrowsehead">3</td><td class="colbrowsehead">Size</td></tr>
  <tr><td>A</td><td></td><td></td><td>1.50 GiB</td></tr>
  <tr><td>B</td><td></td><td></td><td>512 MiB</td></tr>
</tbody></table></div>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "tcg")
	def := site.EnsureDefaults(&site.SiteMetadata{ID: "thegeeks", Schema: "tcg"})
	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "tcg-user" || u.Level != "Power User" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if u.JoinDate != "2024-01-02" || u.LastAccessAt != "2024-02-03" {
		t.Fatalf("dates = join %q last %q", u.JoinDate, u.LastAccessAt)
	}
	if want := int64(2 * (1 << 40)); u.Uploaded != want {
		t.Fatalf("uploaded = %d, want %d", u.Uploaded, want)
	}
	if want := int64(512 * (1 << 30)); u.Downloaded != want {
		t.Fatalf("downloaded = %d, want %d", u.Downloaded, want)
	}
	if u.Ratio != 3.5 || u.Seeding != 4 {
		t.Fatalf("stats = ratio %g seeding %d", u.Ratio, u.Seeding)
	}
	if want := int64(2 * (1 << 30)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestGenericHTMLTCGSearchDefaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/browse.php" || r.URL.Query().Get("search") != "movie" {
			t.Fatalf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		for key, want := range map[string]string{"incldead": "1", "titleonly": "1", "nonboolean": "0"} {
			if got := r.URL.Query().Get(key); got != want {
				t.Fatalf("%s = %q, want %q", key, got, want)
			}
		}
		_, _ = w.Write([]byte(`
<html><body><table border="0"><tbody>
<tr class="ttable">
  <td><a href="browse.php?cat=212">AudioBook</a></td>
  <td><a href="details.php?id=99" title="TCG Movie">details</a> <a href="download.php/99/file.torrent">DL</a></td>
  <td>c3</td><td><span><em>Course subtitle</em></span></td><td>5</td>
  <td>2024-01-02 03:04:05</td><td>2.00 GiB</td><td>Never</td><td>7</td><td>2</td>
</tr>
<tr class="ttable"><td><a href="browse.php?cat=212">cat</a></td><td><font color="blue">hidden</font><a href="details.php?id=100" title="Nope">Nope</a></td></tr>
</tbody></table></body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "tcg")
	def := site.EnsureDefaults(&site.SiteMetadata{ID: "tcg-demo", Schema: "tcg"})
	ts, err := g.Search(context.Background(), def, srv.URL, "cookie", "movie", "", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ts) != 1 {
		t.Fatalf("got %d torrents, want 1: %+v", len(ts), ts)
	}
	got := ts[0]
	if got.Title != "TCG Movie" || got.SubTitle != "Course subtitle" {
		t.Fatalf("title/subtitle = %q/%q", got.Title, got.SubTitle)
	}
	if got.Size != "2147483648" || got.Seeders != 7 || got.Leechers != 2 || got.Completed != 0 {
		t.Fatalf("stats = size %q seeders %d leechers %d completed %d", got.Size, got.Seeders, got.Leechers, got.Completed)
	}
	if got.DetailURL != srv.URL+"/details.php?id=99" || got.DownloadURL != srv.URL+"/download.php/99/file.torrent" {
		t.Fatalf("urls = detail %q download %q", got.DetailURL, got.DownloadURL)
	}
}

func TestGenericHTMLFetchIPTorrentsHumanReadableTraffic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="/u/77">profile</a>`))
		case "/user.php":
			if r.URL.Query().Get("u") != "77" {
				t.Fatalf("u = %q, want 77", r.URL.Query().Get("u"))
			}
			_, _ = w.Write([]byte(`
<h1 class="up-username">ipt-user <img alt="Donor"></h1>
<div style="background-image:url('/assets/up-stat-up.svg')"></div><div class="up-stat-sub">1.50 TiB</div>
<div style="background-image:url('/assets/up-stat-down.svg')"></div><div class="up-stat-sub">512 GiB</div>
<div class="up-stat-value"><svg class="up-ratio-icon"></svg><span>3.0</span></div>
<span class="up-class-badge">Power User</span>
<a href="/mybonus.php">12,345.6 points</a>
<span class="up-field-label">Join Date</span><span><span class="elapsedDate" title="2024-05-06 07:08:09">joined</span></span>
<span class="up-field-label">Last Seen</span><span><span class="elapsedDate" title="2024-05-07 08:09:10">last</span></span>
<span class="up-field-label">Seeding</span><span><a href="/peers?u=77">15</a></span>
<span><a href="/t?u=77">4 uploads</a></span>`))
		case "/peers":
			if r.URL.Query().Get("u") != "77" {
				t.Fatalf("u = %q, want 77", r.URL.Query().Get("u"))
			}
			_, _ = w.Write([]byte(`<table id="torrents"><tbody>
<tr><td><img alt="Movie"></td><td class="al"><a href="/t/1">A</a></td><td>x</td><td>x</td><td>x</td><td>1.00 GiB</td><td>x</td><td>5</td><td>0</td></tr>
<tr><td><img alt="Movie"></td><td class="al"><a href="/t/2">B</a></td><td>x</td><td>x</td><td>x</td><td>512 MiB</td><td>x</td><td>3</td><td>0</td></tr>
</tbody></table>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("iptorrents")
	if !ok {
		t.Fatal("iptorrents definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "")
	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "77" || u.Name != "ipt-user" || u.Level != "Power User" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if want := int64(1536 * (1 << 30)); u.Uploaded != want {
		t.Fatalf("uploaded = %d, want %d", u.Uploaded, want)
	}
	if want := int64(512 * (1 << 30)); u.Downloaded != want {
		t.Fatalf("downloaded = %d, want %d", u.Downloaded, want)
	}
	if u.Ratio != 3 || u.Bonus != 12345.6 || u.Seeding != 15 || u.Uploads != 4 || u.JoinDate != "2024-05-06" || u.LastAccessAt != "2024-05-07" {
		t.Fatalf("stats = ratio %g bonus %g seeding %d uploads %d join %q last %q", u.Ratio, u.Bonus, u.Seeding, u.Uploads, u.JoinDate, u.LastAccessAt)
	}
	if want := int64(1536 * (1 << 20)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
	if !u.IsDonor {
		t.Fatalf("is_donor = false, want true")
	}
}

func TestGazelleFetchAnimeBytesTitleTraffic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<div id="stats_menu"><a href="/user.php?userid=55">stats</a></div><a class="username" href="/user.php?id=55">ab-user</a>`))
		case "/user.php":
			id := r.URL.Query().Get("id")
			if id == "" {
				id = r.URL.Query().Get("userid")
			}
			if id != "55" {
				t.Fatalf("id = %q, want 55", id)
			}
			_, _ = w.Write([]byte(`
<dl>
  <dt>Torrents Uploaded:</dt><dd>22</dd>
  <dt>Uploaded:</dt><dd><span title="1.25 TiB">1.25 TiB</span></dd>
  <dt>Downloaded:</dt><dd><span title="512 GiB">512 GiB</span></dd>
  <dt>Ratio:</dt><dd><span>2.5</span></dd>
  <dt>Seeding:</dt><dd>9</dd>
  <dt>Total seed size:</dt><dd><span title="2 TiB">2 TiB</span></dd>
  <dt>Class:</dt><dd>Legend</dd>
  <dt>Yen per day:</dt><dd>240</dd>
  <dt>Joined:</dt><dd><span title="2024-04-03 02:01:00">joined</span></dd>
  <dt>Last Seen</dt><dd><span title="2024-04-04 03:02:01">seen</span></dd>
</dl>
<div id="yen_count"><a>123,456</a></div>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("animebytes")
	if !ok {
		t.Fatal("animebytes definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelle(hc)
	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "55" || u.Name != "ab-user" || u.Level != "Legend" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if want := int64(1280 * (1 << 30)); u.Uploaded != want {
		t.Fatalf("uploaded = %d, want %d", u.Uploaded, want)
	}
	if want := int64(512 * (1 << 30)); u.Downloaded != want {
		t.Fatalf("downloaded = %d, want %d", u.Downloaded, want)
	}
	if want := int64(2 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
	if u.Ratio != 2.5 || u.Seeding != 9 || u.Uploads != 22 || u.Bonus != 123456 || u.BonusPerHour != 10 || u.JoinDate != "2024-04-03" || u.LastAccessAt != "2024-04-04" {
		t.Fatalf("stats = ratio %g seeding %d uploads %d bonus %g bph %g join %q last %q", u.Ratio, u.Seeding, u.Uploads, u.Bonus, u.BonusPerHour, u.JoinDate, u.LastAccessAt)
	}
}

func TestNexusPHPDefinitionsSeedingBonusPerHourPages(t *testing.T) {
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}

	tests := []struct {
		id   string
		body string
		want float64
	}{
		{
			id: "springsunday",
			body: `<table><tbody><tr class="nowrap">
<td>1</td><td>2</td><td>3</td><td>4</td><td>5</td><td>6</td><td>7</td><td>8</td><td>12.5</td><td>99</td>
</tr></tbody></table>`,
			want: 12.5,
		},
		{
			id:   "hhanclub",
			body: `<div>你当前每小时能获取 11.25 憨豆</div><div>你当前每小时能获取 7.5 做种积分</div>`,
			want: 7.5,
		},
		{
			id: "qingwa",
			body: `<h1>每小时获得的合计蝌蚪</h1><div><table><tr>
<td>1</td><td>2</td><td>3</td><td>4</td><td>5</td><td>6</td><td>7</td><td>8</td><td>9</td>
<td>10</td><td>11</td><td>12</td><td>13</td><td>14</td><td>15</td><td>16</td><td>88.25</td>
</tr></table></div>`,
			want: 88.25,
		},
	}

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	adapter := &BaseHTMLAdapter{hc: hc}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/mybonus.php" {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			def, ok := reg.Definition(tc.id)
			if !ok {
				t.Fatalf("definition %s missing", tc.id)
			}
			def = site.EnsureDefaults(def)
			u := &site.UserInfo{}
			if err := fetchUserInfoPages(context.Background(), adapter, def, srv.URL, "cookie", u); err != nil {
				t.Fatalf("fetchUserInfoPages: %v", err)
			}
			if u.SeedingBonusPerHour != tc.want {
				t.Fatalf("seeding_bonus_per_hour = %g, want %g", u.SeedingBonusPerHour, tc.want)
			}
		})
	}
}
