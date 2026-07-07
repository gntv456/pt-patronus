package schemas

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNexusPHPFetchHDUHhanClubHTPTILoliconUserInfoFromMigratedDefinitions(t *testing.T) {
	tests := []struct {
		id                 string
		uid                string
		name               string
		level              string
		joinDate           string
		uploadedTiB        int64
		downloadGiB        int64
		ratio              float64
		bonusLabel         string
		bonus              float64
		bonusPerHour       float64
		rescueBonusPerHour float64
		numericUserID      int
		seeding            int
		seedingTiB         float64
		uploads            int
		bonusPath          string
		useUUID            bool
		hhanClubLayout     bool
	}{
		{
			id:           "hdupt",
			uid:          "801",
			name:         "hdu-user",
			level:        "Power User",
			joinDate:     "2024-01-02",
			uploadedTiB:  2,
			downloadGiB:  512,
			ratio:        4,
			bonusLabel:   "魔力值",
			bonus:        12345.5,
			bonusPerHour: 21.5,
			seeding:      8,
			seedingTiB:   2.25,
			uploads:      3,
			bonusPath:    "/mybonus.php",
		},
		{
			id:                 "hhanclub",
			uid:                "802",
			name:               "hhan-user",
			level:              "PowerUser",
			joinDate:           "2023-03-04",
			uploadedTiB:        3,
			downloadGiB:        768,
			ratio:              4.5,
			bonusLabel:         "憨豆",
			bonus:              23456.75,
			bonusPerHour:       32.25,
			rescueBonusPerHour: 10,
			seeding:            9,
			seedingTiB:         3.5,
			uploads:            4,
			bonusPath:          "/mybonus.php",
			hhanClubLayout:     true,
		},
		{
			id:           "htpt",
			uid:          "803",
			name:         "htpt-user",
			level:        "Elite User",
			joinDate:     "2022-05-06",
			uploadedTiB:  4,
			downloadGiB:  1024,
			ratio:        4,
			bonusLabel:   "魔力值",
			bonus:        34567.25,
			bonusPerHour: 43.75,
			seeding:      10,
			seedingTiB:   4.75,
			uploads:      5,
			bonusPath:    "/mybonus_new.php",
		},
		{
			id:            "ilolicon",
			uid:           "uuid-804",
			name:          "loli-user",
			level:         "高级魔法少女",
			joinDate:      "2021-07-08",
			uploadedTiB:   5,
			downloadGiB:   1280,
			ratio:         4,
			bonusLabel:    "魔力值",
			bonus:         45678.5,
			bonusPerHour:  54.25,
			numericUserID: 1804,
			seeding:       11,
			seedingTiB:    5.5,
			uploads:       6,
			bonusPath:     "/mybonus.php",
			useUUID:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index.php":
					_, _ = w.Write([]byte(indexHTMLWithNumericUserID(tc.uid, tc.name, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus, tc.seeding, tc.useUUID, tc.numericUserID)))
				case "/userdetails.php":
					if tc.useUUID {
						if r.URL.Query().Get("id") != "" {
							http.NotFound(w, r)
							return
						}
						if r.URL.Query().Get("uuid") != tc.uid {
							t.Fatalf("detail uuid = %q, want %s", r.URL.Query().Get("uuid"), tc.uid)
						}
					} else if r.URL.Query().Get("id") != tc.uid {
						t.Fatalf("detail id = %q, want %s", r.URL.Query().Get("id"), tc.uid)
					}
					if tc.hhanClubLayout {
						_, _ = w.Write([]byte(fmt.Sprintf(`
<a href="userdetails.php?id=%s" class="%s_Name">%s</a>
<span>上传量</span><span>%d.00 TiB</span>
<span>下载量</span><span>%d GiB</span>
<span>分享率</span><span>%.3g</span>
<span>加入日期</span><span>%s 09:10:11</span>
<span>%s</span><div>%.2f</div>
<a href="userdetails.php?id=%s&action=2"><div>%d</div></a>`, tc.uid, tc.level, tc.name, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.joinDate, tc.bonusLabel, tc.bonus, tc.uid, tc.seeding)))
						return
					}
					_, _ = w.Write([]byte(detailHTML(tc.level, tc.joinDate, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus)))
				case tc.bonusPath:
					if tc.hhanClubLayout {
						_, _ = w.Write([]byte(fmt.Sprintf(`<div class="grid"><div class="row-span-4">%.2f</div></div>`, tc.bonusPerHour)))
						return
					}
					if tc.bonusPath == "/mybonus_new.php" {
						_, _ = w.Write([]byte(fmt.Sprintf(`<body>每小时 %.2f 魔力值</body>`, tc.bonusPerHour)))
						return
					}
					_, _ = w.Write([]byte(fmt.Sprintf(`<div>你当前每小时能获取 %.2f 个%s</div>`, tc.bonusPerHour, tc.bonusLabel)))
				case "/rescuesettleinfo.php":
					if !tc.hhanClubLayout {
						http.NotFound(w, r)
						return
					}
					_, _ = w.Write([]byte(`<table>
<tr><td><a href="userdetails.php?id=old">old</a></td><td>a</td><td>b</td><td>c</td><td>d</td><td>120</td></tr>
<tr><td>no user link</td><td>a</td><td>b</td><td>c</td><td>d</td><td>999</td></tr>
<tr><td><a href="userdetails.php?id=802">hhan-user</a></td><td>a</td><td>b</td><td>c</td><td>d</td><td>240</td></tr>
</table>`))
				case "/getusertorrentlistajax.php":
					if tc.useUUID {
						if r.URL.Query().Get("useruuid") != tc.uid {
							t.Fatalf("ajax useruuid = %q, want %s", r.URL.Query().Get("useruuid"), tc.uid)
						}
						if !strings.Contains(r.Header.Get("Referer"), "uuid="+tc.uid) {
							t.Fatalf("referer = %q, want uuid referer", r.Header.Get("Referer"))
						}
					} else if r.URL.Query().Get("userid") != tc.uid {
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
			if tc.numericUserID > 0 && u.NumericUserID != tc.numericUserID {
				t.Fatalf("numeric_user_id = %d, want %d", u.NumericUserID, tc.numericUserID)
			}
			if tc.rescueBonusPerHour > 0 && u.RescueSeedingBonusPerHour != tc.rescueBonusPerHour {
				t.Fatalf("rescue_seeding_bonus_per_hour = %g, want %g", u.RescueSeedingBonusPerHour, tc.rescueBonusPerHour)
			}
		})
	}
}

