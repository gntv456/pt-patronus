package schemas

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchLongPTLuckPTMomentPTMuxuegeUserInfoFromMigratedDefinitions(t *testing.T) {
	tests := []struct {
		id           string
		uid          string
		name         string
		level        string
		joinDate     string
		uploadedTiB  int64
		downloadGiB  int64
		ratio        float64
		bonusLabel   string
		bonus        float64
		bonusPerHour float64
		seeding      int
		seedingTiB   float64
		uploads      int
	}{
		{
			id:           "longpt",
			uid:          "701",
			name:         "long-user",
			level:        "甲龙",
			joinDate:     "2024-02-03",
			uploadedTiB:  2,
			downloadGiB:  512,
			ratio:        4,
			bonusLabel:   "魔力值",
			bonus:        12345.5,
			bonusPerHour: 21.5,
			seeding:      8,
			seedingTiB:   2.25,
			uploads:      3,
		},
		{
			id:           "luckpt",
			uid:          "702",
			name:         "luck-user",
			level:        "Elite User",
			joinDate:     "2023-04-05",
			uploadedTiB:  3,
			downloadGiB:  768,
			ratio:        4.5,
			bonusLabel:   "幸运星",
			bonus:        23456.75,
			bonusPerHour: 32.25,
			seeding:      9,
			seedingTiB:   3.5,
			uploads:      4,
		},
		{
			id:           "momentpt",
			uid:          "703",
			name:         "moment-user",
			level:        "追光者",
			joinDate:     "2022-06-07",
			uploadedTiB:  4,
			downloadGiB:  1024,
			ratio:        4,
			bonusLabel:   "魔力值",
			bonus:        34567.25,
			bonusPerHour: 43.75,
			seeding:      10,
			seedingTiB:   4.75,
			uploads:      5,
		},
		{
			id:           "muxuege",
			uid:          "704",
			name:         "muxue-user",
			level:        "Power User",
			joinDate:     "2021-08-09",
			uploadedTiB:  5,
			downloadGiB:  1280,
			ratio:        4,
			bonusLabel:   "魔力值",
			bonus:        45678.5,
			bonusPerHour: 54.25,
			seeding:      11,
			seedingTiB:   5.5,
			uploads:      6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index.php":
					_, _ = w.Write([]byte(fmt.Sprintf(`
<div id="info_block">
  <a href="userdetails.php?id=%s">%s</a>
  上传量 %d.00 TiB 下载量 %d GiB 分享率 %.3g %s: %.2f 当前活动: %d
</div>`, tc.uid, tc.name, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus, tc.seeding)))
				case "/userdetails.php":
					if r.URL.Query().Get("id") != tc.uid {
						t.Fatalf("detail id = %q, want %s", r.URL.Query().Get("id"), tc.uid)
					}
					_, _ = w.Write([]byte(fmt.Sprintf(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="%s"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>%s 09:10:11</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 %d.00 TiB 下载量 %d GiB 分享率 %.3g</td></tr>
  <tr><td class="rowhead">%s</td><td>%.2f</td></tr>
</table>`, tc.level, tc.joinDate, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus)))
				case "/mybonus.php":
					_, _ = w.Write([]byte(fmt.Sprintf(`<div>你当前每小时能获取 %.2f 个%s</div>`, tc.bonusPerHour, tc.bonusLabel)))
				case "/getusertorrentlistajax.php":
					if r.URL.Query().Get("userid") != tc.uid {
						t.Fatalf("ajax userid = %q, want %s", r.URL.Query().Get("userid"), tc.uid)
					}
					switch r.URL.Query().Get("type") {
					case "seeding":
						_, _ = w.Write([]byte(fmt.Sprintf(`<div>%d | %.2f TiB</div>`, tc.seeding, tc.seedingTiB)))
					case "uploaded":
						_, _ = w.Write([]byte(fmt.Sprintf(`<b>%d</b> records`, tc.uploads)))
					default:
						t.Fatalf("unexpected ajax type %q", r.URL.Query().Get("type"))
					}
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			u := fetchTestUserInfo(t, tc.id, srv.URL)
			assertCoreStats(t, u, tc.uid, tc.name, tc.level, tc.joinDate, tc.uploadedTiB*(1<<40), tc.downloadGiB*(1<<30), tc.ratio, tc.bonus)
			if u.BonusPerHour != tc.bonusPerHour || u.Seeding != tc.seeding || u.Uploads != tc.uploads {
				t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
			}
			if want := int64(tc.seedingTiB * (1 << 40)); u.SeedingSize != want {
				t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
			}
		})
	}
}
