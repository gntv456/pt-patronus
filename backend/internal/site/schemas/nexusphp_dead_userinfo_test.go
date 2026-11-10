// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/site"
)

func TestNexusPHPDeadDefinitionsUserInfoOverrides(t *testing.T) {
	tests := []struct {
		id             string
		uid            string
		nameHTML       string
		wantName       string
		levelCell      string
		wantLevel      string
		joinDate       string
		bonusLabel     string
		bonus          float64
		bonusPerHour   float64
		seeding        int
		seedingSizeTiB float64
		uploads        int
	}{
		{
			id:             "lemonhd",
			uid:            "2201",
			nameHTML:       "lemon-user",
			wantName:       "lemon-user",
			levelCell:      `<img title="Elite User">`,
			wantLevel:      "Elite User",
			joinDate:       "2023-01-02",
			bonusLabel:     "魔力值",
			bonus:          12345.5,
			bonusPerHour:   31.25,
			seeding:        21,
			seedingSizeTiB: 6.5,
			uploads:        4,
		},
		{
			id:             "ptvicomo",
			uid:            "2202",
			nameHTML:       "vico-user",
			wantName:       "vico-user",
			levelCell:      `<b>Island Master</b>`,
			wantLevel:      "Island Master",
			joinDate:       "2022-03-04",
			bonusLabel:     "象草",
			bonus:          22222.25,
			bonusPerHour:   42.5,
			seeding:        22,
			seedingSizeTiB: 7.25,
			uploads:        5,
		},
		{
			id:             "redleaves",
			uid:            "2203",
			nameHTML:       "<b>red-user</b><span class=\"badge\">Power</span>",
			wantName:       "red-user",
			levelCell:      `<img title="Power User">`,
			wantLevel:      "Power User",
			joinDate:       "2021-05-06",
			bonusLabel:     "魔力值",
			bonus:          33333.75,
			bonusPerHour:   53.75,
			seeding:        23,
			seedingSizeTiB: 8.5,
			uploads:        6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index.php":
					_, _ = w.Write([]byte(deadNexusIndexHTML(tc.uid, tc.nameHTML, tc.bonusLabel, tc.bonus, tc.seeding)))
				case "/userdetails.php":
					if r.URL.Query().Get("id") != tc.uid {
						t.Fatalf("detail id = %q, want %s", r.URL.Query().Get("id"), tc.uid)
					}
					_, _ = w.Write([]byte(deadNexusDetailHTML(tc.levelCell, tc.joinDate, tc.bonusLabel, tc.bonus, tc.seedingSizeTiB)))
				case "/mybonus.php":
					_, _ = w.Write([]byte(fmt.Sprintf(`<div>你当前每小时能获取 %.2f 个%s</div>`, tc.bonusPerHour, tc.bonusLabel)))
				case "/getusertorrentlistajax.php":
					if r.URL.Query().Get("userid") != tc.uid {
						t.Fatalf("ajax userid = %q, want %s", r.URL.Query().Get("userid"), tc.uid)
					}
					switch r.URL.Query().Get("type") {
					case "seeding":
						_, _ = w.Write([]byte(fmt.Sprintf(`<div>%d | %.2f TiB</div>`, tc.seeding, tc.seedingSizeTiB)))
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
			assertCoreStats(t, u, tc.uid, tc.wantName, tc.wantLevel, tc.joinDate, 2*(1<<40), 512*(1<<30), 4, tc.bonus)
			if u.BonusPerHour != tc.bonusPerHour || u.Seeding != tc.seeding || u.Uploads != tc.uploads {
				t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
			}
			if want := int64(tc.seedingSizeTiB * (1 << 40)); u.SeedingSize != want {
				t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
			}
		})
	}
}

