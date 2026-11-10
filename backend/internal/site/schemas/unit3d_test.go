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

// TestUnit3DSearch：canned /torrents 页（1 真 torrent 行 + 1 表头行）→ 验行过滤（丢表头）、
// 字段抽取、download 链接 /download_check/ → /download/ 重写、EnsureDefaults 注入的默认选择器。
func TestUnit3DSearch(t *testing.T) {
	var gotName, gotPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotName = r.URL.Query().Get("name")
		gotPerPage = r.URL.Query().Get("perPage")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
		<div class="torrent-search--list__results"><table><tbody>
		<tr><th>名称</th><th>大小</th><th>做种</th><th>下载</th></tr>
		<tr>
		  <td><a class="view-torrent" href="/torrents/123">My Movie 2024</a></td>
		  <td class="torrent-search--list__size">2.5 GB</td>
		  <td class="torrent-search--list__seeders">7</td>
		  <td class="torrent-search--list__leechers">2</td>
		  <td class="torrent-search--list__completed">40</td>
		  <td><a href="/torrents/download_check/123">dl</a></td>
		</tr>
		</tbody></table></div>
		</body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u := NewUnit3D(hc)
	def := &site.SiteMetadata{ID: "u3d", Schema: "unit3d"} // 选择器由 Search 内 EnsureDefaults 注入

	ts, err := u.Search(context.Background(), def, srv.URL, "", "hello", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "hello" {
		t.Errorf("backend received name param = %q, want hello", gotName)
	}
	if gotPerPage != "100" {
		t.Errorf("perPage = %q, want 100", gotPerPage)
	}
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent (header row filtered), got %d", len(ts))
	}
	if ts[0].Title != "My Movie 2024" {
		t.Errorf("Title = %q", ts[0].Title)
	}
	if ts[0].Seeders != 7 || ts[0].Completed != 40 {
		t.Errorf("Seeders/Completed = %d/%d", ts[0].Seeders, ts[0].Completed)
	}
	if ts[0].DetailURL != srv.URL+"/torrents/123" {
		t.Errorf("DetailURL = %q", ts[0].DetailURL)
	}
	// /download_check/ → /download/ 重写
	if ts[0].DownloadURL != srv.URL+"/torrents/download/123" {
		t.Errorf("DownloadURL = %q (want download_check rewritten to download)", ts[0].DownloadURL)
	}
}

func TestUnit3DSearchJSONAPI(t *testing.T) {
	var gotPath, gotName, gotPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotName = r.URL.Query().Get("name")
		gotPerPage = r.URL.Query().Get("perPage")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":[{
			"id": 77,
			"name": "HUNO Movie",
			"size": "2.00 GiB",
			"seeders": 8,
			"leechers": 1,
			"times_completed": 12,
			"download_link": "/api/torrents/77/download",
			"category": {"name": "Movie"}
		}]}}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u := NewUnit3D(hc)
	def := &site.SiteMetadata{
		ID: "huno", Schema: "unit3d", SearchEntry: "/api/torrents/filter", SearchParam: "name",
		Selectors: site.SiteSelectors{
			Rows: site.ElementQuery{Selectors: []string{"missing", "data.data"}},
			Fields: map[string]site.ElementQuery{
				"title":     {Selectors: []string{"name", "attributes.name"}},
				"detail":    {Selectors: []string{"id", "attributes.id"}, Filters: []site.Filter{{Name: "prepend", Args: "/torrents/"}}},
				"download":  {Selectors: []string{"download_link", "attributes.download_link"}},
				"size":      {Selectors: []string{"size", "attributes.size"}},
				"seeders":   {Selectors: []string{"seeders", "attributes.seeders"}},
				"leechers":  {Selectors: []string{"leechers", "attributes.leechers"}},
				"completed": {Selectors: []string{"times_completed", "attributes.times_completed"}},
				"category":  {Selectors: []string{"category.name", "attributes.category.name"}},
			},
		},
	}

	ts, err := u.Search(context.Background(), def, srv.URL, "cookie", "huno", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/torrents/filter" || gotName != "huno" || gotPerPage != "100" {
		t.Fatalf("request = path %q name %q perPage %q", gotPath, gotName, gotPerPage)
	}
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent, got %d", len(ts))
	}
	got := ts[0]
	if got.Title != "HUNO Movie" || got.Category != "Movie" {
		t.Fatalf("title/category = %q/%q", got.Title, got.Category)
	}
	if got.DetailURL != srv.URL+"/torrents/77" {
		t.Fatalf("detail = %q", got.DetailURL)
	}
	if got.DownloadURL != srv.URL+"/api/torrents/77/download" {
		t.Fatalf("download = %q", got.DownloadURL)
	}
	if got.Size != "2.00 GiB" || got.Seeders != 8 || got.Leechers != 1 || got.Completed != 12 {
		t.Fatalf("stats = size %q seeders %d leechers %d completed %d", got.Size, got.Seeders, got.Leechers, got.Completed)
	}
}

