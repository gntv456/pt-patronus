// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchHDVideoUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`
<div id="top-stats-bar"><a class="top-stat" href="mybonus.php"><b>99,001.5</b></a></div>
<div id="info_block"><a href="userdetails.php?id=703">video-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "703" {
				t.Fatalf("detail id = %q, want 703", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Ultimate User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2022-02-03 04:05:06</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 6.00 TiB 下载量 2.00 TiB 分享率 3.0</td></tr>
  <tr><td class="rowhead">电影票</td><td>99,001.5</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`
<h1>每小时获得的合计魔力值</h1>
<div><table><tr><td>基本奖励</td><td>12.5</td></tr></table></div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "703" {
				t.Fatalf("ajax userid = %q, want 703", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>13 | 6.75 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<b>10</b> records`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "hdvideo", srv.URL)
	assertCoreStats(t, u, "703", "video-user", "Ultimate User", "2022-02-03", 6*(1<<40), 2*(1<<40), 3, 99001.5)
	if u.BonusPerHour != 12.5 || u.Seeding != 13 || u.Uploads != 10 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(6.75 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
