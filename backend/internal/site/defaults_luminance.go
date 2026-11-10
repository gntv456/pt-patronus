// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

// ensureLuminanceDefaults 填 Luminance 专属默认（同 ensureGazelleDefaults 形）。
// 搜索 /torrents.php?action=advanced&title=<kw>；单层 table#torrent_table（无 group 扁平化）。
func ensureLuminanceDefaults(cp *SiteMetadata) {
	if cp.SearchParam == "" || cp.SearchParam == "search" {
		cp.SearchParam = "title"
	}
	if cp.ExtraParam == nil {
		cp.ExtraParam = map[string]string{}
	}
	if _, ok := cp.ExtraParam["action"]; !ok {
		cp.ExtraParam["action"] = "advanced"
	}
	if cp.Selectors.Rows.Selector == "" && len(cp.Selectors.Rows.Selectors) == 0 {
		cp.Selectors = DefaultLuminanceSelectors()
	} else if len(cp.Selectors.Fields) == 0 {
		cp.Selectors.Fields = DefaultLuminanceSelectors().Fields
	}
	dfl := DefaultLuminanceUserInfo()
	cp.UserInfo.Fields = mergeDefaultFields(cp.UserInfo.Fields, dfl.Fields)
	cp.UserInfo.DetailFields = mergeDefaultFields(cp.UserInfo.DetailFields, dfl.DetailFields)
	if len(cp.NoLogin.URLPattern) == 0 && len(cp.NoLogin.HttpStatus) == 0 &&
		cp.NoLogin.Selector == "" && len(cp.NoLogin.Content) == 0 {
		cp.NoLogin = DefaultLuminanceNoLogin()
	}
}

// DefaultLuminanceSelectors 返回 Luminance 框架种子表选择器（单层，无 group）。
// size/seeders/leechers/completed 由 detectColumns 按表头动态解析。
func DefaultLuminanceSelectors() SiteSelectors {
	return SiteSelectors{
		Rows: ElementQuery{Selector: "table#torrent_table tr"},
		Fields: map[string]ElementQuery{
			"title":    {Selector: `a[href*="torrents.php?id="]`},
			"detail":   {Selector: `a[href*="torrents.php?id="]`, Attr: "href"},
			"download": {Selector: `a[href*="torrents.php?action=download"]`, Attr: "href"},
			"time":     {Selector: `span.time[title]`, Attr: "title"},
			"category": {Selector: `td.cats_col > div[title] > a`, Attr: "href"},
			"subtitle": {Selector: "div.tags"},
		},
	}
}

// DefaultLuminanceUserInfo 返回 Luminance 用户资料选择器（ul.stats > li:contains）。
// bonus 在 Luminance 叫 "Credits"（#bonusdiv h4）。Path/DetailPath 留空（适配器硬编码两步）。
func DefaultLuminanceUserInfo() UserInfoDef {
	return UserInfoDef{
		Fields: map[string]ElementQuery{
			"name":       {Selector: `a.username`},
			"uploaded":   {Selectors: []string{`ul.stats > li:contains("Uploaded:")`}, Filters: []Filter{{Name: "parse_size"}}},
			"downloaded": {Selectors: []string{`ul.stats > li:contains("Downloaded:")`}, Filters: []Filter{{Name: "parse_size"}}},
			"ratio":      {Selector: `ul.stats > li:contains("Ratio:") > span`},
			"level":      {Selectors: []string{`span.rank`, `ul.stats > li:contains("Class:")`}},
			"bonus":      {Selectors: []string{`div#bonusdiv > h4`, `h4:contains("Credits:")`}},
			"seeding":    {Selector: `ul.stats > li:contains("Seeding:")`},
			"leeching":   {Selector: `ul.stats > li:contains("Leeching:")`},
			"seeding_size": {
				Selector: `ul.stats > li:contains("Seeding Size:")`,
				Filters:  []Filter{{Name: "parse_size"}},
			},
			"uploads": {
				Selector: `ul.stats > li[title]:contains("Uploaded:")`,
				Filters:  []Filter{{Name: "split", Args: "[:::0"}, {Name: "parse_number"}},
			},
			"posts": {Selector: `ul.stats > li:contains("Forum Posts:")`, Filters: []Filter{{Name: "parse_number"}}},
			"bonus_per_hour": {
				Selector: `div#bonuslog`,
				Filters: []Filter{
					{Name: "regexp", Args: `(?i)\|\s*[+-]?([\d.,]+)\s*credits\s*\|`},
					{Name: "divide", Args: "24"},
				},
			},
		},
		DetailFields: map[string]ElementQuery{
			"join_date":      {Selector: `ul.stats > li:contains("Joined:") > span.time`, Attr: "title", Filters: []Filter{{Name: "parse_time"}}},
			"last_access_at": {Selector: `ul.stats > li:contains("Last Seen:") > span`, Attr: "title", Filters: []Filter{{Name: "parse_time"}}},
		},
	}
}

// DefaultLuminanceNoLogin 返回 Luminance 未登录判定。
func DefaultLuminanceNoLogin() NoLoginDef {
	return NoLoginDef{
		HttpStatus: []int{401, 403},
		URLPattern: []string{"login.php", "login"},
	}
}
