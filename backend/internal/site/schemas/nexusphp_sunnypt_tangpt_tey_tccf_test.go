package schemas

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchSunnyPTTangPTTeyTCCFUserInfoFromMigratedDefinitions(t *testing.T) {
	tests := []struct {
		id           string
		uid          string
		name         string
		level        string
		joinDate     string
		uploadedTiB  int64
		downloadGiB  int64
		ratio        float64
		bonus        float64
		bonusPerHour float64
		seeding      int
		seedingTiB   float64
		uploads      int
	}{
		// 注：sunnypt 已于 2026 年从 NexusPHP 整体改版为 Next.js SPA（schema=tnode），不再走 HTML 解析；
		// 其用户信息抓取由 schemas/sunnypt.go 的 def.ID=="sunnypt" 分支短路（Bearer JSON API），
		// 故从此 NexusPHP HTML 迁移定义测试中移除。tangpt/tey/tccf 仍为 NexusPHP，保留。
		{
			id:           "tangpt",
			uid:          "1402",
			name:         "tang-user",
			level:        "Elite User",
			joinDate:     "2023-03-04",
			uploadedTiB:  3,
			downloadGiB:  768,
			ratio:        4.5,
			bonus:        23456.75,
			bonusPerHour: 32.25,
			seeding:      9,
			seedingTiB:   3.5,
			uploads:      4,
		},
		{
			id:           "tey",
			uid:          "1403",
			name:         "tey-user",
			level:        "Crazy User",
			joinDate:     "2022-05-06",
			uploadedTiB:  4,
			downloadGiB:  1024,
			ratio:        4,
			bonus:        34567.25,
			bonusPerHour: 43.75,
			seeding:      10,
			seedingTiB:   4.75,
			uploads:      5,
		},
		{
			id:           "tccf",
			uid:          "1404",
			name:         "tccf-user",
			level:        "士官",
			joinDate:     "2021-07-08",
			uploadedTiB:  5,
			downloadGiB:  1280,
			ratio:        4,
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
					_, _ = w.Write([]byte(indexHTML(tc.uid, tc.name, tc.uploadedTiB, tc.downloadGiB, tc.ratio, "魔力值", tc.bonus, tc.seeding, false)))
				case "/userdetails.php":
					if r.URL.Query().Get("id") != tc.uid {
						t.Fatalf("detail id = %q, want %s", r.URL.Query().Get("id"), tc.uid)
					}
					_, _ = w.Write([]byte(detailHTML(tc.level, tc.joinDate, tc.uploadedTiB, tc.downloadGiB, tc.ratio, "魔力值", tc.bonus)))
				case "/mybonus.php":
					_, _ = w.Write([]byte(fmt.Sprintf(`<div>你当前每小时能获取 %.2f 个魔力值</div>`, tc.bonusPerHour)))
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
