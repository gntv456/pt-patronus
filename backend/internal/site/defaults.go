package site

// DefaultNexusPHPSelectors 返回经好学源码校准的 NexusPHP 标准选择器。
// 站点定义（YAML）可省略 selectors，adapter 自动填入这些默认值，保持 YAML 精简。
// 集中维护一份，所有 NexusPHP 站受益（个别差异时在站点 YAML 里覆盖即可）。
func DefaultNexusPHPSelectors() SiteSelectors {
	return SiteSelectors{
		Rows: ElementQuery{Selector: "table.torrents tr"},
		Fields: map[string]ElementQuery{
			"title":     {Selector: `a[href^="details.php"]:not([href*="#"])`},
			"subtitle":  {Selector: ".torrent-subtitle"},
			"detail":    {Selector: `a[href^="details.php"]:not([href*="#"])`, Attr: "href"},
			"download":  {Selector: `a[href^="download.php"]`, Attr: "href"},
			"completed": {Selector: `a[href^="viewsnatches.php"]`},
			"seeders":   {Selector: `a[href*="#seeders"]`},
			"leechers":  {Selector: `a[href*="#leechers"]`},
			"size":      {Selector: "td:nth-last-child(5)"},
			// 免费/促销图标（NexusPHP 源码标准 pro_free class；inspect 校准：attr:alt="Free"）。
			// search.go:113 Extract 非空 → Free=true，filter FREE 规则据此生效。
			// class^="pro_free" 覆盖 pro_free / pro_free2up（含免费的促销）。
			"free": {Selector: `img[class^="pro_free"]`, Attr: "alt"},
		},
	}
}

