package site

func ensureTNodeDefaults(cp *SiteMetadata) {
	if !cp.Medal.Enabled && cp.Medal.ShopPath == "" {
		cp.Medal.Enabled = true
	}
	if cp.Medal.ShopPath == "" {
		cp.Medal.ShopPath = "medals.php"
	}
	if cp.Medal.OwnPath == "" {
		cp.Medal.OwnPath = "medals.php?action=owned"
	}
}

// ensureMinimalDefaults 填所有 schema 共享的通用路径默认。
func ensureMinimalDefaults(cp *SiteMetadata) {
	if cp.SearchEntry == "" {
		cp.SearchEntry = "torrents.php"
	}
	if cp.SearchParam == "" {
		cp.SearchParam = "search"
	}
	// H&R 列表页路径默认值（Enabled 默认 false，需站点显式开启）
	if cp.HR.Path == "" {
		cp.HR.Path = "getusertorrentlist.php?type=hr"
	}
}

// ensureMTeamDefaults：馒头走 JSON API（非 HTML 抓取），其适配器已硬编码实现
// FetchFreeleech（/api/system/promotion/rules）与 FetchAnnouncement（/api/news/list）。
// 这里仅需把两项 capability 默认放行（Enabled=true），否则 chain 层的 !Enabled 守卫会
// 直接返回空，前端误显「该站点未开启站免检测」（mteam.yaml 未配 freeleech/announcement 段）。
// Path 无需——适配器内部硬编码端点，不读 def.Freeleech.Path/Announcement.Path。
// 复用 applyInfoDefaults：与 NexusPHP 一致，站点 YAML 显式 enabled:false 可关闭。
func ensureMTeamDefaults(cp *SiteMetadata) {
	cp.Freeleech.Enabled, cp.Freeleech.Path = applyInfoDefaults(cp.Freeleech.Enabled, cp.Freeleech.Path, FreeleechDef{Enabled: true})
	cp.Announcement.Enabled, cp.Announcement.Path = applyInfoDefaults(cp.Announcement.Enabled, cp.Announcement.Path, FreeleechDef{Enabled: true})
}

func ensureRartrackerDefaults(cp *SiteMetadata) {
	if cp.SearchEntry == "" || cp.SearchEntry == "torrents.php" {
		cp.SearchEntry = "/api/v1/torrents"
	}
	if cp.SearchParam == "" || cp.SearchParam == "search" {
		cp.SearchParam = "searchText"
	}
	if cp.Format == "" {
		cp.Format = "json"
	}
	if cp.ExtraParam == nil {
		cp.ExtraParam = map[string]string{}
	}
	for k, v := range map[string]string{
		"limit":          "100",
		"index":          "0",
		"section":        "all",
		"extendedSearch": "false",
		"watchview":      "false",
	} {
		if _, ok := cp.ExtraParam[k]; !ok {
			cp.ExtraParam[k] = v
		}
	}
	dfl := DefaultRartrackerSelectors()
	if cp.Selectors.Rows.Selector == "" && len(cp.Selectors.Rows.Selectors) == 0 {
		cp.Selectors.Rows = dfl.Rows
	}
	cp.Selectors.Fields = mergeDefaultFields(cp.Selectors.Fields, dfl.Fields)
}

func DefaultRartrackerSelectors() SiteSelectors {
	return SiteSelectors{
		Rows: ElementQuery{Selector: ":self"},
		Fields: map[string]ElementQuery{
			"title":     {Selector: "name"},
			"detail":    {Selector: "id", Filters: []Filter{{Name: "prepend", Args: "/torrent/"}}},
			"size":      {Selector: "size"},
			"time":      {Selector: "added"},
			"seeders":   {Selector: "seeders"},
			"leechers":  {Selector: "leechers"},
			"completed": {Selector: "timesCompleted"},
			"category":  {Selector: "category"},
		},
	}
}
