package site

// ensureUnit3DDefaults 填 UNIT3D 专属默认（镜像 ensureNexusPHPDefaults，仅填空段，幂等）。
// UNIT3D（Laravel 框架）页面结构跨站稳定：种子表 .torrent-search--list__*、用户资料 ratio-bar +
// i18n dt/dd 块。站点 YAML 可覆盖任一段（如 deep-space 主题站只覆盖 user_info 选择器）。
// 与 NexusPHP 不同：UserInfo.Path/DetailPath 刻意留空——UNIT3D 用户资料走适配器硬编码的
// 三步流程（/ → /users/{name} → /users/{name}/earnings），不经通用 Path 机制。
// 不启用 NexusPHP capability（Freeleech/Harem/Messages/Detail）——属后续里程碑。
func ensureUnit3DDefaults(cp *SiteMetadata) {
	// 覆盖通用默认：UNIT3D 搜索入口是 /torrents，关键字参数是 name（非 torrents.php/search）。
	if cp.SearchEntry == "" || cp.SearchEntry == "torrents.php" {
		cp.SearchEntry = "torrents"
	}
	if cp.SearchParam == "" || cp.SearchParam == "search" {
		cp.SearchParam = "name"
	}
	// 选择器：rows 空（单数 Selector 与复数 Selectors 都空）才整体取默认；
	// 仅 rows 有值（per-site 覆盖）时仍补 fields 默认。
	if cp.Selectors.Rows.Selector == "" && len(cp.Selectors.Rows.Selectors) == 0 {
		cp.Selectors = DefaultUnit3DSelectors()
	} else if len(cp.Selectors.Fields) == 0 {
		cp.Selectors.Fields = DefaultUnit3DSelectors().Fields
	}
	// 用户资料：Fields/DetailFields 空才填默认；Path/DetailPath 留空（适配器硬编码三步）。
	dflUI := DefaultUnit3DUserInfo()
	cp.UserInfo.Fields = mergeDefaultFields(cp.UserInfo.Fields, dflUI.Fields)
	cp.UserInfo.DetailFields = mergeDefaultFields(cp.UserInfo.DetailFields, dflUI.DetailFields)
	// 未登录判定：任一字段已配则尊重站点，否则填默认。
	if len(cp.NoLogin.URLPattern) == 0 && len(cp.NoLogin.HttpStatus) == 0 &&
		cp.NoLogin.Selector == "" && len(cp.NoLogin.Content) == 0 {
		cp.NoLogin = DefaultUnit3DNoLogin()
	}
}

// DefaultUnit3DSelectors 返回 UNIT3D 框架标准种子表选择器（cascadia v1.3.3 兼容）。
// 注意：rows 用 Selectors 回退列表（ParseSearch 只读单数 Selector，故 unit3d 走 ParseUnit3DSearch，
// 它处理回退列表 + 丢弃非 /torrents/{id} 的表头/广告行）。download 链接 UNIT3D 用 /download_check/
// 需重写为 /download/（replace 过滤）。
func DefaultUnit3DSelectors() SiteSelectors {
	return SiteSelectors{
		Rows: ElementQuery{Selectors: []string{
			"div.torrent-search--list__results > table > tbody > tr", // 新版布局
			"div.table-responsive table > tbody > tr",                // 旧版布局
		}},
		Fields: map[string]ElementQuery{
			"title":  {Selectors: []string{"a.view-torrent", "a.torrent-search--list__name"}},
			"detail": {Selectors: []string{"a.view-torrent", "a.torrent-search--list__name"}, Attr: "href"},
			"download": {
				Selectors: []string{`a[href*="/download/"]`, `a[href*="/download_check/"]`},
				Attr:      "href",
				Filters:   []Filter{{Name: "replace", Args: "/download_check/:::/download/"}},
			},
			"size":      {Selectors: []string{"td.torrent-search--list__size", `td > span.text-blue:contains("B")`}},
			"seeders":   {Selectors: []string{"td.torrent-search--list__seeders", `a[href*="/peers"] > span.text-green`}},
			"leechers":  {Selectors: []string{"td.torrent-search--list__leechers", `a[href*="/peers"] > span.text-red`}},
			"completed": {Selectors: []string{"td.torrent-search--list__completed", `a[href*="/history"] > span.text-orange`}},
			"time":      {Selectors: []string{"time"}, Attr: "title"},
			"category": {
				Selectors: []string{
					`a[href*="/categories/"] img[data-original-title]`,
					`a[href*="/categories/"] i[data-original-title]`,
					"div.torrent-search--list__category img",
				},
				Attr: "data-original-title",
			},
		},
	}
}