func indexHTML(uid, name string, uploadedTiB, downloadGiB int64, ratio float64, bonusLabel string, bonus float64, seeding int, uuid bool) string {
	return indexHTMLWithNumericUserID(uid, name, uploadedTiB, downloadGiB, ratio, bonusLabel, bonus, seeding, uuid, 0)
}

func indexHTMLWithNumericUserID(uid, name string, uploadedTiB, downloadGiB int64, ratio float64, bonusLabel string, bonus float64, seeding int, uuid bool, numericUserID int) string {
	query := "id=" + uid
	if uuid {
		query = "uuid=" + uid
	}
	inviteLink := ""
	if numericUserID > 0 {
		inviteLink = fmt.Sprintf(`<a href="invite.php?id=%d">邀请</a>`, numericUserID)
	}
	return fmt.Sprintf(`
<div id="info_block">
  <a href="userdetails.php?%s" class="PowerUser_Name">%s</a>
  %s
  上传量 %d.00 TiB 下载量 %d GiB 分享率 %.3g %s: %.2f 当前活动: %d
</div>`, query, name, inviteLink, uploadedTiB, downloadGiB, ratio, bonusLabel, bonus, seeding)
}

func detailHTML(level, joinDate string, uploadedTiB, downloadGiB int64, ratio float64, bonusLabel string, bonus float64) string {
	return fmt.Sprintf(`
<table>
  <tr><td class="rowhead">等级</td><td><img title="%s"></td></tr>
  <tr><td class="rowhead">加入日期</td><td>%s 09:10:11</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 %d.00 TiB 下载量 %d GiB 分享率 %.3g</td></tr>
  <tr><td class="rowhead">%s</td><td>%.2f</td></tr>
</table>`, level, joinDate, uploadedTiB, downloadGiB, ratio, bonusLabel, bonus)
}
