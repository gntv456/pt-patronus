package schemas

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchCBGUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`
<div id="info_block">
  <a class="User_Name" href="userdetails.php?id=333">cbg-user</a>
  上传量 2.00 TiB 下载量 1.00 TiB 分享率 2.0 魔力值: 33,333.3 当前活动: 14
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "333" {
				t.Fatalf("detail id = %q, want 333", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="熟练巧匠"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2024-03-04 05:06:07</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 1.00 TiB 分享率 2.0</td></tr>
  <tr><td class="rowhead">魔力值</td><td>33,333.3</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`
<table>
  <tr><th>项目</th><th>点数</th><th>合计</th></tr>
  <tr><td>基本奖励</td><td>1.0</td><td rowspan="4">36.5</td></tr>
</table>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "333" {
				t.Fatalf("ajax userid = %q, want 333", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>14 | 4.25 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<b>4</b> records`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "cbg", srv.URL)
	assertCoreStats(t, u, "333", "cbg-user", "熟练巧匠", "2024-03-04", 2*(1<<40), 1*(1<<40), 2, 33333.3)
	if u.BonusPerHour != 36.5 || u.Seeding != 14 || u.Uploads != 4 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(4.25 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestNexusPHPFetchCHDBitsUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`
<div id="info_block">
  <a href="userdetails.php?id=444">chd-user</a>
  上传量 6.00 TiB 下载量 2.00 TiB 分享率 3.0 魔力值: 88,888.8 当前活动: 16
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "444" {
				t.Fatalf("detail id = %q, want 444", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">用户等级</td><td><img title="Power User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2020-10-11 12:13:14</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 6.00 TiB 下载量 2.00 TiB 分享率 3.0</td></tr>
  <tr><td class="rowhead">魔力值</td><td>88,888.8</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div>你当前每小时能获取 55.75 个魔力值</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "444" {
				t.Fatalf("ajax userid = %q, want 444", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`<div>16 | 7.00 TiB</div>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<b>13</b> records`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "chdbits", srv.URL)
	assertCoreStats(t, u, "444", "chd-user", "Power User", "2020-10-11", 6*(1<<40), 2*(1<<40), 3, 88888.8)
	if u.BonusPerHour != 55.75 || u.Seeding != 16 || u.Uploads != 13 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(7 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
