package schemas

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchXingTanXingYunGeZMPTUserInfoFromMigratedDefinitions(t *testing.T) {
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
		bonusHTML    string
	}{
		{
			id:           "xingtan",
			uid:          "1601",
			name:         "xingtan-user",
			level:        "Power User",
			joinDate:     "2024-01-02",
			uploadedTiB:  2,
			downloadGiB:  512,
			ratio:        4,
			bonusLabel:   "杏仁值",
			bonus:        12345.5,
			bonusPerHour: 21.5,
			seeding:      8,
			seedingTiB:   2.25,
			uploads:      3,
			bonusHTML:    `<div>你当前每小时能获取 21.50 个杏仁值</div>`,
		},
		{
			id:           "xingyunge",
			uid:          "1602",
			name:         "xyg-user",
			level:        "斗王",
			joinDate:     "2023-03-04",
			uploadedTiB:  3,
			downloadGiB:  768,
			ratio:        4.5,
			bonusLabel:   "星焱",
			bonus:        23456.75,
			bonusPerHour: 32.25,
			seeding:      9,
			seedingTiB:   3.5,
			uploads:      4,
			bonusHTML:    `<table><tr><td class="text">32.25</td></tr></table>`,
		},
		{
			id:           "zmpt",
			uid:          "1603",
			name:         "zmpt-user",
			level:        "道馆训练家(IU)",
			joinDate:     "2022-05-06",
			uploadedTiB:  4,
			downloadGiB:  1024,
			ratio:        4,
			bonusLabel:   "电力值",
			bonus:        34567.25,
			bonusPerHour: 43.75,
			seeding:      10,
			seedingTiB:   4.75,
			uploads:      5,
			bonusHTML:    `<table><tr><td rowspan="4">43.75</td></tr></table>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index.php":
					_, _ = w.Write([]byte(indexHTML(tc.uid, tc.name, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus, tc.seeding, false)))
				case "/userdetails.php":
					if r.URL.Query().Get("id") != tc.uid {
						t.Fatalf("detail id = %q, want %s", r.URL.Query().Get("id"), tc.uid)
					}
					_, _ = w.Write([]byte(detailHTML(tc.level, tc.joinDate, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus)))
				case "/mybonus.php":
					_, _ = w.Write([]byte(tc.bonusHTML))
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
