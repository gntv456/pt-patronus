// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// nexusMultiDef 构造一份类好学的完整 def（color_* 基础字段 + rowhead detail 字段），
// 不依赖磁盘 yaml，自包含验证 NexusPHP.FetchUserInfo 的多页编排。
func nexusMultiDef() *site.SiteMetadata {
	return &site.SiteMetadata{
		ID: "t", Name: "T", Schema: "nexusphp",
		UserInfo: site.UserInfoDef{
			Path: "usercp.php",
			Fields: map[string]site.ElementQuery{
				"uploaded":   {Selector: "font.color_uploaded", Parent: true, Filters: []site.Filter{{Name: "regexp", Args: `(?:上传量)[：:]?\s*([\d.,]+\s*[KMGTPE]?i?B)`}}},
				"downloaded": {Selector: "font.color_downloaded", Parent: true, Filters: []site.Filter{{Name: "regexp", Args: `(?:下载量)[：:]?\s*([\d.,]+\s*[KMGTPE]?i?B)`}}},
				"ratio":      {Selector: "font.color_ratio", Parent: true, Filters: []site.Filter{{Name: "regexp", Args: `(?:分享率)[：:]?\s*([-+]?\d+\.?\d*)`}}},
				"seeding":    {Selector: "font.color_active", Parent: true, Filters: []site.Filter{{Name: "regexp", Args: `当前活动[：:]?\s*(\d+)\D+(\d+):::1`}}},
				"leeching":   {Selector: "font.color_active", Parent: true, Filters: []site.Filter{{Name: "regexp", Args: `当前活动[：:]?\s*(\d+)\D+(\d+):::2`}}},
			},
			DetailPath: "userdetails.php",
			DetailFields: map[string]site.ElementQuery{
				"name":      {Selector: `td[class*="rowhead"]:contains("用户名") + td`},
				"email":     {Selector: `td[class*="rowhead"]:contains("邮箱") + td`},
				"level":     {Selector: `td[class*="rowhead"]:contains("等级") + td img`, Attr: "title"},
				"join_date": {Selector: `td[class*="rowhead"]:contains("加入日期") + td`, Filters: []site.Filter{{Name: "regexp", Args: `(\d{4}-\d{2}-\d{2})`}}},
			},
		},
	}
}

// nexusMultiHandler 多页路由：usercp / userdetails / mybonus / getusertorrentlistajax。
// detailMissing=true 时 userdetails 返回 404（验证子页失败容错）。
func nexusMultiHandler(t *testing.T, detailMissing bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usercp.php":
			_, _ = w.Write([]byte(`<html><body>
<a href="https://x/userdetails.php?id=8">me</a>
<div><span><font class="color_ratio">分享率:</font> 5.5</span>
<span><font class="color_uploaded">上传量:</font> 10 TiB</span>
<span><font class="color_active">当前活动:</font> <img class="arrowup">7 <img class="arrowdown">2</span></div>
</body></html>`))
		case "/userdetails.php":
			if detailMissing {
				w.WriteHeader(404)
				return
			}
			_, _ = w.Write([]byte(`<table>
<tr><td class="rowhead">用户名</td><td>测试玩家</td></tr>
<tr><td class="rowhead">邮箱</td><td>player@example.com</td></tr>
<tr><td class="rowhead">等级</td><td><img title="贵宾"></td></tr>
<tr><td class="rowhead">加入日期</td><td>2023-01-02 03:04:05</td></tr></table>`))
		case "/mybonus.php":
			// 真实 NexusPHP mybonus：含「你当前每小时能获取」的 div。
			// 故意在外层放魔力总量 999999 干扰——bphValueRe 必须锚定「能获取」后取 123.4。
			_, _ = w.Write([]byte(`<html><body>
<div>当前火花: <b>999999</b>
  <div>你当前每小时能获取 123.4 个火花</div>
</div></body></html>`))
		case "/getusertorrentlistajax.php":
			tp := r.URL.Query().Get("type")
			if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`<table></table>`)) // 末页无行
				return
			}
			if tp == "uploaded" {
				// 发布数：头部「<b>N</b>条记录」+ 种子表
				_, _ = w.Write([]byte(`<div><b>5</b>条记录</div>
<table><tr><td><a href="details.php?id=1">t1</a></td></tr><tr><td><a href="details.php?id=2">t2</a></td></tr></table>`))
				return
			}
			// type=seeding：NexusPHP 汇总 div「N | SIZE」在前，种子表在后
			_, _ = w.Write([]byte(`<div><div>7 | 2.00 GiB</div></div>
<table>
<tr><td>t1</td><td>1.5 GB</td></tr>
<tr><td>t2</td><td>500 MB</td></tr></table>`))
		case "/claim.php":
			_, _ = w.Write([]byte(`<html><body><div>认领 3/8</div></body></html>`))
		default:
			w.WriteHeader(404)
		}
	})
}