// TestUnit3DFetchUserInfo：三步流程。server 按路径切换：
//   - /  → 顶部 settings 链接（发现用户名 bob）
//   - /users/bob → ratio-bar uploaded + profile__registration
//   - /users/bob/earnings → .panelV2 dl.key-value 第 3 个 dd（bonusPerHour）
func TestUnit3DFetchUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body><a href="/users/bob/settings">Bob</a></body></html>`))
		case "/users/bob":
			_, _ = w.Write([]byte(`<html><body>
			<ul class="ratio-bar">
			  <li class="ratio-bar__uploaded"><a href="/users/bob/uploads"><i class="fa fa-arrow-up"></i> 10 GB</a></li>
			</ul>
			<dl><dt>Seeding Size</dt><dd>1.5 TiB</dd></dl>
			<time class="profile__registration" datetime="2022-03-04">2022</time>
			</body></html>`))
		case "/users/bob/earnings":
			_, _ = w.Write([]byte(`<html><body><div class="panelV2"><dl class="key-value">
			  <dd>seedbonus</dd><dd>total</dd><dd>123.45</dd>
			</dl></div></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u := NewUnit3D(hc)
	def := &site.SiteMetadata{ID: "u3d", Schema: "unit3d"}

	ui, err := u.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.Name != "bob" {
		t.Errorf("Name = %q, want bob", ui.Name)
	}
	if ui.Uploaded != 10*1024*1024*1024 {
		t.Errorf("Uploaded = %d, want 10 GiB", ui.Uploaded)
	}
	if ui.SeedingSize != 1536*1024*1024*1024 {
		t.Errorf("SeedingSize = %d, want 1.5 TiB", ui.SeedingSize)
	}
	if ui.BonusPerHour != 123.45 {
		t.Errorf("BonusPerHour = %v, want 123.45", ui.BonusPerHour)
	}
}

