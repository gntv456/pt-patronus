// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchPTerPTFansPTGTKPTLGSUserInfoFromMigratedDefinitions(t *testing.T) {
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
			id:           "pter",
			uid:          "1101",
			name:         "pter-user",
			level:        "加菲猫 POWER USER",
			joinDate:     "2024-01-02",
			uploadedTiB:  2,
			downloadGiB:  512,
			ratio:        4,
			bonusLabel:   "猫粮",
			bonus:        12345.5,
			bonusPerHour: 21.5,
			seeding:      8,
			seedingTiB:   2.25,
			uploads:      7,
		},
		{
			id:           "ptfans",
			uid:          "1102",
			name:         "ptfans-user",
			level:        "Power User",
			joinDate:     "2023-03-04",
			uploadedTiB:  3,
			downloadGiB:  768,
			ratio:        4.5,
			bonusLabel:   "魔力值",
			bonus:        23456.75,
			bonusPerHour: 32.25,
			seeding:      9,
			seedingTiB:   3.5,
			uploads:      4,
		},
		{
			id:           "ptgtk",
			uid:          "1103",
			name:         "ptgtk-user",
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
		},
		{
			id:           "ptlgs",
			uid:          "1104",
			name:         "ptlgs-user",
			level:        "Power User",
			joinDate:     "2021-07-08",
			uploadedTiB:  5,
			downloadGiB:  1280,
			ratio:        4,
			bonusLabel:   "工分",
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
					_, _ = w.Write([]byte(indexHTML(tc.uid, tc.name, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus, tc.seeding, false)))
				case "/userdetails.php":
					if r.URL.Query().Get("id") != tc.uid {
						t.Fatalf("detail id = %q, want %s", r.URL.Query().Get("id"), tc.uid)
					}
					_, _ = w.Write([]byte(detailHTML(tc.level, tc.joinDate, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus)))
				case "/mybonus.php":
					_, _ = w.Write([]byte(fmt.Sprintf(`<div>你当前每小时能获取 %.2f 个%s</div>`, tc.bonusPerHour, tc.bonusLabel)))
				case "/torrents.php":
					if tc.id != "pter" {
						http.NotFound(w, r)
						return
					}
					if got := r.URL.Query().Get("search"); got != tc.name {
						t.Fatalf("upload search = %q, want %s", got, tc.name)
					}
					switch r.URL.Query().Get("check") {
					case "prechecked":
						_, _ = w.Write([]byte(`<table class="torrents"><tbody><tr><th>标题</th></tr><tr><td>A</td></tr><tr><td>B</td></tr></tbody></table>`))
					case "checked":
						_, _ = w.Write([]byte(`<p class="np-pager"><b>1</b><b>5</b></p>`))
					default:
						t.Fatalf("unexpected pter upload check %q", r.URL.Query().Get("check"))
					}
				case "/getusertorrentlistajax.php":
					if r.URL.Query().Get("userid") != tc.uid {
						t.Fatalf("ajax userid = %q, want %s", r.URL.Query().Get("userid"), tc.uid)
					}
					switch r.URL.Query().Get("type") {
					case "seeding":
						_, _ = w.Write([]byte(fmt.Sprintf(`<div>%d | %.2f TiB</div>`, tc.seeding, tc.seedingTiB)))
					case "uploaded":
						if tc.id == "pter" {
							t.Fatalf("pter uploads should come from torrents.php uploadCountAdd pages, not uploaded ajax fallback")
						}
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
