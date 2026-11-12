// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

// ensureSmallHorseDefaults fills smallhorse parser defaults
// shape: index user.php?id discovery, user.php stats blocks, and
// torrents.php?type=seeding&userid=N table#torrent_table seeding totals.
func ensureSmallHorseDefaults(cp *SiteMetadata) {
	if cp.Selectors.Rows.Selector == "" && len(cp.Selectors.Rows.Selectors) == 0 {
		cp.Selectors = DefaultSmallHorseSelectors()
	} else if len(cp.Selectors.Fields) == 0 {
		cp.Selectors.Fields = DefaultSmallHorseSelectors().Fields
	}
	dfl := DefaultSmallHorseUserInfo()
	cp.UserInfo.Pages = mergeDefaultUserInfoPages(cp.UserInfo.Pages, dfl.Pages)
	if len(cp.NoLogin.URLPattern) == 0 && len(cp.NoLogin.HttpStatus) == 0 &&
		cp.NoLogin.Selector == "" && len(cp.NoLogin.Content) == 0 {
		cp.NoLogin = DefaultSmallHorseNoLogin()
	}
}

// DefaultSmallHorseSelectors returns best-effort search selectors for the
// single table used by SmallHorse-style sites. User data is covered below by
// UserInfo.Pages because the framework requires uid discovery before stats.
func DefaultSmallHorseSelectors() SiteSelectors {
	return SiteSelectors{
		Rows: ElementQuery{Selector: "table#torrent_table tr"},
		Fields: map[string]ElementQuery{
			"title":    {Selectors: []string{`a[href*="torrents.php?id="]`, `a[href*="details.php?id="]`}},
			"detail":   {Selectors: []string{`a[href*="torrents.php?id="]`, `a[href*="details.php?id="]`}, Attr: "href"},
			"download": {Selectors: []string{`a[href*="torrents.php?action=download"]`, `a[href*="download.php"]`}, Attr: "href"},
			"size":     {Selector: "td:nth-child(6)"},
			"seeders":  {Selector: "td:nth-child(8)"},
			"leechers": {Selector: "td:nth-child(9)"},
		},
	}
}

// DefaultSmallHorseUserInfo mirrors the smallhorse framework
// small_horse.py with executable CSS selectors and filters.
func DefaultSmallHorseUserInfo() UserInfoDef {
	stats := "ul.stats.nobullet:eq(1)"
	return UserInfoDef{
		Pages: []UserInfoPageDef{
			{
				Path: "/",
				Fields: map[string]ElementQuery{
					"id":   {Selector: `a[href*="user.php?id="]:first`, Attr: "href", Filters: []Filter{{Name: "queryparam", Args: "id"}}},
					"name": {Selector: `a[href*="user.php?id="]:first`},
				},
			},
			{
				Path: "user.php?id={id}",
				Fields: map[string]ElementQuery{
					"name":      {Selector: `a[href*="user.php?id="]:first`},
					"join_date": {Selector: stats + " > li:eq(0) span", Filters: []Filter{{Name: "parse_time"}}},
					"uploaded": {
						Selector: stats + " > li:eq(2)",
						Filters:  []Filter{{Name: "regexp", Args: `(?i)(?:Uploaded|上传|上傳)?[^0-9]*([\d.,]+\s*[KMGTPE]i?B)`}, {Name: "parse_size"}},
					},
					"downloaded": {
						Selector: stats + " > li:eq(3)",
						Filters:  []Filter{{Name: "regexp", Args: `(?i)(?:Downloaded|下载|下載)?[^0-9]*([\d.,]+\s*[KMGTPE]i?B)`}, {Name: "parse_size"}},
					},
					"ratio": {
						Selectors: []string{stats + " > li:eq(4) span", stats + " > li:eq(4)", stats + " > li:eq(5)"},
						Filters:   []Filter{{Name: "regexp", Args: `(?i)(?:Ratio|分享率|比率)?[^0-9∞]*([∞\d.]+)`}, {Name: "replace", Args: "∞:::9999"}},
					},
					"bonus": {
						Selector: stats + " > li:eq(5)",
						Filters:  []Filter{{Name: "regexp", Args: `(?i)(?:Bonus|Credits|魔力|积分|積分)?[^0-9]*([\d.,]+(?:\.\d+)?)`}, {Name: "parse_number"}},
					},
					"level": {
						Selector: "ul.stats.nobullet:eq(3) > li:eq(0)",
						Filters:  []Filter{{Name: "regexp", Args: `(?i)(?:Class|Level|等级|等級)\s*:\s*(.+):::1`}, {Name: "trim"}},
					},
					"leeching": {
						Selector: "ul.stats.nobullet:eq(4) > li:eq(6)",
						Filters:  []Filter{{Name: "regexp", Args: `(?i)(?:Leeching|下载中|下載中)?[^0-9]*(\d+)`}, {Name: "parse_number"}},
					},
				},
			},
			{
				Path: "torrents.php?type=seeding&userid={id}",
				Fields: map[string]ElementQuery{
					"seeding":      {Selector: "table#torrent_table tr:not(:first-child) td:nth-child(6)", Attr: "count"},
					"seeding_size": {Selector: "table#torrent_table tr:not(:first-child) td:nth-child(6)", Attr: "text_all", Filters: []Filter{{Name: "sum_size"}}},
				},
			},
		},
	}
}

func DefaultSmallHorseNoLogin() NoLoginDef {
	return NoLoginDef{
		HttpStatus: []int{401, 403},
		URLPattern: []string{"login.php", "login"},
		Selector:   `form[action*="login"], input[name="password"]`,
	}
}