func TestUnit3DFetchUserInfoWithCustomPages(t *testing.T) {
	var sawCustomBonus bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body><a class="top-nav__username" href="/users/alice">Alice</a></body></html>`))
		case "/users/alice":
			_, _ = w.Write([]byte(`<html><body>
			<div class="col-lg-10">组别: Seeder 注册日期</div>
			<div class="block">注册日期 2024-05-06 12:00:00 总发布 42 用户 ID</div>
			<a href="/mail/create?receiver_id=99">message</a>
			<img class="avatar" src="https://img.example/monika.png">
			<table class="user-info">
			  <tr><td>虚拟上传量</td><td>10.00 TiB</td></tr>
			  <tr><td>虚拟下载量</td><td>2.00 TiB</td></tr>
			  <tr><td>分享率</td><td>5.0</td></tr>
			  <tr><td>魔力</td><td>12345.6</td></tr>
			  <tr><td>做种体积</td><td>3.50 TiB</td></tr>
			</table>
			<span class="badge-user">总做种数量 7</span>
			<span class="badge-user">总发布 42</span>
			</body></html>`))
		case "/users/alice/earnings":
			http.NotFound(w, r)
		case "/users/alice/bonus/transactions/create":
			sawCustomBonus = true
			_, _ = w.Write([]byte(`<html><body><aside><div class="panelV2">
			<dl class="key-value"><dt>每小时获得魔力数量</dt><dd>88.8</dd></dl>
			</div></aside></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u := NewUnit3D(hc)
	def := &site.SiteMetadata{
		ID:     "monikadesign",
		Schema: "unit3d",
		UserInfo: site.UserInfoDef{
			Fields: map[string]site.ElementQuery{
				"id": {Selectors: []string{`a[href*="mail/create?receiver_id="]`}, Attr: "href", Filters: []site.Filter{{Name: "queryparam", Args: "receiver_id"}}},
				"name": {
					Selectors: []string{`a.top-nav__username[href*="/users/"]`, `a[href*="/users/"][href$="/settings"]`},
					Attr:      "href",
					Filters:   []site.Filter{{Name: "regexp", Args: `/users/([^/?#]+)`}, {Name: "url_decode"}},
				},
				"uploaded":     {Selectors: []string{`table.user-info tr:contains("虚拟上传量")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:虚拟上传量|Uploaded|Upload)[^0-9]*([\d.,]+\s*[KMGTPE]i?B)`}, {Name: "parse_size"}}},
				"downloaded":   {Selectors: []string{`table.user-info tr:contains("虚拟下载量")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:虚拟下载量|Downloaded|Download)[^0-9]*([\d.,]+\s*[KMGTPE]i?B)`}, {Name: "parse_size"}}},
				"ratio":        {Selectors: []string{`table.user-info tr:contains("分享率")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:分享率|Ratio)[^0-9∞Inf]*([\d.]+|∞|Inf|Infinity)`}}},
				"bonus":        {Selectors: []string{`table.user-info tr:contains("魔力")`}, Filters: []site.Filter{{Name: "parse_number"}}},
				"level":        {Selectors: []string{`.col-lg-10:contains("组别")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:组别|User Class|Class|等级)[：:]?\s*([A-Za-z][A-Za-z0-9 ]*)`}}},
				"join_date":    {Selectors: []string{`.block:contains("注册日期")`}, Filters: []site.Filter{{Name: "regexp", Args: `注册日期\s+(.+?)\s+总发布`}, {Name: "parse_time"}}},
				"seeding":      {Selectors: []string{`span.badge-user:contains("总做种数量")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?:总做种数量|Seeding)[^0-9]*(\d+)`}, {Name: "parse_number"}}},
				"seeding_size": {Selectors: []string{`table.user-info tr:contains("做种体积")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?i)(?:做种体积|做种大小|Seeding Size|Seedsize)[^0-9]*([\d.,]+\s*[KMGTPE]i?B)`}, {Name: "parse_size"}}},
				"uploads":      {Selectors: []string{`span.badge-user:contains("总发布")`}, Filters: []site.Filter{{Name: "regexp", Args: `(?:总发布|Uploads?)[^0-9]*(\d+)`}, {Name: "parse_number"}}},
				"avatar":       {Selectors: []string{"img.avatar", `img[src*="avatar"]`, "img.user-image", ".header img.img-circle"}, Attr: "src", Filters: []site.Filter{{Name: "clean_avatar"}}},
				"bonus_per_hour": {
					Selectors: []string{`dl.key-value:contains("每小时获得魔力数量")`},
					Filters:   []site.Filter{{Name: "regexp", Args: `(?i)(?:每小时获得魔力数量|per\s*hour|hourly|/h|时魔)[^0-9]{0,40}([\d.]+)`}, {Name: "parse_number"}},
				},
			},
			Pages: []site.UserInfoPageDef{{
				Path:      "/users/{name}/bonus/transactions/create",
				UseFields: []string{"bonus_per_hour"},
			}},
		},
	}

	ui, err := u.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if !sawCustomBonus {
		t.Fatal("custom bonus page was not requested")
	}
	if ui.ID != "99" || ui.Name != "alice" || ui.Level != "Seeder" || ui.JoinDate != "2024-05-06" {
		t.Fatalf("identity = id %q name %q level %q join %q", ui.ID, ui.Name, ui.Level, ui.JoinDate)
	}
	if ui.Uploaded != 10*(1<<40) || ui.Downloaded != 2*(1<<40) || ui.Ratio != 5 {
		t.Fatalf("traffic = uploaded %d downloaded %d ratio %g", ui.Uploaded, ui.Downloaded, ui.Ratio)
	}
	if ui.Seeding != 7 || ui.SeedingSize != int64(3.5*float64(1<<40)) || ui.Uploads != 42 {
		t.Fatalf("seed stats = seeding %d size %d uploads %d", ui.Seeding, ui.SeedingSize, ui.Uploads)
	}
	if ui.Bonus != 12345.6 || ui.BonusPerHour != 88.8 {
		t.Fatalf("bonus = %g bonus_per_hour = %g", ui.Bonus, ui.BonusPerHour)
	}
	if ui.Avatar != "https://img.example/monika.png" {
		t.Fatalf("avatar = %q", ui.Avatar)
	}
}

// TestUnit3DDetectsLoggedOut：cookie 失效 → 302 跳 /login → ErrNotLoggedIn。
func TestUnit3DDetectsLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusFound)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true, FollowRedirect: false})
	u := NewUnit3D(hc)
	def := &site.SiteMetadata{ID: "u3d", Schema: "unit3d"}

	_, err := u.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != site.ErrNotLoggedIn {
		t.Errorf("expected ErrNotLoggedIn, got %v", err)
	}
}
