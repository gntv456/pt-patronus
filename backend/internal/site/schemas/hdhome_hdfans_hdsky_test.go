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

func TestNexusPHPFetchHDHomeUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block"><a href="userdetails.php?id=77">home-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "77" {
				t.Fatalf("detail id = %q, want 77", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Crazy User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2023-03-04 05:06:07</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 512 GiB 分享率 4.0</td></tr>
  <tr><td class="rowhead">魔力值</td><td>88,000.5</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<table><tr><td class="text"><h1>魔力值</h1>
<div align="center">1</div><div align="center">2</div><div align="center">3</div><div align="center">4</div><div align="center">5</div>
<div align="center">6</div><div align="center">7</div><div align="center">8</div><div align="center">9</div><div align="center">10</div>
</td></tr></table>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "77" {
				t.Fatalf("ajax userid = %q, want 77", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>9 | 2.25 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<b>6</b> records`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "hdhome", srv.URL)
	assertCoreStats(t, u, "77", "home-user", "Crazy User", "2023-03-04", 2*(1<<40), 512*(1<<30), 4, 88000.5)
	if u.BonusPerHour != 16 || u.Seeding != 9 || u.Uploads != 6 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(2.25 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestNexusPHPFetchHDFansUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block"><a href="userdetails.php?id=88">fans-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "88" {
				t.Fatalf("detail id = %q, want 88", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Elite User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2022-08-09 10:11:12</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 3.00 TiB 下载量 1.00 TiB 分享率 3.0</td></tr>
  <tr><td class="rowhead">魔力值</td><td>66,000.25</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>每小时获得的合计魔力值 88.5</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "88" {
				t.Fatalf("ajax userid = %q, want 88", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div data-count="12">12 | 4.50 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div data-count="3"></div>`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "hdfans", srv.URL)
	assertCoreStats(t, u, "88", "fans-user", "Elite User", "2022-08-09", 3*(1<<40), 1*(1<<40), 3, 66000.25)
	if u.BonusPerHour != 88.5 || u.Seeding != 12 || u.Uploads != 3 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(4.5 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestNexusPHPFetchHDSkyUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block"><a href="userdetails.php?id=99">sky-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "99" {
				t.Fatalf("detail id = %q, want 99", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Power User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2025-05-06 07:08:09</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 4.00 TiB 下载量 2.00 TiB 分享率 2.0</td></tr>
  <tr><td class="rowhead">魔力</td><td>55,000</td></tr>
  <tr><td class="rowhead">当前做种</td><td>总做种 5 做种体积 1.25 TiB</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 45.25 个魔力</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "99" {
				t.Fatalf("ajax userid = %q, want 99", r.URL.Query().Get("userid"))
			}
			if r.URL.Query().Get("type") != "uploaded" {
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
			_, _ = w.Write([]byte(`<b>2</b> records`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "hdsky", srv.URL)
	assertCoreStats(t, u, "99", "sky-user", "Power User", "2025-05-06", 4*(1<<40), 2*(1<<40), 2, 55000)
	if u.BonusPerHour != 45.25 || u.Seeding != 5 || u.Uploads != 2 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(1.25 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func fetchTestUserInfo(t *testing.T, id, baseURL string) *site.UserInfo {
	t.Helper()
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition(id)
	if !ok {
		t.Fatalf("%s definition missing", id)
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u, err := NewNexusPHP(hc).FetchUserInfo(context.Background(), def, baseURL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo(%s): %v", id, err)
	}
	return u
}

func assertCoreStats(t *testing.T, u *site.UserInfo, id, name, level, join string, uploaded, downloaded int64, ratio, bonus float64) {
	t.Helper()
	if u.ID != id || u.Name != name || u.Level != level || u.JoinDate != join {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.Uploaded != uploaded {
		t.Fatalf("uploaded = %d, want %d", u.Uploaded, uploaded)
	}
	if u.Downloaded != downloaded {
		t.Fatalf("downloaded = %d, want %d", u.Downloaded, downloaded)
	}
	if u.Ratio != ratio || u.Bonus != bonus {
		t.Fatalf("ratio/bonus = %g/%g, want %g/%g", u.Ratio, u.Bonus, ratio, bonus)
	}
}
