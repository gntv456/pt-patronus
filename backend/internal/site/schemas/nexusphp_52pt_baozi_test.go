// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetch52PTUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block"><a href="userdetails.php?id=707">school-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "707" {
				t.Fatalf("detail id = %q, want 707", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Power User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2024-07-08 09:10:11</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 5.00 TiB 下载量 2.00 TiB 分享率 2.5</td></tr>
  <tr><td class="rowhead">魔力值</td><td>77,777.75</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<table><tr><td class="text">你当前每小时能获取 23.5 个魔力值</td></tr></table>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "707" {
				t.Fatalf("ajax userid = %q, want 707", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>17 | 5.25 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<b>8</b> records`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "52pt", srv.URL)
	assertCoreStats(t, u, "707", "school-user", "Power User", "2024-07-08", 5*(1<<40), 2*(1<<40), 2.5, 77777.75)
	if u.BonusPerHour != 23.5 || u.Seeding != 17 || u.Uploads != 8 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(5.25 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestNexusPHPFetchBaoziUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`
<div id="welcome_text"><span class="nowrap"><a href="userdetails.php?id=808">baozi-user</a></span></div>
<div id="menu_lists_text"><div>分享率：3.0</div></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "808" {
				t.Fatalf("detail id = %q, want 808", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Elite User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2023-02-03 04:05:06</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 6.00 TiB 下载量 2.00 TiB 分享率 3.0</td></tr>
  <tr><td class="rowhead">魔力值</td><td>88,888.8</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`
<h1>每小时获得的合计魔力值</h1>
<div><table><tr><td>基本奖励</td><td>31.25</td></tr></table></div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "808" {
				t.Fatalf("ajax userid = %q, want 808", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div data-count="19">19 | 6.50 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<b>11</b> records`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "baozi", srv.URL)
	assertCoreStats(t, u, "808", "baozi-user", "Elite User", "2023-02-03", 6*(1<<40), 2*(1<<40), 3, 88888.8)
	if u.BonusPerHour != 31.25 || u.Seeding != 19 || u.Uploads != 11 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(6.5 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