func TestNexusPHPFetchUserInfoMultiPage(t *testing.T) {
	srv := httptest.NewServer(nexusMultiHandler(t, false))
	defer srv.Close()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)

	u, err := n.FetchUserInfo(context.Background(), nexusMultiDef(), srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if got := float64(u.Uploaded) / float64(1<<40); got < 9.9 || got > 10.1 {
		t.Errorf("uploaded: got %g TiB want ~10", got)
	}
	if u.Ratio < 5.4 || u.Ratio > 5.6 {
		t.Errorf("ratio: got %g want ~5.5", u.Ratio)
	}
	if u.Seeding != 7 || u.Leeching != 2 {
		t.Errorf("seeding/leeching: got %d/%d want 7/2", u.Seeding, u.Leeching)
	}
	if u.Level != "贵宾" {
		t.Errorf("level (from userdetails): got %q want 贵宾", u.Level)
	}
	if u.ID != "8" {
		t.Errorf("id (from usercp link ?id=): got %q want 8", u.ID)
	}
	if u.Name != "测试玩家" {
		t.Errorf("name (from userdetails rowhead): got %q want 测试玩家", u.Name)
	}
	if u.Email != "player@example.com" {
		t.Errorf("email (from userdetails rowhead): got %q want player@example.com", u.Email)
	}
	if u.JoinDate != "2023-01-02" {
		t.Errorf("join_date (from userdetails): got %q want 2023-01-02", u.JoinDate)
	}
	if u.BonusPerHour < 123.3 || u.BonusPerHour > 123.5 {
		t.Errorf("bonus_per_hour (from mybonus): got %g want ~123.4", u.BonusPerHour)
	}
	// 做种体积：取汇总 div「7 | 2.00 GiB」的 2.00 GiB（而非逐行累加）
	if u.SeedingSize != 2*1073741824 {
		t.Errorf("seeding_size (from summary div): got %d want %d", u.SeedingSize, 2*1073741824)
	}
	// 发布数：取 getusertorrentlistajax?type=uploaded 头部「<b>5</b>条记录」
	if u.Uploads != 5 {
		t.Errorf("uploads (from ajax header): got %d want 5", u.Uploads)
	}
	if u.ClaimCount != 3 || u.MaxClaim != 8 {
		t.Errorf("claims (from claim.php): got %d/%d want 3/8", u.ClaimCount, u.MaxClaim)
	}
}

// TestNexusPHPFetchUserInfoDetailMissing userdetails 404 仍返回基础统计（best-effort 容错）。
func TestNexusPHPFetchUserInfoDetailMissing(t *testing.T) {
	srv := httptest.NewServer(nexusMultiHandler(t, true))
	defer srv.Close()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)

	u, err := n.FetchUserInfo(context.Background(), nexusMultiDef(), srv.URL, "cookie")
	if err != nil {
		t.Fatalf("detail 404 should not fail whole fetch: %v", err)
	}
	if u.Uploaded == 0 {
		t.Error("base uploaded should still be parsed when detail page 404s")
	}
	if u.Level != "" {
		t.Errorf("level should be empty when detail page missing, got %q", u.Level)
	}
}

