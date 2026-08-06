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
