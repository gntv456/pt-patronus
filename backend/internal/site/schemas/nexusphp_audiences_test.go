package schemas

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchAudiencesUserInfoFromMigratedDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`
<div class="site-userbar__name"><a href="userdetails.php?id=909">aud-user</a></div>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "909" {
				t.Fatalf("detail id = %q, want 909", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<div class="ud-hero">
  <span class="ud-class"><b class="CrazyUser_Name">(街头霸王)Crazy User</b></span>
  <div class="ud-hero__meta"><span><i class="far fa-calendar-plus"></i>加入日期：2024-12-05 11:50:05</span></div>
</div>
<div class="ud-stats">
  <div class="ud-stat"><div class="ud-stat__body"><div class="ud-stat__value">2.778 TB</div><div class="ud-stat__label">上传量</div></div></div>
  <div class="ud-stat"><div class="ud-stat__body"><div class="ud-stat__value">508.31 GB</div><div class="ud-stat__label">下载量</div></div></div>
  <div class="ud-stat"><div class="ud-stat__body"><div class="ud-stat__value">5.595</div><div class="ud-stat__label">分享率</div></div></div>
  <div class="ud-stat"><div class="ud-stat__body"><div class="ud-stat__value">564,831.6</div><div class="ud-stat__label">爆米花</div></div></div>
</div>
<span class="site-userbar__compact-metric site-userbar__compact-metric--active" title="活跃 ↑ 75 / ↓ 0"></span>`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(`<div class="mybonus-side__rate">你当前每小时能获取 44.5 粒爆米花</div>`))
		case "/getusertorrentlistajax.php":
			if r.URL.Query().Get("userid") != "909" {
				t.Fatalf("ajax userid = %q, want 909", r.URL.Query().Get("userid"))
			}
			switch r.URL.Query().Get("type") {
			case "seeding":
				_, _ = w.Write([]byte(`
<table>
  <tr><th>名称</th><th>数量</th><th>大小</th></tr>
  <tr><td><a href="details.php?id=1">one</a></td><td></td><td>1.00 GiB</td></tr>
  <tr><td>Total</td><td>75</td><td>7.25 TiB</td></tr>
</table>`))
			case "uploaded":
				_, _ = w.Write([]byte(`<b>12</b> records`))
			default:
				t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "audiences", srv.URL)
	assertCoreStats(t, u, "909", "aud-user", "(街头霸王)Crazy User", "2024-12-05", 3054443301961, 545793706557, 5.595, 564831.6)
	if u.BonusPerHour != 44.5 || u.Seeding != 75 || u.Uploads != 12 {
		t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
	}
	if want := int64(7.25 * (1 << 40)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
