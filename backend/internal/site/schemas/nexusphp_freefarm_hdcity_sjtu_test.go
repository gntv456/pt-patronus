// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchFreeFarmUserInfoFromMigratedDefinition(t *testing.T) {
	const (
		uid          = "3101"
		name         = "freefarm-user"
		uploadedTiB  = int64(2)
		downloadGiB  = int64(512)
		ratio        = 4.0
		bonus        = 12345.5
		bonusPerHour = 18.75
		seeding      = 7
		seedingTiB   = 2.5
		uploads      = 4
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(indexHTML(uid, name, uploadedTiB, downloadGiB, ratio, "魔力值", bonus, seeding, false)))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != uid {
				t.Fatalf("detail id = %q, want %s", r.URL.Query().Get("id"), uid)
			}
			_, _ = w.Write([]byte(detailHTML("Power User", "2024-01-02", uploadedTiB, downloadGiB, ratio, "魔力值", bonus)))
		case "/mybonus.php":
			_, _ = w.Write([]byte(fmt.Sprintf(`<table><tr><td rowspan="3">%.2f</td></tr></table>`, bonusPerHour)))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != uid {
				t.Fatalf("ajax userid = %q, want %s", r.URL.Query().Get("userid"), uid)
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(fmt.Sprintf(`<div>%d | %.2f TiB</div>`, seeding, seedingTiB)))
			case "uploaded":
				_, _ = w.Write([]byte(fmt.Sprintf(`<b>%d</b> records`, uploads)))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "freefarm", srv.URL)
	assertCoreStats(t, u, uid, name, "Power User", "2024-01-02", uploadedTiB*(1<<40), downloadGiB*(1<<30), ratio, bonus)
	if u.BonusPerHour != bonusPerHour || u.Seeding != seeding || u.Uploads != uploads {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(seedingTiB * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestNexusPHPFetchHDCityUserInfoFromMigratedDefinition(t *testing.T) {
	const (
		uid          = "3202"
		name         = "city-user"
		bonus        = 12345.5
		bonusPerHour = 28.5
		seeding      = 9
		seedingTiB   = 3.25
		uploads      = 6
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/userdetails":
			_, _ = w.Write([]byte(fmt.Sprintf(`
<center>
  <div class="text_alt">
    <span style="font-weight:bold">%s</span>
    <img src="/pic/class/3.gif">
    <span class="text">ID: %s</span>
  </div><br><div>
    <span class="text">上传量: 5.00 TiB 下载量: 1.25 TiB</span>
    <span class="text">占位</span>
    <span class="text">占位</span>
    <span class="text">Ratio: 4.0 魅力值 %.2f</span>
    <span class="text">加入日期 2023-04-05 06:07:08</span>
  </div>
</center>`, name, uid, bonus)))
		case "/mybonus":
			_, _ = w.Write([]byte(fmt.Sprintf(`<div>你当前每小时能获取 %.2f 个魅力值</div>`, bonusPerHour)))
		case "/getusertorrentlistajax":
			if r.URL.Query().Get("userid") != uid {
				t.Fatalf("ajax userid = %q, want %s", r.URL.Query().Get("userid"), uid)
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(fmt.Sprintf(`<div data-count="%d">%d | %.2f TiB</div>`, seeding, seeding, seedingTiB)))
			case "uploaded":
				_, _ = w.Write([]byte(fmt.Sprintf(`<div data-count="%d"></div>`, uploads)))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "hdcity", srv.URL)
	assertCoreStats(t, u, uid, name, "权天使", "2023-04-05", 5*(1<<40), int64(1.25*float64(1<<40)), 4, bonus)
	if u.BonusPerHour != bonusPerHour || u.Seeding != seeding || u.Uploads != uploads {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(seedingTiB * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestNexusPHPFetchSJTUUserInfoFromMigratedDefinition(t *testing.T) {
	const (
		uid          = "3303"
		name         = "sjtu-user"
		uploadedTiB  = int64(4)
		downloadGiB  = int64(1024)
		ratio        = 4.0
		bonus        = 34567.25
		bonusPerHour = 38.25
		uploads      = 8
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(indexHTML(uid, name, uploadedTiB, downloadGiB, ratio, "魔力值", bonus, 0, false)))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != uid {
				t.Fatalf("detail id = %q, want %s", r.URL.Query().Get("id"), uid)
			}
			_, _ = w.Write([]byte(detailHTML("Elite User", "2022-06-07", uploadedTiB, downloadGiB, ratio, "魔力值", bonus)))
		case "/mybonus.php":
			_, _ = w.Write([]byte(fmt.Sprintf(`<div>你当前每小时能获取 %.2f 个魔力值</div>`, bonusPerHour)))
		case "/viewusertorrents.php":
			if r.URL.Query().Get("id") != uid || r.URL.Query().Get("show") != "seeding" {
				t.Fatalf("seeding query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`<table>
<tr><th>标题</th><th>状态</th><th>大小</th></tr>
<tr><td>A</td><td>做种</td><td>1.50 TiB</td></tr>
<tr><td>B</td><td>做种</td><td>512 GiB</td></tr>
</table>`))
		case "/viewtorrentsdetail.php":
			if r.URL.Query().Get("id") != uid {
				t.Fatalf("uploads id = %q, want %s", r.URL.Query().Get("id"), uid)
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`<a href="userdetails.php?id=%s">%s</a> %d <div id="ka"></div>`, uid, name, uploads)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "sjtu", srv.URL)
	assertCoreStats(t, u, uid, name, "Elite User", "2022-06-07", uploadedTiB*(1<<40), downloadGiB*(1<<30), ratio, bonus)
	if u.BonusPerHour != bonusPerHour || u.Seeding != 2 || u.Uploads != uploads {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(2 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