// DefaultNexusPHPUserInfo 返回标准用户资料选择器。
//
// NexusPHP 框架的 font.color_* 是常量（color_uploaded/downloaded/ratio/active/bonus），
// 三种主流皮肤（cute-top-profile / Audiences menu-base-info / 经典 info_block）都带这些类，
// 只是值的位置不同——故统一用「命中 color_* → 取父节点文本 → 正则抽值」锚点策略，
// 覆盖绝大多数国内 NexusPHP 站（实测校准自好学/财神/蟹黄堡）。
//
// 富字段 level/join_date/name/email 在 userdetails.php?id=<uid> 的 rowhead 表格里，需适配器先发现 uid。
// name/email 为 best-effort：部分皮肤资料页无对应行或邮箱部分隐藏（a***@b.com），抓不到留空。
func DefaultNexusPHPUserInfo() UserInfoDef {
	return UserInfoDef{
		Path: "usercp.php",
		Fields: map[string]ElementQuery{
			"uploaded": {
				Selector: "font.color_uploaded, span.color_uploaded",
				Parent:   true,
				Filters:  []Filter{{Name: "regexp", Args: `(?:上传量|上傳量|Uploaded)[：:]?\s*([\d.,]+\s*[KMGTPE]?i?B)`}, {Name: "parse_size"}},
			},
			"downloaded": {
				Selector: "font.color_downloaded, span.color_downloaded",
				Parent:   true,
				Filters:  []Filter{{Name: "regexp", Args: `(?:下载量|下載量|Downloaded)[：:]?\s*([\d.,]+\s*[KMGTPE]?i?B)`}, {Name: "parse_size"}},
			},
			"ratio": {
				Selector: "font.color_ratio, span.color_ratio",
				Parent:   true,
				Filters:  []Filter{{Name: "regexp", Args: `(?:分享率|比率|Ratio)[：:]?\s*([-+]?\d+\.?\d*)`}},
			},
			// 魔力/积分：NexusPHP 用户条固定形如「<font class=color_bonus>魔力值</font> [使用]: 940,870.1」，
			// 数值紧跟 mybonus 链接「[使用]」之后（标签名因站而异：魔力值/U币/爆米花/Karma，但 [使用] 锚点稳定）。
			// 站点货币名不同时仍命中（锚 [使用] 而非标签）；无该结构的站返空 → 0（无害，站点可 yaml 覆盖）。
			// 部分皮肤（如 OurBits）用 <span> 而非 <font>，故 font/span 双标签命中。
			"bonus": {
				Selector: "font.color_bonus, span.color_bonus",
				Parent:   true,
				Filters:  []Filter{{Name: "regexp", Args: `\[使用\][：:]?\s*([\d.,]+)`}},
			},
			"bonus_per_hour": {
				Selectors: []string{
					`div:contains("你当前每小时能获取")`,
					`div:contains("你當前每小時能獲取")`,
					`div:contains("You are currently getting")`,
				},
				Filters: []Filter{{Name: "regexp", Args: `(?i)(?:能获取|能獲取|currently\s+getting)[^0-9-+]{0,24}([-+]?[\d,]+\.?\d*)`}, {Name: "parse_number"}},
			},
			// 做种数：用户条「<font class=color_active>当前活动:</font> ↑N ↓M」——首个数字即做种数。
			// （做种体积 seeding_size 由适配器 ajax 列表另取；此处只取「数」。）
			"seeding": {
				Selector: "font.color_active, span.color_active",
				Parent:   true,
				Filters:  []Filter{{Name: "regexp", Args: `(?:当前活动|当前做种|Active)[：:]?\s*(\d+)`}},
			},
			"seeding_size": {
				Selectors: []string{
					`td[class*="rowhead"]:contains("当前做种") + td`,
					`td[class*="rowhead"]:contains("目前做種") + td`,
					`td[class*="rowhead"]:contains("Current Seeding") + td`,
					`td[class*="rowhead"]:contains("做种大小") + td`,
					`td[class*="rowhead"]:contains("做種大小") + td`,
					`td[class*="rowhead"]:contains("Seeding Size") + td`,
				},
				Filters: []Filter{{Name: "regexp", Args: `(?i)([\d.,]+\s*(?:[KMGTPE]i?B|Bytes))`}, {Name: "parse_size"}},
			},
			"claim_count": {
				Selectors: []string{
					`#info_block a[href*="claim.php"]:contains("/")`,
					`a[href*="claim.php"]:contains("/")`,
					`td[class*="rowhead"]:contains("认领") + td`,
					`td[class*="rowhead"]:contains("認領") + td`,
					`td[class*="rowhead"]:contains("Claim") + td`,
					`#info_block`,
				},
				Filters: []Filter{{Name: "claim_count"}},
			},
			"max_claim": {
				Selectors: []string{
					`#info_block a[href*="claim.php"]:contains("/")`,
					`a[href*="claim.php"]:contains("/")`,
					`td[class*="rowhead"]:contains("认领") + td`,
					`td[class*="rowhead"]:contains("認領") + td`,
					`td[class*="rowhead"]:contains("Claim") + td`,
					`#info_block`,
				},
				Filters: []Filter{{Name: "claim_max"}},
			},
		},
		DetailPath: "userdetails.php",
		DetailFields: map[string]ElementQuery{
			"name": {
				// 资料页 rowhead「用户名」行；繁体站用「用戶名」。部分皮肤仅在顶部 h1，抓不到留空。
				Selectors: []string{
					`td[class*="rowhead"]:contains("用户名") + td`,
					`td[class*="rowhead"]:contains("用戶名") + td`,
				},
			},
			"email": {
				// 资料页 rowhead 邮箱行（邮箱/信箱/E-mail/Email）；部分隐藏原样保留，无则留空。
				Selectors: []string{
					`td[class*="rowhead"]:contains("邮箱") + td`,
					`td[class*="rowhead"]:contains("信箱") + td`,
					`td[class*="rowhead"]:contains("E-mail") + td`,
					`td[class*="rowhead"]:contains("Email") + td`,
				},
			},
			"level": {
				Selector: `td[class*="rowhead"]:contains("等级") + td img`,
				Attr:     "title",
			},
			"level_id": {
				Selectors: []string{
					`td[class*="rowhead"]:contains("等级") + td img`,
					`td[class*="rowhead"]:contains("等級") + td img`,
					`td[class*="rowhead"]:contains("Class") + td img`,
				},
				Attr:    "src",
				Filters: []Filter{{Name: "regexp", Args: `/class/(\d+)\.gif`}, {Name: "add", Args: "-1"}},
			},
			"avatar": {
				Selectors: []string{
					`td[class*="rowhead"]:contains("头像") + td img`,
					`td[class*="rowhead"]:contains("頭像") + td img`,
					`td[class*="rowhead"]:contains("Avatar") + td img`,
					`img[name="avatarimg"]`,
					`img.avatar`,
					`img[src*="avatar"]`,
					`img[src*="avatars"]`,
				},
				Attr:    "src",
				Filters: []Filter{{Name: "clean_avatar"}},
			},
			"is_donor": {
				Selectors: []string{
					`h1 img[alt="Donor"]`,
					`img[src*="flag"][alt="Donor"]`,
				},
				Attr:    "alt",
				Filters: []Filter{{Name: "bool"}},
			},
			"join_date": {
				Selector: `td[class*="rowhead"]:contains("加入日期") + td`,
				Filters:  []Filter{{Name: "regexp", Args: `(\d{4}-\d{2}-\d{2})`}},
			},
			"last_access_at": {
				Selectors: []string{
					`td[class*="rowhead"]:contains("最近动向") + td`,
					`td[class*="rowhead"]:contains("最近動向") + td`,
					`td[class*="rowhead"]:contains("Last Action") + td`,
					`td[class*="rowhead"]:contains("Last Access") + td`,
				},
				Filters: []Filter{{Name: "regexp", Args: `^([^(]+)`}, {Name: "trim"}},
			},
			"true_uploaded": {
				Selectors: []string{
					`td[class*="rowhead"]:contains("传输") + td`,
					`td[class*="rowhead"]:contains("傳送") + td`,
					`td[class*="rowhead"]:contains("Transfers") + td`,
					`#info_block`,
				},
				Filters: []Filter{{Name: "regexp", Args: `(?i)(?:实际上传量|真实上传量|實際上傳量|真實上傳量|Real\s+Uploaded|Actual\s+Uploaded)[\s\S]{0,120}?([\d.,]+\s*[KMGTPE]?i?B)`}, {Name: "parse_size"}},
			},
			"true_downloaded": {
				Selectors: []string{
					`td[class*="rowhead"]:contains("传输") + td`,
					`td[class*="rowhead"]:contains("傳送") + td`,
					`td[class*="rowhead"]:contains("Transfers") + td`,
					`#info_block`,
				},
				Filters: []Filter{{Name: "regexp", Args: `(?i)(?:实际下载量|真实下载量|實際下載量|真實下載量|Real\s+Downloaded|Actual\s+Downloaded)[\s\S]{0,120}?([\d.,]+\s*[KMGTPE]?i?B)`}, {Name: "parse_size"}},
			},
			"true_ratio": {
				Selectors: []string{
					`td[class*="rowhead"]:contains("传输") + td`,
					`td[class*="rowhead"]:contains("傳送") + td`,
					`td[class*="rowhead"]:contains("Transfers") + td`,
					`#info_block`,
				},
				Filters: []Filter{{Name: "regexp", Args: `(?i)(?:实际分享率|真实分享率|實際分享率|真實分享率|Real\s+Ratio|Actual\s+Ratio)[^0-9+-]{0,120}([-+]?\d+\.?\d*)`}},
			},
			"seeding_bonus": {
				Selectors: []string{
					`td[class*="rowhead"]:contains("做种积分") + td`,
					`td[class*="rowhead"]:contains("做種積分") + td`,
					`td[class*="rowhead"]:contains("保种积分") + td`,
					`td[class*="rowhead"]:contains("保種積分") + td`,
					`td[class*="rowhead"]:contains("Seeding Bonus") + td`,
					`td[class*="rowhead"]:contains("Seed Bonus") + td`,
					`#info_block`,
				},
				Filters: []Filter{{Name: "regexp", Args: `(?i)(?:做种积分|做種積分|保种积分|保種積分|Seeding\s+Bonus|Seed\s+Bonus)[^0-9+-]{0,120}([-+]?[\d,]+\.?\d*)`}, {Name: "parse_number"}},
			},
			"claim_count": {
				Selectors: []string{
					`#info_block a[href*="claim.php"]:contains("/")`,
					`a[href*="claim.php"]:contains("/")`,
					`td[class*="rowhead"]:contains("认领") + td`,
					`td[class*="rowhead"]:contains("認領") + td`,
					`td[class*="rowhead"]:contains("Claim") + td`,
					`#info_block`,
				},
				Filters: []Filter{{Name: "claim_count"}},
			},
			"max_claim": {
				Selectors: []string{
					`#info_block a[href*="claim.php"]:contains("/")`,
					`a[href*="claim.php"]:contains("/")`,
					`td[class*="rowhead"]:contains("认领") + td`,
					`td[class*="rowhead"]:contains("認領") + td`,
					`td[class*="rowhead"]:contains("Claim") + td`,
					`#info_block`,
				},
				Filters: []Filter{{Name: "claim_max"}},
			},
		},
		Pages: []UserInfoPageDef{
			{
				Path:   "usercp.php",
				Params: map[string]string{"action": "personal"},
				Fields: map[string]ElementQuery{
					"personal_avatar": {
						Selectors: []string{`input[name="avatar"]`, `input[name='avatar']`},
						Attr:      "value",
						Filters:   []Filter{{Name: "clean_avatar"}},
					},
				},
			},
		},
	}
}

