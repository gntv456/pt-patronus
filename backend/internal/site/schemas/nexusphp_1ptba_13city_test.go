package schemas

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetch1PTBAUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block"><a href="userdetails.php?id=705">bar-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "705" {
				t.Fatalf("detail id = %q, want 705", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Elite User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2024-04-05 06:07:08</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 3.00 TiB 下载量 1.00 TiB 分享率 3.0</td></tr>
  <tr><td class="rowhead">魔力值</td><td>44,444.25</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 22.5 个魔力值</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "705" {
				t.Fatalf("ajax userid = %q, want 705", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>15 | 3.25 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<b>7</b> records`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "1ptba", srv.URL)
	assertCoreStats(t, u, "705", "bar-user", "Elite User", "2024-04-05", 3*(1<<40), 1*(1<<40), 3, 44444.25)
	if u.BonusPerHour != 22.5 || u.Seeding != 15 || u.Uploads != 7 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(3.25 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestNexusPHPFetch13CityUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`<div id="info_block"><a href="userdetails.php?id=706">city-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "706" {
				t.Fatalf("detail id = %q, want 706", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="懵懂求知"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2023-10-09 08:07:06</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 4.00 TiB 下载量 2.00 TiB 分享率 2.0</td></tr>
  <tr><td class="rowhead">啤酒瓶</td><td>55,555.5</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`
<h1>每小时获得的合计啤酒瓶</h1>
<div><table><tr><td>基本奖励</td><td>19.75</td></tr></table></div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "706" {
				t.Fatalf("ajax userid = %q, want 706", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>16 | 4.75 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<div data-count="9"></div>`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "13city", srv.URL)
	assertCoreStats(t, u, "706", "city-user", "懵懂求知", "2023-10-09", 4*(1<<40), 2*(1<<40), 2, 55555.5)
	if u.BonusPerHour != 19.75 || u.Seeding != 16 || u.Uploads != 9 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(4.75 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
