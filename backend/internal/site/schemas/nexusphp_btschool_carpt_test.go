package schemas

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchBTSchoolUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`
<div id="info_block">
  <a href="userdetails.php?id=111">bt-user</a>
  上传量 1.50 TiB 下载量 500 GiB 分享率 3.0 魔力值: 12,345.5 当前活动: 4
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "111" {
				t.Fatalf("detail id = %q, want 111", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Elite User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2022-01-02 03:04:05</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 1.50 TiB 下载量 500 GiB 分享率 3.0</td></tr>
  <tr><td class="rowhead">魔力值</td><td>12,345.5</td></tr>
  <tr><td class="rowhead">当前做种</td><td>(9 个种子，共计 2.50 TiB)</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<table><tr><td class="text">你当前每小时能获取 21.25 个魔力值</td></tr></table>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "111" {
				t.Fatalf("ajax userid = %q, want 111", r.URL.Query().Get("userid"))
			}
			if r.URL.Query().Get("type") != "uploaded" {
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
			_, _ = w.Write([]byte(`<b>7</b> records`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "btschool", srv.URL)
	assertCoreStats(t, u, "111", "bt-user", "Elite User", "2022-01-02", int64(1.5*(1<<40)), 500*(1<<30), 3, 12345.5)
	if u.BonusPerHour != 21.25 || u.Seeding != 9 || u.Uploads != 7 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(2.5 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestNexusPHPFetchCarPTUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`
<div id="info_block">
  <a href="userdetails.php?id=222">car-user</a>
  上传量 4.00 TiB 下载量 1.00 TiB 分享率 4.0 魔力值: 66,666.6
</div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "222" {
				t.Fatalf("detail id = %q, want 222", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="Crazy User"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2021-06-07 08:09:10</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 4.00 TiB 下载量 1.00 TiB 分享率 4.0</td></tr>
  <tr><td class="rowhead">魔力值</td><td>66,666.6</td></tr>
</table>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`
<div id="outer">
  <h1>每小时获得的合计魔力值</h1>
  <div><table cellpadding="5">
    <tr><th>项目</th><th>点数</th><th>合计</th></tr>
    <tr><td>基本奖励</td><td>1.0</td><td rowspan="10">42.75</td></tr>
  </table></div>
</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "222" {
				t.Fatalf("ajax userid = %q, want 222", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`
<table>
  <tr><th>名称</th><th>数量</th><th>大小</th></tr>
  <tr><td><a href="details.php?id=1">one</a></td><td></td><td>1.00 GiB</td></tr>
  <tr><td>Total</td><td>15</td><td>3.75 TiB</td></tr>
</table>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<b>5</b> records`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "carpt", srv.URL)
	assertCoreStats(t, u, "222", "car-user", "Crazy User", "2021-06-07", 4*(1<<40), 1*(1<<40), 4, 66666.6)
	if u.BonusPerHour != 42.75 || u.Seeding != 15 || u.Uploads != 5 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(3.75 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