// DefaultNoLogin 返回标准 NexusPHP 未登录判定。
func DefaultNoLogin() NoLoginDef {
	return NoLoginDef{
		URLPattern: []string{"login.php", "takelogin.php"},
		// Selector 检测登录表单（NexusPHP login.php 通用特征），比纯文本"登录"精确：
		// 已登录的 index.php 常含"登录"字样（导航/统计），整页文本匹配会误判未登录
		// （财神 index 就是被旧 Content=["登录"] 误判 → 公告恒空）。
		Selector:   `form[action*="takelogin"], form[action*="login.php"], input[name="password"]`,
		Content:    []string{"请先登录", "您必须先登录"},
		HttpStatus: []int{401, 403},
	}
}

// EnsureDefaults 按 schema 填充站点定义的空段（不改原 def，返回副本）。
//
// 单入口分发（dispatch）：被 ~20 处调用（chain 层 + 全部 capability 适配器），
// 签名固定不变。内部按 def.Schema 路由：
//   - ensureMinimalDefaults：所有 schema 共享的路径默认（搜索入口 / HR 路径）。
//   - nexusphp（含空 schema，向后兼容现有 20 站与测试）：ensureNexusPHPDefaults 填选择器/capability 默认。
//   - 其它 schema（unit3d/gazelle/...）：仅通用项，不注入 NexusPHP 选择器，避免污染
//     （非 NexusPHP 站填入 table.torrents tr / font.color_* 会抓错页面）。各 schema 专属
//     默认在该 schema 适配器落地时补 ensureXxxDefaults 分支。
//
// 幂等：同一 def 多次调用结果一致（回归测试 TestEnsureDefaultsIdempotent 钉死）。
func EnsureDefaults(def *SiteMetadata) *SiteMetadata {
	cp := *def
	ensureMinimalDefaults(&cp)
	switch cp.Schema {
	case "", "nexusphp":
		ensureNexusPHPDefaults(&cp)
	case "unit3d":
		ensureUnit3DDefaults(&cp)
	case "gazelle":
		ensureGazelleDefaults(&cp)
	case "luminance":
		ensureLuminanceDefaults(&cp)
	case "tcg":
		ensureTCGDefaults(&cp)
	case "smallhorse":
		ensureSmallHorseDefaults(&cp)
	case "mteam":
		ensureMTeamDefaults(&cp)
	case "rartracker":
		ensureRartrackerDefaults(&cp)
	default:
		// 非 nexusphp：仅通用项，保留站点 YAML 自定义，不注入 NexusPHP 默认。
	}
	return &cp
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

// ── Gazelle（HTML，group→torrent 扁平化）──

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

// ── Luminance（HTML，Gazelle fork，单层表格）──

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

// -- TCG family --

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

// -- SmallHorse (Luminance-like lightweight tracker framework) --

// ensureSmallHorseDefaults fills defaults for MoviePilot's SmallHorse parser
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

// DefaultSmallHorseUserInfo mirrors MoviePilot app/modules/indexer/parser/
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

// ensureNexusPHPDefaults 填 NexusPHP 专属默认：选择器（selectors/user_info/no_login/detail）
// 与 capability 页面路径（freeleech/announcement/harem/messages）。各段独立判断，仅填空段——
// 站点 YAML 可只覆盖需要定制的部分（如 bonus/seeding 站点专属选择器）。原 EnsureDefaults
// 的 NexusPHP 逻辑原样保留，确保现有 20 站行为零回归。
func ensureNexusPHPDefaults(cp *SiteMetadata) {
	dflSelectors := DefaultNexusPHPSelectors()
	if cp.Selectors.Rows.Selector == "" && len(cp.Selectors.Rows.Selectors) == 0 {
		cp.Selectors.Rows = dflSelectors.Rows
	}
	cp.Selectors.Fields = mergeDefaultFields(cp.Selectors.Fields, dflSelectors.Fields)
	dfl := DefaultNexusPHPUserInfo()
	if cp.UserInfo.Path == "" {
		cp.UserInfo.Path = dfl.Path
	}
	if cp.UserInfo.DetailPath == "" {
		cp.UserInfo.DetailPath = dfl.DetailPath
	}
	// 逐键合并用户资料选择器（Fields=基础页 usercp，DetailFields=富资料页 userdetails）：
	// 站点 YAML 只覆盖需要定制的键（如 bonus），其余键继承 NexusPHP 默认。
	// 原先是「整段非空就不填默认」——站点只配 bonus 会导致 uploaded/downloaded/ratio 全部丢失
	// （站点明细恒空，见 audiences/cbg/ubits/tangpt 校准）。逐键合并修复此问题，且对已显式配齐
	// 全部键的站点零影响（其键已存在，不被覆盖）。mergeDefaultFields 新建 map，不污染共享原 def。
	cp.UserInfo.Fields = mergeDefaultFields(cp.UserInfo.Fields, dfl.Fields)
	cp.UserInfo.DetailFields = mergeDefaultFields(cp.UserInfo.DetailFields, dfl.DetailFields)
	if cp.Schema == "nexusphp" {
		cp.UserInfo.Pages = mergeDefaultUserInfoPages(cp.UserInfo.Pages, dfl.Pages)
	}
	if len(cp.NoLogin.URLPattern) == 0 && len(cp.NoLogin.Content) == 0 {
		cp.NoLogin = DefaultNoLogin()
	}
	// 全站免费/促销 + 公告：首页横幅/公告区通用，默认开启；仅填空 Path。
	cp.Freeleech.Enabled, cp.Freeleech.Path = applyInfoDefaults(cp.Freeleech.Enabled, cp.Freeleech.Path, DefaultNexusPHPFreeleech())
	cp.Announcement.Enabled, cp.Announcement.Path = applyInfoDefaults(cp.Announcement.Enabled, cp.Announcement.Path, DefaultNexusPHPAnnouncement())
	// 后宫：默认开启 invite.php
	if !cp.Harem.Enabled && cp.Harem.InvitePath == "" {
		cp.Harem.Enabled = true
	}
	if cp.Harem.InvitePath == "" {
		cp.Harem.InvitePath = "invite.php"
	}
	if cp.Harem.InviteePath == "" {
		cp.Harem.InviteePath = "invite.php"
	}
	if cp.Harem.MaxPages <= 0 {
		cp.Harem.MaxPages = 20
	}
	// 勋章墙：默认开启 medal.php（商店）+ userdetails.php（已拥有勋章）。站点可 enabled:false 关闭。
	if !cp.Medal.Enabled && cp.Medal.ShopPath == "" {
		cp.Medal.Enabled = true
	}
	if cp.Medal.ShopPath == "" {
		cp.Medal.ShopPath = "medal.php"
	}
	if cp.Medal.OwnPath == "" {
		cp.Medal.OwnPath = "userdetails.php"
	}
	// 私信：默认开启 messages.php（CSRF 字段空=不带 token 直接 POST）
	if !cp.Messages.Enabled && cp.Messages.ListPath == "" {
		cp.Messages.Enabled = true
	}
	if cp.Messages.ListPath == "" {
		cp.Messages.ListPath = "messages.php"
	}
	// 签到：默认开启 attendance.php + POST。签到前总先探测首页提取真实签到表单/链接
	// （探测失败回退 Path），故无需单独的探测开关。
	if !cp.Checkin.Enabled && cp.Checkin.Path == "" {
		cp.Checkin.Enabled = true
	}
	if cp.Checkin.Path == "" {
		cp.Checkin.Path = "attendance.php"
	}
	if cp.Checkin.Method == "" {
		cp.Checkin.Method = "POST"
	}
	// 种子详情页：默认开启。逐字段合并默认值——站点可只覆盖个别字段（如 title），
	// 其余（description/screenshots/...）继承 NexusPHP 默认，无需为改一个字段而重写整块。
	if !cp.Detail.Enabled && len(cp.Detail.Fields) == 0 {
		cp.Detail.Enabled = true
	}
	if cp.Detail.Enabled {
		dfl := DefaultNexusPHPDetail().Fields
		if len(cp.Detail.Fields) == 0 {
			cp.Detail.Fields = dfl
		} else {
			// 合并到新 map，避免改写站点原 def 的 map（def 可能被复用）。
			merged := make(map[string]ElementQuery, len(cp.Detail.Fields)+len(dfl))
			for k, v := range cp.Detail.Fields {
				merged[k] = v
			}
			for k, v := range dfl {
				if _, ok := merged[k]; !ok {
					merged[k] = v
				}
			}
			cp.Detail.Fields = merged
		}
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

// applyInfoDefaults 合并 Freeleech/Announcement 默认值：站点未显式配置（Enabled=false 且 Path 空）
// 时整体取默认（开启 + 默认路径）；否则仅补空 Path。站点 YAML 可用 enabled:false 关闭。
func applyInfoDefaults(curEnabled bool, curPath string, def FreeleechDef) (bool, string) {
	if !curEnabled && curPath == "" {
		return def.Enabled, def.Path
	}
	if curPath == "" {
		curPath = def.Path
	}
	return curEnabled, curPath
}

// mergeDefaultFields 把 def 里「site 未定义的键」逐个补入，返回新 map（不覆盖 site 已配的键，不污染 site 原 map）。
// 用于 NexusPHP UserInfo.Fields / DetailFields 的默认值逐键合并——站点只覆盖个别键（如 bonus），
// 其余键仍继承默认（uploaded/downloaded/ratio/...）。新建 map 是因 EnsureDefaults 浅拷贝 struct，
// map 字段仍与原 def 共享，原地改写会污染 Registry 里的共享定义。
func mergeDefaultFields(site, def map[string]ElementQuery) map[string]ElementQuery {
	if len(def) == 0 {
		return site
	}
	out := make(map[string]ElementQuery, len(site)+len(def))
	for k, v := range site {
		out[k] = v
	}
	for k, v := range def {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

func mergeDefaultUserInfoPages(sitePages, defaultPages []UserInfoPageDef) []UserInfoPageDef {
	if len(defaultPages) == 0 {
		return sitePages
	}
	covered := map[string]bool{}
	for _, page := range sitePages {
		for key := range page.Fields {
			covered[key] = true
		}
		for _, key := range page.UseFields {
			covered[key] = true
		}
	}
	out := append([]UserInfoPageDef{}, sitePages...)
	for _, page := range defaultPages {
		needed := false
		for key := range page.Fields {
			if !covered[key] {
				needed = true
				covered[key] = true
			}
		}
		for _, key := range page.UseFields {
			if !covered[key] {
				needed = true
				covered[key] = true
			}
		}
		if needed {
			out = append(out, page)
		}
	}
	return out
}

// DefaultNexusPHPFreeleech 返回全站免费/促销检测默认值（首页 index.php）。
func DefaultNexusPHPFreeleech() FreeleechDef {
	return FreeleechDef{Enabled: true, Path: "index.php"}
}

// DefaultNexusPHPAnnouncement 返回站点公告抓取默认值（首页 index.php）。复用 FreeleechDef 形状。
func DefaultNexusPHPAnnouncement() FreeleechDef {
	return FreeleechDef{Enabled: true, Path: "index.php"}
}

// DefaultNexusPHPDetail 返回 details.php 详情页选择器（NexusPHP 框架常量）。
// 各站 DOM 略有差异，此为代表性默认；接真实站点后按需在站点 YAML 覆盖（可用
// POST /api/v1/sites/inspect 对真实 details.php 校准）。
func DefaultNexusPHPDetail() DetailDef {
	return DetailDef{
		Enabled: true,
		Fields: map[string]ElementQuery{
			"title":       {Selector: `a[href*="details.php"][title]`, Attr: "title"},
			"description": {Selector: `#kdescr, .torrentdesc, #kdetails`, Attr: "html"},
			"screenshots": {Selector: `#kdescr img, .torrent_screenshot img`, Attr: "src"},
			"poster":      {Selector: `.torrent封面 img, .torrent-info img`, Attr: "src"},
			"imdb_id": {
				Selector: `a[href*="imdb.com/title/"], a[href*="imdb.cn/title/"]`,
				Attr:     "href",
				Filters:  []Filter{{Name: "regexp", Args: `tt\d+`}},
			},
			"douban_id": {
				Selector: `a[href*="movie.douban.com/subject/"], a[href*="douban.com/subject/"]`,
				Attr:     "href",
				Filters:  []Filter{{Name: "regexp", Args: `subject/(\d+):::1`}},
			},
			"download": {Selector: `a[href^="download.php"]`, Attr: "href"},
		},
	}
}

// EnsureLoginDefaults 填 LoginDef 的空字段为好学/NexusPHP 标准默认值（不改原 def，返回副本）。
func EnsureLoginDefaults(def *SiteMetadata) *SiteMetadata {
	def = EnsureDefaults(def)
	if !def.Login.Enabled {
		return def
	}
	cp := *def
	if cp.Login.PagePath == "" {
		cp.Login.PagePath = "login.php"
	}
	if cp.Login.ActionPath == "" {
		cp.Login.ActionPath = "takelogin.php"
	}
	fm := &cp.Login.Form
	if fm.Username == "" {
		fm.Username = "username"
	}
	if fm.Password == "" {
		fm.Password = "password"
	}
	if fm.ImageHash == "" {
		fm.ImageHash = "imagehash"
	}
	if fm.ImageString == "" {
		fm.ImageString = "imagestring"
	}
	if fm.TwoStepCode == "" {
		fm.TwoStepCode = "two_step_code"
	}
	if cp.Login.Captcha.GenPath == "" {
		cp.Login.Captcha.GenPath = "image.php"
	}
	if cp.Login.Captcha.HashInputSelector == "" {
		cp.Login.Captcha.HashInputSelector = `input[name="imagehash"]`
	}
	return &cp
}