// DefaultUnit3DUserInfo 返回 UNIT3D 用户资料选择器（ratio-bar + i18n dt/dd 双轨）。
// cascadia 不支持 :has(> x) 前导组合子，故 PT-depiler 的 :has(> i.fa-x) 改为 :has(i.fa-x)
// （ratio-bar 内 <i> 是 <a> 唯一子代，后代匹配等价）。dt:contains('Label') + dd 覆盖中英文。
// name 选择器从顶部导航 settings 链接的 href 抽用户名（三步流程第 1 步用 ParseUnit3DUsername）。
func DefaultUnit3DUserInfo() UserInfoDef {
	return UserInfoDef{
		// Path/DetailPath 刻意留空：UNIT3D 走适配器硬编码三步（/, /users/{name}, /users/{name}/earnings）。
		Fields: map[string]ElementQuery{
			"name": {
				Selectors: []string{`a[href*="/users/"][href*="settings"]`},
				Attr:      "href",
				Filters:   []Filter{{Name: "regexp", Args: `users/([^/?#]+)`}},
			},
			"uploaded":   {Selectors: []string{"li.ratio-bar__uploaded a:has(i.fa-arrow-up)", "span:has(i.fa-arrow-up)"}, Filters: []Filter{{Name: "parse_size"}}},
			"downloaded": {Selectors: []string{"li.ratio-bar__downloaded a:has(i.fa-arrow-down)", "span:has(i.fa-arrow-down)"}, Filters: []Filter{{Name: "parse_size"}}},
			"ratio": {Selectors: []string{
				`dt:contains("Ratio") + dd`, `dt:contains("分享率") + dd`, `dt:contains("比率") + dd`,
				"li.ratio-bar__ratio a:has(i.fa-sync-alt)", "span:has(i.fa-sync-alt)",
			}},
			"bonus":    {Selectors: []string{"li.ratio-bar__points a:has(i.fa-coins)", "span:has(i.fa-coins)"}},
			"seeding":  {Selectors: []string{"li.ratio-bar__seeding a:has(i.fa-upload)", "span:has(i.fa-upload)"}},
			"leeching": {Selectors: []string{"li.ratio-bar__leeching a:has(i.fa-download)", "span:has(i.fa-download)"}},
			"seeding_size": {Selectors: []string{
				`dt:contains("Seeding Size") + dd`, `dt:contains("Seeding size") + dd`,
				`dt:contains("做种体积") + dd`, `dt:contains("做種體積") + dd`,
				`td:contains("Seeding Size") + td`, `td:contains("Seeding size") + td`,
				`td:contains("做种体积") + td`, `td:contains("做種體積") + td`,
			}, Filters: []Filter{{Name: "parse_size"}}},
			"level": {Selectors: []string{"div.content span.badge-user", "a.user-tag__link"}, Attr: "title"},
			"join_date": {
				Selectors: []string{"time.profile__registration"},
				Attr:      "datetime",
				Filters:   []Filter{{Name: "parse_time"}},
			},
		},
		DetailFields: map[string]ElementQuery{
			// /users/{name}/earnings 页 .panelV2 dl.key-value 的第 3 个 dd（PT-depiler dd:nth(2) 0-基）。
			// cascidia 不支持裸 :nth(N)，用 :nth-of-type(3)。
			"bonus_per_hour": {Selectors: []string{".panelV2 dl.key-value dd:nth-of-type(3)", "dl.key-value dd:nth-of-type(3)"}},
		},
	}
}

// DefaultUnit3DNoLogin 返回 UNIT3D 未登录判定。UNIT3D cookie 夺效时重定向到 /login 或返 401/403。
func DefaultUnit3DNoLogin() NoLoginDef {
	return NoLoginDef{
		HttpStatus: []int{401, 403},
		URLPattern: []string{"/login", "verify", "checkpoint"},
	}
}
