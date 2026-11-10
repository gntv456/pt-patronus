// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/site"
)

func TestNexusPHPFetchHDDolbyUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block"><a href="userdetails.php?id=704">dolby-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "704" {
				t.Fatalf("detail id = %q, want 704", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Extreme User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2021-12-11 10:09:08</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 8.00 TiB 下载量 2.00 TiB 分享率 4.0</td></tr>
  <tr><td class="rowhead">鲸币</td><td>123,456.75</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<table width="800"><tbody><tr><td>合计</td><td>66.6/小时</td></tr></tbody></table>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "704" {
				t.Fatalf("ajax userid = %q, want 704", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>14 | 7.25 TiB</div>`))
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

	u := fetchTestUserInfo(t, "hddolby", srv.URL)
	assertCoreStats(t, u, "704", "dolby-user", "Extreme User", "2021-12-11", 8*(1<<40), 2*(1<<40), 4, 123456.75)
	if u.BonusPerHour != 66.6 || u.Seeding != 14 || u.Uploads != 11 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(7.25 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestJoyHDDefinitionMarkedDead(t *testing.T) {
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("joyhd")
	if !ok {
		t.Fatal("joyhd definition missing")
	}
	if !def.Dead {
		t.Fatal("joyhd should be retained as a dead definition")
	}
	if !site.EnsureDefaults(def).Dead {
		t.Fatal("EnsureDefaults should preserve joyhd dead marker")
	}
}
