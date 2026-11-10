// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

// ensureGazelleDefaults 填 Gazelle 专属默认（镜像 ensureUnit3DDefaults，仅填空段，幂等）。
// Gazelle 搜索入口 /torrents.php（同通用默认），关键字参数 searchstr，固定 searchsubmit=1。
// group→torrent 扁平化逻辑无法用 Rows 选择器表达，故由 ParseGazelleSearch 处理；
// size/seeders/leechers/completed 不配静态选择器，由 detectColumns 按表头动态解析。
func ensureGazelleDefaults(cp *SiteMetadata) {
	if cp.SearchParam == "" || cp.SearchParam == "search" {
		cp.SearchParam = "searchstr"
	}
	if cp.ExtraParam == nil {
		cp.ExtraParam = map[string]string{}
	}
	if _, ok := cp.ExtraParam["searchsubmit"]; !ok {
		cp.ExtraParam["searchsubmit"] = "1"
	}
	if cp.Selectors.Rows.Selector == "" && len(cp.Selectors.Rows.Selectors) == 0 {
		cp.Selectors = DefaultGazelleSelectors()
	} else if len(cp.Selectors.Fields) == 0 {
		cp.Selectors.Fields = DefaultGazelleSelectors().Fields
	}
	dfl := DefaultGazelleUserInfo()
	cp.UserInfo.Fields = mergeDefaultFields(cp.UserInfo.Fields, dfl.Fields)
	cp.UserInfo.DetailFields = mergeDefaultFields(cp.UserInfo.DetailFields, dfl.DetailFields)
	if len(cp.NoLogin.URLPattern) == 0 && len(cp.NoLogin.HttpStatus) == 0 &&
		cp.NoLogin.Selector == "" && len(cp.NoLogin.Content) == 0 {
		cp.NoLogin = DefaultGazelleNoLogin()
	}
}

// DefaultGazelleSelectors 返回 Gazelle 框架种子表选择器。
// Rows 选全部 tr（含表头/group 头/group_torrent 子/独立 torrent），ParseGazelleSearch 按行类分类。
func DefaultGazelleSelectors() SiteSelectors {
	return SiteSelectors{
		Rows: ElementQuery{Selector: "table.torrent_table tr"},
		Fields: map[string]ElementQuery{
			"download": {Selector: `a[href*="torrents.php?action=download"]`, Attr: "href"},
			"detail":   {Selector: `a[href*="torrents.php?id="]`, Attr: "href"},
			"title":    {Selector: `a[href*="torrents.php?id="]`},
			"subtitle": {Selector: ".tags"},
			"time":     {Selector: `span.time[title], time[title]`, Attr: "title"},
			"category": {Selector: `a[href*="filter_cat"] img`, Attr: "alt"},
			"free":     {Selector: `strong:contains("Freeleech!")`},
		},
	}
}

// DefaultGazelleUserInfo 返回 Gazelle 用户资料选择器（genStatBoxSelector 模式：分区头 + ul.stats li）。
// Path/DetailPath 留空：Gazelle 走适配器硬编码两步（/ 发现 uid → /user.php?id=N）。
func DefaultGazelleUserInfo() UserInfoDef {
	return UserInfoDef{
		Fields: map[string]ElementQuery{
			"name":       {Selector: `a.username[href*="user.php"]`},
			"uploaded":   {Selectors: []string{`div:contains("Stats") + ul.stats > li:contains("Uploaded")`, `div:contains("Statistics") + ul.stats > li:contains("Uploaded")`}, Filters: []Filter{{Name: "parse_size"}}},
			"downloaded": {Selectors: []string{`div:contains("Stats") + ul.stats > li:contains("Downloaded")`, `div:contains("Statistics") + ul.stats > li:contains("Downloaded")`}, Filters: []Filter{{Name: "parse_size"}}},
			"ratio":      {Selectors: []string{`div:contains("Stats") + ul.stats > li:contains("Ratio:")`, `div:contains("Statistics") + ul.stats > li:contains("Ratio:")`}},
			"bonus":      {Selectors: []string{`div:contains("Stats") + ul.stats > li:contains("Bonus Points:")`, `div:contains("Statistics") + ul.stats > li:contains("Bonus Points:")`}},
			"level":      {Selectors: []string{`div:contains("Personal") + ul.stats > li:contains("Class:")`}, Filters: []Filter{{Name: "regexp", Args: `Class:\s*(.+):::1`}}},
			"uploads":    {Selectors: []string{`div:contains("Community") + ul.stats > li:contains("Uploaded")`, `div:contains("Community") + ul.stats > li:contains("Uploaded:")`}, Filters: []Filter{{Name: "parse_number"}}},
			"seeding":    {Selectors: []string{`div:contains("Community") + ul.stats > li:contains("Seeding:")`}, Filters: []Filter{{Name: "regexp", Args: `(?i)Seeding[^0-9]*(\d+)`}, {Name: "parse_number"}}},
			"seeding_size": {Selectors: []string{
				`div:contains("Community") + ul.stats > li:contains("Total seed size")`,
				`div:contains("Community") + ul.stats > li:contains("Seeding size")`,
				`div:contains("Community") + ul.stats > li:contains("Seeding Size")`,
			}, Filters: []Filter{{Name: "regexp", Args: `(?i)(?:Total seed size|Seeding size)[^0-9]*([\d.,]+\s*[KMGTPE]i?B)`}, {Name: "parse_size"}}},
		},
		DetailFields: map[string]ElementQuery{
			"join_date": {Selectors: []string{`div:contains("Stats") + ul.stats > li:contains("Joined:") > span`}, Attr: "title", Filters: []Filter{{Name: "parse_time"}}},
		},
	}
}

// DefaultGazelleNoLogin 返回 Gazelle 未登录判定。cookie 失效重定向 login.php。
func DefaultGazelleNoLogin() NoLoginDef {
	return NoLoginDef{
		HttpStatus: []int{401, 403},
		URLPattern: []string{"login.php", "login"},
		Selector:   `form[action*="login.php"]`,
	}
}
