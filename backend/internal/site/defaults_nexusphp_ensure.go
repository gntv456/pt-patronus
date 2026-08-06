package site

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
