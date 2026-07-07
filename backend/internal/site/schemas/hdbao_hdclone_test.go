package schemas

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchHDBaoUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block"><a href="userdetails.php?id=701">bao-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "701" {
				t.Fatalf("detail id = %q, want 701", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Crazy User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2024-01-02 03:04:05</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 512 GiB 分享率 4.0</td></tr>
  <tr><td class="rowhead">魔力值</td><td>77,777.25</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<table><tr><td>基本奖励</td><td>7.25</td></tr></table>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "701" {
				t.Fatalf("ajax userid = %q, want 701", r.URL.Query().Get("userid"))
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

	u := fetchTestUserInfo(t, "hdbao", srv.URL)
	assertCoreStats(t, u, "701", "bao-user", "Crazy User", "2024-01-02", 2*(1<<40), 512*(1<<30), 4, 77777.25)
	if u.BonusPerHour != 7.25 || u.Seeding != 9 || u.Uploads != 6 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(2.25 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestNexusPHPFetchHDCloneUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block"><a href="userdetails.php?id=702">clone-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "702" {
				t.Fatalf("detail id = %q, want 702", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Elite User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2023-09-08 07:06:05</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量: 5.00 TiB 下载量: 1.25 TiB 分享率: 4.0</td></tr>
  <tr><td class="rowhead">魔力值</td><td>88,888.5</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 33.75 个魔力值</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "702" {
				t.Fatalf("ajax userid = %q, want 702", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div data-count="11">11 | 3.50 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div data-count="8"></div>`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "hdclone", srv.URL)
	assertCoreStats(t, u, "702", "clone-user", "Elite User", "2023-09-08", 5*(1<<40), int64(1.25*(1<<40)), 4, 88888.5)
	if u.BonusPerHour != 33.75 || u.Seeding != 11 || u.Uploads != 8 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(3.5 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
