// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

func ensureTCGDefaults(cp *SiteMetadata) {
	if cp.SearchEntry == "" || cp.SearchEntry == "torrents.php" {
		cp.SearchEntry = "browse.php"
	}
	if cp.SearchParam == "" {
		cp.SearchParam = "search"
	}
	if cp.ExtraParam == nil {
		cp.ExtraParam = map[string]string{}
	}
	for k, v := range map[string]string{"incldead": "1", "titleonly": "1", "nonboolean": "0"} {
		if _, ok := cp.ExtraParam[k]; !ok {
			cp.ExtraParam[k] = v
		}
	}
	dflSel := DefaultTCGSelectors()
	if cp.Selectors.Rows.Selector == "" && len(cp.Selectors.Rows.Selectors) == 0 {
		cp.Selectors.Rows = dflSel.Rows
	}
	cp.Selectors.Fields = mergeDefaultFields(cp.Selectors.Fields, dflSel.Fields)
	dflUI := DefaultTCGUserInfo()
	cp.UserInfo.Pages = mergeDefaultUserInfoPages(cp.UserInfo.Pages, dflUI.Pages)
	if len(cp.NoLogin.URLPattern) == 0 && len(cp.NoLogin.HttpStatus) == 0 &&
		cp.NoLogin.Selector == "" && len(cp.NoLogin.Content) == 0 {
		cp.NoLogin = DefaultTCGNoLogin()
	}
}

func DefaultTCGSelectors() SiteSelectors {
	return SiteSelectors{
		Rows: ElementQuery{Selector: "table[border='0'] > tbody > tr.ttable:has(a[href^='browse.php?cat=']):not(:has(font[color='blue'])):not(:contains('*TCG*'))"},
		Fields: map[string]ElementQuery{
			"title":     {Selector: `a[href^="details.php?id="]`, Attr: "title"},
			"detail":    {Selector: `a[href^="details.php?id="]`, Attr: "href"},
			"download":  {Selector: `a[href^="download.php/"]`, Attr: "href"},
			"time":      {Selector: "td:nth-child(6)", Filters: []Filter{{Name: "parse_time"}}},
			"size":      {Selector: "td:nth-child(7)", Filters: []Filter{{Name: "parse_size"}}},
			"completed": {Selector: "td:nth-child(8)", Filters: []Filter{{Name: "replace", Args: "Never:::0"}, {Name: "parse_number"}}},
			"seeders":   {Selector: "td:nth-child(9)", Filters: []Filter{{Name: "parse_number"}}},
			"leechers":  {Selector: "td:nth-child(10)", Filters: []Filter{{Name: "parse_number"}}},
			"comments":  {Selector: "td:nth-child(5)", Filters: []Filter{{Name: "parse_number"}}},
			"subtitle":  {Selector: "span > em"},
		},
	}
}

func DefaultTCGUserInfo() UserInfoDef {
	return UserInfoDef{
		Pages: []UserInfoPageDef{
			{
				Path: "main.php",
				Fields: map[string]ElementQuery{
					"id":   {Selector: `a[href*="userdetails.php?id="]:first`, Attr: "href", Filters: []Filter{{Name: "queryparam", Args: "id"}}},
					"name": {Selector: `a[href*="userdetails.php?id="]:first`},
				},
			},
			{
				Path:   "userdetails.php",
				Params: map[string]string{"id": "{id}"},
				Fields: map[string]ElementQuery{
					"join_date":      {Selector: `.embedded td:contains("Join date") + td:first`, Filters: []Filter{{Name: "split", Args: "(:::0"}, {Name: "trim"}, {Name: "parse_time"}}},
					"last_access_at": {Selector: `.embedded td:contains("Last seen") + td:first`, Filters: []Filter{{Name: "split", Args: "(:::0"}, {Name: "trim"}, {Name: "parse_time"}}},
					"uploaded":       {Selector: `td.rowhead:contains("Uploaded") + td`, Filters: []Filter{{Name: "split", Args: "(:::0"}, {Name: "trim"}, {Name: "parse_size"}}},
					"downloaded":     {Selector: `td.rowhead:contains("Downloaded") + td`, Filters: []Filter{{Name: "split", Args: "(:::0"}, {Name: "trim"}, {Name: "parse_size"}}},
					"ratio":          {Selectors: []string{`td.statuslink span:contains("Ratio:") + span`, `td.rowhead:contains("Ratio") + td`}, Filters: []Filter{{Name: "replace", Args: "---:::-1"}, {Name: "parse_number"}}},
					"seeding":        {Selector: `img[title="seeders"] + span:first`, Filters: []Filter{{Name: "parse_number"}}},
					"level":          {Selector: `td.rowhead:contains("Class") + td`},
					"seeding_size": {
						Selector: `div#kd1 > table.details > tbody tr td:not(.colbrowsehead):nth-child(4)`,
						Attr:     "text_all",
						Filters:  []Filter{{Name: "sum_size"}},
					},
				},
			},
		},
	}
}

func DefaultTCGNoLogin() NoLoginDef {
	return NoLoginDef{
		HttpStatus: []int{401, 403},
		URLPattern: []string{"login.php", "takelogin.php"},
		Selector:   `form[action*="takelogin"], form[action*="login.php"], input[name="password"]`,
	}
}