func TestNexusPHPHaresUserInfoFromMoviePilotReference(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(`
<a href="user.php?id=2401"><span>hares-link</span></a>
<div>奶糖余额</div><div>98,765.5</div>`))
		case "/user.php":
			if r.URL.Query().Get("id") != "2401" {
				t.Fatalf("detail id = %q, want 2401", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`
<div class="layui-hares-user-info-right">
  <span>用户名：<a href="user.php?id=2401"><span>hares-user</span></a></span>
  <span>等级：<b>白兔长老</b></span>
  <span>注册日期：2024-02-03 10:11:12&#13;其它</span>
  <span>上传量：3.00 TiB</span>
  <span>下载量：512 GiB</span>
  <span>分享率：<em>6.0</em></span>
</div>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := fetchTestUserInfo(t, "hares", srv.URL)
	assertCoreStats(t, u, "2401", "hares-user", "白兔长老", "2024-02-03", 3*(1<<40), 512*(1<<30), 6, 98765.5)
}

func deadNexusIndexHTML(uid, nameHTML, bonusLabel string, bonus float64, seeding int) string {
	return fmt.Sprintf(`
<div id="info_block">
  <a href="userdetails.php?id=%s" class="PowerUser_Name">%s</a>
  <span><font class="color_uploaded">上传量</font>: 2.00 TiB</span>
  <span><font class="color_downloaded">下载量</font>: 512 GiB</span>
  <span><font class="color_ratio">分享率</font>: 4.000</span>
  <span><font class="color_bonus">%s</font> [使用]: %.2f</span>
  <span><font class="color_active">当前活动</font>: %d 0</span>
  <span class="lemon-seeding"><img class="arrowup" src="up.png"> %d</span>
</div>`, uid, nameHTML, bonusLabel, bonus, seeding, seeding)
}

func deadNexusDetailHTML(levelCell, joinDate, bonusLabel string, bonus, seedingSizeTiB float64) string {
	return fmt.Sprintf(`
<table>
  <tr><td class="rowhead">等级</td><td>%s</td></tr>
  <tr><td class="rowhead">加入日期</td><td>%s 09:10:11</td></tr>
  <tr><td class="rowhead">传输</td><td>上传量 2.00 TiB 下载量 512 GiB 分享率 4.000</td></tr>
  <tr><td class="rowhead">%s</td><td>%.2f</td></tr>
  <tr><td class="rowhead">当前做种</td><td>共 21 个，%.2f TiB</td></tr>
</table>`, levelCell, joinDate, bonusLabel, bonus, seedingSizeTiB)
}

func TestNexusPHPDeadDefinitionsCategoriesFromReferences(t *testing.T) {
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	tests := []struct {
		id         string
		catCount   int
		wantMovie  []string
		wantTV     []string
		wantMusic  []string
		wantCatIDs []string
	}{
		{id: "cdfile", catCount: 9, wantMovie: []string{"401", "404", "405"}, wantTV: []string{"402", "403"}, wantMusic: []string{"406", "408"}, wantCatIDs: []string{"401", "408", "409"}},
		{id: "icc2022", catCount: 9, wantMovie: []string{"401", "404", "405"}, wantTV: []string{"402", "403"}, wantMusic: []string{"406", "408"}, wantCatIDs: []string{"401", "408", "409"}},
		{id: "ptchina", catCount: 3, wantMovie: []string{"401", "404"}, wantTV: []string{"402"}, wantCatIDs: []string{"401", "402", "404"}},
		{id: "xauat6", catCount: 13, wantMovie: []string{"401", "403", "408"}, wantTV: []string{"402", "405"}, wantMusic: []string{"406", "407"}, wantCatIDs: []string{"401", "410", "424"}},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			def, ok := reg.Definition(tc.id)
			if !ok {
				t.Fatalf("%s definition missing", tc.id)
			}
			if len(def.Categories) != tc.catCount {
				t.Fatalf("categories = %d, want %d", len(def.Categories), tc.catCount)
			}
			for _, id := range tc.wantCatIDs {
				assertCategoryID(t, def, id)
			}
			assertStringSlice(t, def.SearchCategories["movie"], tc.wantMovie)
			assertStringSlice(t, def.SearchCategories["tv"], tc.wantTV)
			assertStringSlice(t, def.SearchCategories["music"], tc.wantMusic)
		})
	}
}

func assertCategoryID(t *testing.T, def *site.SiteMetadata, id string) {
	t.Helper()
	for _, c := range def.Categories {
		if c.ID == id {
			return
		}
	}
	t.Fatalf("%s categories missing id %s: %#v", def.ID, id, def.Categories)
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}