func TestNexusPHPUserInfoPagesReuseDefaultBonusPerHour(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usercp.php":
			_, _ = w.Write([]byte(`<a href="userdetails.php?id=8">me</a>`))
		case "/userdetails.php":
			_, _ = w.Write([]byte(`<table></table>`))
		case "/bonus.php":
			if r.URL.Query().Get("show") != "description" {
				t.Fatalf("bonus query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`<div>当前魔力值 999999 <div>你当前每小时能获取 77.5 个魔力值</div></div>`))
		case "/mybonus.php":
			t.Fatalf("hard-coded /mybonus.php fallback should not be used when user_info.pages parsed bonus_per_hour")
		case "/getusertorrentlistajax.php":
			_, _ = w.Write([]byte(`<table></table>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)

	u, err := n.FetchUserInfo(context.Background(), &site.SiteMetadata{
		ID:     "custom-bonus-page",
		Schema: "nexusphp",
		UserInfo: site.UserInfoDef{Pages: []site.UserInfoPageDef{{
			Path:      "/bonus.php",
			Params:    map[string]string{"show": "description"},
			UseFields: []string{"bonus_per_hour"},
		}}},
	}, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.BonusPerHour != 77.5 {
		t.Fatalf("bonus_per_hour = %g, want 77.5", u.BonusPerHour)
	}
}

func TestNexusPHPFetchSeedingUsesDefinitionTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom-list.php" {
			t.Fatalf("unexpected path %s", r.URL.String())
		}
		if r.URL.Query().Get("uid") != "42" || r.URL.Query().Get("kind") != "seeding" {
			t.Fatalf("unexpected query %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`<div>3 | 4.00 GiB</div>`))
	}))
	defer srv.Close()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})

	count, size := fetchSeeding(
		context.Background(),
		hc,
		srv.URL,
		"42",
		"cookie",
		"/custom-list.php?uid={id}&kind={type}",
	)
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
	if size != 4*1073741824 {
		t.Fatalf("size = %d, want %d", size, 4*1073741824)
	}
}

func TestNexusPHPFetchSeedingParsesPTTimeOuterSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getusertorrentlist.php" {
			t.Fatalf("unexpected path %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`<div id="outer">
<span>ignore</span><span>ignore</span><span><b>11</b></span><span>总大小 6.50 TiB</span>
<table><tr><td>fallback</td><td>1.00 GiB</td></tr></table>
</div>`))
	}))
	defer srv.Close()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})

	count, size := fetchSeeding(
		context.Background(),
		hc,
		srv.URL,
		"42",
		"cookie",
		"/getusertorrentlist.php?userid={id}&type={type}",
	)
	if count != 11 {
		t.Fatalf("count = %d, want 11", count)
	}
	want := int64(6.5 * float64(1<<40))
	if size != want {
		t.Fatalf("size = %d, want %d", size, want)
	}
}

func TestNexusPHPFetchSeedingUsesSizeColumnOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Referer"); !strings.Contains(got, "/userdetails.php?id=42") {
			t.Fatalf("Referer = %q, want userdetails.php?id=42", got)
		}
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`<table></table>`))
			return
		}
		_, _ = w.Write([]byte(`<table class="torrents">
<tr><th>Name</th><th>Size</th><th>Seeders</th></tr>
<tr><td><a href="details.php?id=1">one</a><span>promo 99 GiB</span></td><td>1.00 GiB</td><td>8</td></tr>
<tr><td><a href="details.php?id=2">two</a><span>note 88 GiB</span></td><td>2.00 GiB</td><td>4</td></tr>
</table>`))
	}))
	defer srv.Close()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})

	count, size := fetchSeeding(
		context.Background(),
		hc,
		srv.URL,
		"42",
		"cookie",
		"/list.php?uid={id}&kind={type}",
	)
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	if size != 3*1073741824 {
		t.Fatalf("size = %d, want %d", size, 3*1073741824)
	}
}

func TestNexusPHPFetchSeedingParsesTotalRow(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<table>
<tr><th>Name</th><th>Count</th><th>Size</th></tr>
<tr><td><a href="details.php?id=1">one</a></td><td></td><td>1.00 GiB</td></tr>
<tr><td>Total</td><td>17</td><td>8.50 TiB</td></tr>
</table>`))
	if err != nil {
		t.Fatalf("parse doc: %v", err)
	}
	count, size := parseTorrentListSize(doc)
	if count != 17 {
		t.Fatalf("count = %d, want 17", count)
	}
	want := int64(8.5 * float64(1<<40))
	if size != want {
		t.Fatalf("size = %d, want %d", size, want)
	}
}

func TestNexusPHPFetchSeedingTotalRowIsSummary(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Query().Get("page") != "" {
			t.Fatalf("summary total row should not request page %s", r.URL.Query().Get("page"))
		}
		_, _ = w.Write([]byte(`<table>
<tr><th>Name</th><th>Count</th><th>Size</th></tr>
<tr><td><a href="details.php?id=1">one</a></td><td></td><td>1.00 GiB</td></tr>
<tr><td>Total</td><td>17</td><td>8.50 TiB</td></tr>
</table>`))
	}))
	defer srv.Close()
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})

	count, size := fetchSeeding(
		context.Background(),
		hc,
		srv.URL,
		"42",
		"cookie",
		"/getusertorrentlistajax.php?userid={id}&type={type}",
	)
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
	if count != 17 {
		t.Fatalf("count = %d, want 17", count)
	}
	want := int64(8.5 * float64(1<<40))
	if size != want {
		t.Fatalf("size = %d, want %d", size, want)
	}
}

func TestNexusPHPDetailSeedingFallback(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		"<table><tr><td>\u505a\u79cd\u7edf\u8ba1</td><td>\u603b\u505a\u79cd\u6570: 9 \u603b\u505a\u79cd\u4f53\u79ef: 6.5 TiB</td></tr></table>",
	))
	if err != nil {
		t.Fatalf("parse detail: %v", err)
	}
	u := &site.UserInfo{}
	fillSeedingFromDetail(doc, u)
	if u.Seeding != 9 {
		t.Fatalf("seeding = %d, want 9", u.Seeding)
	}
	if want := int64(6.5 * float64(1<<40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
