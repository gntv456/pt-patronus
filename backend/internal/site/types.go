// Package site 实现站点适配框架：Schema（按平台）+ Definition（按站点，YAML 配置驱动）
// 两层抽象，借鉴 PT-depiler 的架构。Selector 引擎用 goquery，新增站点零代码。
package site

import (
	"strconv"
	"strings"
	"time"
)

// ElementQuery 是配置驱动的字段提取规则（抄 PT-depiler IElementQuery 的实用子集）。
type ElementQuery struct {
	Selector  string   `yaml:"selector" binding:"max=512"`       // 相对上下文的 CSS 选择器（单选）
	Selectors []string `yaml:"selectors" binding:"dive,max=512"` // 回退选择器列表：按序逐个尝试，取首个非空结果。优先于 Selector
	Text      string   `yaml:"text" binding:"max=2000"`          // 固定文本值；对应 PT-depiler text
	Ancestor  string   `yaml:"ancestor" binding:"max=512"`       // 命中节点后向上找到最近祖先，再读取/继续 find
	Find      string   `yaml:"find" binding:"max=512"`           // 命中节点/祖先内继续查找子节点
	Parent    bool     `yaml:"parent"`                           // 命中节点后先爬到父节点再取文本/属性。为 font.color_* 锚点设计——值是 label 的兄弟文本节点，取父节点文本才能拿到「上传量: 104.564 TB」整串
	Attr      string   `yaml:"attr" binding:"max=64"`            // 空/"text" 取文本；"html" 取 HTML；否则取属性（href/src/...）
	Data      string   `yaml:"data" binding:"max=128"`           // PT-depiler data 字段；HTML 读取 data-*，JSON 读取同名路径
	Remove    string   `yaml:"remove" binding:"max=512"`         // 提取前移除的子选择器（如广告/图标）
	Filters   []Filter `yaml:"filters" binding:"dive"`           // 后处理链
}

// Filter 后处理。Name ∈ {regexp, replace, trim, lower, upper, parse_number}。
//   - regexp:   Args 为正则（可带 :::N 后缀指定取第 N 个捕获组，默认组 1；:::0 取全匹配）；
//     无捕获组取全匹配。用于「当前活动: 27 0」之类一锚多值场景按组拆字段。
//   - replace:  Args 形如 "old:::new"
//   - parse_number: 抽取首个数字（去千分位逗号）
type Filter struct {
	Name string `yaml:"name" binding:"max=32"`
	Args string `yaml:"args" binding:"max=512"`
}

// TorrentDetail 是单条种子的详情页（简介/截图/MediaInfo/外链 ID/技术规格）。
// 由 def.Detail.Fields 选择器驱动；字段 best-effort（未抽到则留空）。
// 选择器覆盖较广，实际可用字段因站点而异——前端按非空渲染。
type TorrentDetail struct {
	Title       string   `json:"title"`
	SubTitle    string   `json:"subtitle,omitempty"`
	PosterURL   string   `json:"poster_url,omitempty"`  // 详情页海报/封面
	Description string   `json:"description,omitempty"` // 原始 HTML（前端渲染）
	Screenshots []string `json:"screenshots,omitempty"` // 绝对 URL
	MediaInfo   string   `json:"mediainfo,omitempty"`   // 原始 MediaInfo 文本
	FileList    []string `json:"file_list,omitempty"`   // 每个文件一行（路径）
	IMDBID      string   `json:"imdb_id,omitempty"`     // tt1234567
	DoubanID    string   `json:"douban_id,omitempty"`
	TMDBID      string   `json:"tmdb_id,omitempty"`
	Category    string   `json:"category,omitempty"`
	Size        string   `json:"size,omitempty"`
	Codec       string   `json:"codec,omitempty"`      // x264/x265/...
	Source      string   `json:"source,omitempty"`     // BluRay/WEB-DL/...
	Resolution  string   `json:"resolution,omitempty"` // 1080p/2160p/...
	Seeders     int      `json:"seeders,omitempty"`
	Leechers    int      `json:"leechers,omitempty"`
	DownloadURL string   `json:"download_url,omitempty"` // 解析后的绝对 URL
	DetailURL   string   `json:"detail_url"`             // 实际抓取的页面 URL
}

// Torrent 是一条种子。
type Torrent struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	SubTitle    string   `json:"subtitle,omitempty"`
	DetailURL   string   `json:"detail_url"`
	DownloadURL string   `json:"download_url"`
	Size        string   `json:"size"`
	Seeders     int      `json:"seeders"`
	Leechers    int      `json:"leechers"`
	Completed   int      `json:"completed"`
	Time        string   `json:"time,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Free        bool     `json:"free,omitempty"`
	SiteName    string   `json:"site_name,omitempty"` // 来源站（工作流/RSS 保留；SearchSites 已按站聚合但 Torrent 单条无站名）
	// 规则引擎结构化字段（publish_time / downloadvolumefactor 规则用）。
	// 未填充时引擎降级：Free 近似 factor、PubDate 零值时 publish_time 规则放行。
	PubDate              time.Time `json:"pub_date,omitempty"`
	DownloadVolumeFactor float64   `json:"downloadvolumefactor,omitempty"` // 0=FREE,1=正常,0.5/2=促销；0 且 Free=false 视为未知→1
	UploadVolumeFactor   float64   `json:"uploadvolumefactor,omitempty"`   // 上传因子：0=未设/正常,2=双倍上传（对齐 MP-2，此前遗漏）
	Labels               []string  `json:"labels,omitempty"`
}

// HRTorrent 是一条处于 H&R（Hit & Run）考核中的种子。
// 好学/NexusPHP 源码语义：status 1=监控中(Inspecting)/2=达标(Reached)/3=未达标(Unreached)/4=豁免(Pardoned)。
type HRTorrent struct {
	TorrentID    string  `json:"torrent_id"`
	Title        string  `json:"title"`
	Size         string  `json:"size"`
	Ratio        float64 `json:"ratio"`
	SeedTime     string  `json:"seed_time"`     // 原始「做种时间」文本
	Status       string  `json:"status"`        // 原始状态文本（监控中/达标/未达标/豁免…）
	DeadlineLeft string  `json:"deadline_left"` // 原始「剩余考核时间」文本（如 12:30:00）
	DetailURL    string  `json:"detail_url"`
}

// SeedingItem 是做种列表中的一条种子（逐条明细，供保种契约按官种/体积维度聚合）。
type SeedingItem struct {
	Title  string   `json:"title"`
	Size   int64    `json:"size"`             // 字节
	Labels []string `json:"labels,omitempty"` // 行内促销/官方等标签文本（best-effort）
}

// SeedingAggregate 是做种明细的一个聚合（全部或官种子集）。
type SeedingAggregate struct {
	Count int   `json:"count"`
	Size  int64 `json:"size"` // 字节
}

// SeedingStats 是做种明细按官种维度聚合的结果。
//   - Total：全部做种；Official：官种子集（IsOfficial 命中）
//   - Supported=false 表示该站不支持逐行做种列表解析（契约层应回退 UserInfo 汇总并提示）
type SeedingStats struct {
	Total     SeedingAggregate `json:"total"`
	Official  SeedingAggregate `json:"official"`
	Supported bool             `json:"supported"`
}

// UserInfo 是站点用户资料。Level 用于好学 VIP 判定。
type UserInfo struct {
	ID                        string  `json:"id"`
	Name                      string  `json:"name"`
	Email                     string  `json:"email,omitempty"` // 资料页邮箱（best-effort：部分站不展示或部分隐藏 a***@b.com，原样保留）
	Level                     string  `json:"level"`
	LevelID                   int     `json:"level_id,omitempty"`
	NumericUserID             int     `json:"numeric_user_id,omitempty"`
	Avatar                    string  `json:"avatar,omitempty"`
	PersonalAvatar            string  `json:"personal_avatar,omitempty"`
	IsDonor                   bool    `json:"is_donor,omitempty"`
	Uploaded                  int64   `json:"uploaded"`
	Downloaded                int64   `json:"downloaded"`
	Ratio                     float64 `json:"ratio"`
	TrueUploaded              int64   `json:"true_uploaded,omitempty"`
	TrueDownloaded            int64   `json:"true_downloaded,omitempty"`
	TrueRatio                 float64 `json:"true_ratio,omitempty"`
	Seeding                   int     `json:"seeding"`
	Leeching                  int     `json:"leeching"`
	SeedingSize               int64   `json:"seeding_size"`
	Uploads                   int     `json:"uploads"` // 发布数（用户上传的种子数）
	Adoptions                 int     `json:"adoptions,omitempty"`
	Posts                     int     `json:"posts,omitempty"`
	Snatches                  int     `json:"snatches,omitempty"`
	TotalTraffic              int64   `json:"total_traffic,omitempty"`
	UniqueGroups              int     `json:"unique_groups,omitempty"`
	PerfectFLAC               int     `json:"perfect_flac,omitempty"`
	Invited                   int     `json:"invited,omitempty"`
	Bonus                     float64 `json:"bonus"`
	BonusPerHour              float64 `json:"bonus_per_hour"`
	SeedingBonus              float64 `json:"seeding_bonus,omitempty"`
	SeedingBonusPerHour       float64 `json:"seeding_bonus_per_hour,omitempty"`
	RescueSeedingBonusPerHour float64 `json:"rescue_seeding_bonus_per_hour,omitempty"`
	SeedingTime               int64   `json:"seeding_time,omitempty"`
	ClaimCount                int     `json:"claim_count,omitempty"`
	MaxClaim                  int     `json:"max_claim,omitempty"`
	JoinDate                  string  `json:"join_date,omitempty"`
	LastAccessAt              string  `json:"last_access_at,omitempty"`
}

// SiteMetadata 是一个站点的定义（definitions/*.yaml 反序列化目标）。
type SiteMetadata struct {
	ID          string            `yaml:"id"`
	Name        string            `yaml:"name"`
	Schema      string            `yaml:"schema"` // nexusphp | mteam | unit3d | gazelle | tnode | rousi
	URLs        []string          `yaml:"urls"`
	SearchEntry string            `yaml:"search_entry"` // 相对路径，默认 torrents.php
	SearchParam string            `yaml:"search_param"` // 关键字参数名，默认 search
	ExtraParam  map[string]string `yaml:"extra_param"`  // 额外固定参数
	// Format 显式标注响应格式，供 generic 适配器分发 HTML/JSON 解析；空=响应体首字节嗅探。
	Format string `yaml:"format"` // "" 自动 | "html" | "json"
	// SearchCategories 是「按媒体类型搜索」的每站分类映射：键 movie/tv/music → 该站 cat ID 列表。
	// 命中时 NexusPHP 搜索 URL 追加 cat[]=<id>（精准服务端过滤）。未配置则不发 cat[]，
	// 由 chain 层 FilterByCategory 兜底（按行分类文本丢弃明显异类，多为音乐）。
	SearchCategories  map[string][]string `yaml:"search_categories"`
	Selectors         SiteSelectors       `yaml:"selectors"`
	UserInfo          UserInfoDef         `yaml:"user_info"`
	HR                HRDef               `yaml:"hr"`
	Freeleech         FreeleechDef        `yaml:"freeleech"`
	Announcement      AnnouncementDef     `yaml:"announcement"`
	Harem             HaremDef            `yaml:"harem"`
	Medal             MedalDef            `yaml:"medal"`
	Messages          MessageDef          `yaml:"messages"`
	Checkin           CheckinDef          `yaml:"checkin"`
	Detail            DetailDef           `yaml:"detail"`
	NoLogin           NoLoginDef          `yaml:"no_login"`
	Login             LoginDef            `yaml:"login"`
	Categories        []CategoryDef       `yaml:"categories"`
	AKA               []string            `yaml:"aka,omitempty"`
	Description       string              `yaml:"description,omitempty"`
	Tags              []string            `yaml:"tags,omitempty"`
	TimezoneOffset    string              `yaml:"timezone_offset,omitempty"`
	LegacyURLs        []string            `yaml:"legacy_urls,omitempty"`
	Dead              bool                `yaml:"dead,omitempty"`
	LevelRequirements []LevelRequirement  `yaml:"level_requirements,omitempty"`
}

// LevelRequirement 描述站点等级体系的一级（移植自 PT-depiler levelRequirements，纯数据）。
// 214 个站点定义含此数据，供前端展示等级要求；好学 VIP 判定仍走 UserInfoDef.VIPLevels。
type LevelRequirement struct {
	ID           int      `yaml:"id" json:"id"`
	Name         string   `yaml:"name" json:"name"`
	NameAKA      []string `yaml:"name_aka,omitempty" json:"name_aka,omitempty"`
	Interval     string   `yaml:"interval,omitempty" json:"interval,omitempty"`     // ISO8601 duration，如 P5W
	Downloaded   string   `yaml:"downloaded,omitempty" json:"downloaded,omitempty"` // 阈值，带单位如 "200GB"
	Uploaded     string   `yaml:"uploaded,omitempty" json:"uploaded,omitempty"`
	Ratio        float64  `yaml:"ratio,omitempty" json:"ratio,omitempty"`
	Bonus        float64  `yaml:"bonus,omitempty" json:"bonus,omitempty"`
	SeedingBonus float64  `yaml:"seeding_bonus,omitempty" json:"seeding_bonus,omitempty"`
	SeedingTime  string   `yaml:"seeding_time,omitempty" json:"seeding_time,omitempty"`
	Uploads      int      `yaml:"uploads,omitempty" json:"uploads,omitempty"`
	IsKept       bool     `yaml:"is_kept,omitempty" json:"is_kept,omitempty"`       // 保号等级
	GroupType    string   `yaml:"group_type,omitempty" json:"group_type,omitempty"` // user/vip/manager
	Privilege    string   `yaml:"privilege,omitempty" json:"privilege,omitempty"`
}

// HRDef 描述站点的 H&R（Hit & Run）列表页结构。Enabled 为 false 时跳过 H&R 检测。
type HRDef struct {
	Enabled bool                    `yaml:"enabled"` // 是否启用 H&R 检测（非所有站开启 H&R）
	Path    string                  `yaml:"path"`    // 相对路径，默认 getusertorrentlist.php?type=hr
	Rows    ElementQuery            `yaml:"rows"`
	Fields  map[string]ElementQuery `yaml:"fields"` // 约定键: title/size/ratio/seedtime/status/deadline/detail
}

// FreeleechStatus 是站点全站免费/促销状态。
type FreeleechStatus string

const (
	FreeleechActive   FreeleechStatus = "active"   // 正在站免
	FreeleechUpcoming FreeleechStatus = "upcoming" // 即将站免
	FreeleechNone     FreeleechStatus = "none"     // 暂无站免
	FreeleechFailed   FreeleechStatus = "failed"   // 读取失败
)

// SiteFreeleech 是一次站点全站免费/促销探测结果。区别于单种 Free（torrents.php 行内免费图标）：
// 这里抓的是站点首页横幅级别的「全站免费 / 2X / 站免 / 促销」公告。
type SiteFreeleech struct {
	SiteID    uint            `json:"site_id"`
	SiteName  string          `json:"site_name"`
	Status    FreeleechStatus `json:"status"`
	Title     string          `json:"title,omitempty"`
	Content   string          `json:"content,omitempty"`
	Link      string          `json:"link,omitempty"`
	Color     string          `json:"color,omitempty"` // 横幅背景色（用于「即将站免」蓝色判定）
	Error     string          `json:"error,omitempty"`
	FetchedAt time.Time       `json:"fetched_at"`
}

// FreeleechDef 描述站点全站免费/促销检测。默认抓 index.php 首页横幅，所有 NexusPHP 站通用。
type FreeleechDef struct {
	Enabled bool   `yaml:"enabled"` // 默认 true（首页横幅通用）；站点可显式关闭
	Path    string `yaml:"path"`    // 相对路径，默认 index.php
}

// AnnouncementItem 是一条结构化公告（标题/日期/正文）。解析失败时退化为单条 raw 文本。
type AnnouncementItem struct {
	Title   string `json:"title,omitempty"`
	Date    string `json:"date,omitempty"`
	Content string `json:"content,omitempty"`
}

// SiteAnnouncement 是一次站点公告抓取结果。
type SiteAnnouncement struct {
	SiteID    uint               `json:"site_id"`
	SiteName  string             `json:"site_name"`
	RawText   string             `json:"raw_text,omitempty"` // 原始公告文本（解析失败时回退展示）
	Items     []AnnouncementItem `json:"items"`
	FetchedAt time.Time          `json:"fetched_at"`
	Error     string             `json:"error,omitempty"`
}

// AnnouncementDef 描述站点公告抓取。默认抓 index.php 首页公告区。
type AnnouncementDef struct {
	Enabled bool   `yaml:"enabled"` // 默认 true（首页公告区通用）
	Path    string `yaml:"path"`    // 相对路径，默认 index.php
}

// HaremDef 描述「后宫」（邀请库存 + 被邀请人花名册）抓取。默认 invite.php。
type HaremDef struct {
	Enabled     bool   `yaml:"enabled"`      // 默认 true
	InvitePath  string `yaml:"invite_path"`  // 邀请页，默认 invite.php
	InviteePath string `yaml:"invitee_path"` // 被邀请人页，默认 invite.php（同页 ?menu=invitee）
	MaxPages    int    `yaml:"max_pages"`    // 被邀请人翻页上限，默认 20
}

// MedalDef 描述「勋章墙」（勋章商店 + 已拥有勋章）抓取。
//   - ShopPath：勋章商店页，默认 medal.php（NexusPHP）；好学等现代皮肤用卡片/表格双布局。
//   - OwnPath：已拥有勋章来源页，默认 userdetails.php（需先发现 uid）。
//   - Enabled 默认 true（NexusPHP 通用）；站点可显式关闭。
type MedalDef struct {
	Enabled  bool   `yaml:"enabled"`   // 默认 true
	ShopPath string `yaml:"shop_path"` // 默认 medal.php
	OwnPath  string `yaml:"own_path"`  // 默认 userdetails.php
}

// InviteStatus 是邀请库存快照。
//   - NexusPHP 不设 shop 价格（PermanentInvitePrice/TemporaryInvitePrice 恒 -1）→ buyable* 恒 0
//   - MTeam 设 TemporaryInvitePrice=80000 → buyableTemporary = bonus/80000
type InviteStatus struct {
	CanInvite            bool    `json:"can_invite"`
	Reason               string  `json:"reason,omitempty"`
	PermanentCount       int     `json:"permanent_count"`
	TemporaryCount       int     `json:"temporary_count"`
	Bonus                float64 `json:"bonus"`
	PermanentInvitePrice float64 `json:"permanent_invite_price"` // -1=未知/无商店
	TemporaryInvitePrice float64 `json:"temporary_invite_price"` // -1=未知/无商店
}

// TotalRemain 剩余邀请总数（永+临）。
func (s *InviteStatus) TotalRemain() int { return s.PermanentCount + s.TemporaryCount }

// BuyablePermanent 可购买的永久邀请数（price>0 且 bonus>0 时 bonus/price，向零截断）。
func (s *InviteStatus) BuyablePermanent() int {
	if s.PermanentInvitePrice > 0 && s.Bonus > 0 {
		return int(s.Bonus / s.PermanentInvitePrice)
	}
	return 0
}

// BuyableTemporary 可购买的临时邀请数。
func (s *InviteStatus) BuyableTemporary() int {
	if s.TemporaryInvitePrice > 0 && s.Bonus > 0 {
		return int(s.Bonus / s.TemporaryInvitePrice)
	}
	return 0
}

// TotalBuyable 总可购买数。
func (s *InviteStatus) TotalBuyable() int { return s.BuyablePermanent() + s.BuyableTemporary() }

// InviteeInfo 是一名被邀请人。
type InviteeInfo struct {
	UserID     string `json:"user_id,omitempty"` // userdetails.php?id=，用于排除"自己"
	Username   string `json:"username"`
	Email      string `json:"email,omitempty"`
	Uploaded   int64  `json:"uploaded"`
	Downloaded int64  `json:"downloaded"`
	Ratio      string `json:"ratio,omitempty"`
	Status     string `json:"status"` // confirmed / pending / disabled
	Enabled    bool   `json:"enabled"`
}

// IsDead 是否已禁用/封禁（enabled=false 或状态命中禁用关键词）。
func (i *InviteeInfo) IsDead() bool {
	if !i.Enabled {
		return true
	}
	s := strings.ToLower(strings.TrimSpace(i.Status))
	if s == "" {
		return false
	}
	switch s {
	case "no", "disabled", "disable", "banned", "ban", "suspended", "forbidden":
		return true
	}
	return containsAny(s, "禁用", "封禁", "停用", "禁止", "未启用")
}

// IsPending 是否待确认/未激活。
func (i *InviteeInfo) IsPending() bool {
	s := strings.ToLower(strings.TrimSpace(i.Status))
	return s == "pending" || containsAny(s, "待确认", "待確認", "未确认", "未確認", "未激活")
}

// HasNoTrafficData 是否无流量数据（纯做种/新用户）。
func (i *InviteeInfo) HasNoTrafficData() bool { return i.Uploaded <= 0 && i.Downloaded <= 0 }

// IsLowRatio 分享率 <1.0（需 downloaded>0 才评估，纯做种者不判）。
func (i *InviteeInfo) IsLowRatio() bool {
	if i.Downloaded <= 0 {
		return false
	}
	parsed := parseRatioFloat(i.Ratio)
	if parsed < 0 {
		parsed = float64(i.Uploaded) / float64(i.Downloaded)
	}
	return parsed >= 0 && parsed < 1.0
}

// parseRatioFloat 解析分享率串（去千分位逗号；∞/inf 视为极大值；失败返回 -1）。
func parseRatioFloat(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	switch strings.ToLower(s) {
	case "", "∞", "inf", "infinite":
		if s == "" {
			return -1
		}
		return 1e9
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return f
}

// PrescriptionItem 是单站后宫药单行。
type PrescriptionItem struct {
	SiteID         uint          `json:"site_id"`
	SiteName       string        `json:"site_name"`
	Remain         int           `json:"remain"`          // = status.TotalRemain()
	PermanentCount int           `json:"permanent_count"` // 剩余永久邀请（供总览求和）
	TemporaryCount int           `json:"temporary_count"` // 剩余临时邀请
	CanBuy         int           `json:"can_buy"`         // = status.TotalBuyable()
	CanInvite      bool          `json:"can_invite"`
	HaremCount     int           `json:"harem_count"` // 被邀请人数
	DeadCount      int           `json:"dead_count"`  // 已禁用数
	Invitees       []InviteeInfo `json:"invitees"`
	Error          string        `json:"error,omitempty"`
}

// HaremOverview 是 7 格汇总（仅成功站点）。
type HaremOverview struct {
	TotalSites       int `json:"total_sites"`
	TotalInvitees    int `json:"total_invitees"`
	TotalPermInvites int `json:"total_perm_invites"`
	TotalTempInvites int `json:"total_temp_invites"`
	TotalLowRatio    int `json:"total_low_ratio"`
	TotalBanned      int `json:"total_banned"`
	TotalNoData      int `json:"total_no_data"`
}

// MessageDef 描述站点私信抓取。默认 messages.php。
type MessageDef struct {
	Enabled   bool   `yaml:"enabled"`    // 默认 true
	ListPath  string `yaml:"list_path"`  // 列表页，默认 messages.php
	CSRFField string `yaml:"csrf_field"` // 回复所需 CSRF 字段名（空=无 CSRF，直接 POST）
}

// CheckinDef 描述站点签到（NexusPHP 标准 POST attendance.php）。解决三类签到失败：
//   - Path：配真实签到端点（attendance.php 不存在的站，404）。
//   - 签到前总先 GET 首页探测真实签到表单/链接 + 提取 hidden（含 formhash），命中则用之，
//     否则回退 Path+Method+Form（解决空 POST 缺 formhash 被拒，500）。
//   - Headers：配自定义请求头（Referer/UA/X-CSRF，绕 WAF/CSRF，如 468）。
//   - Form：固定表单字段（如 qdxq=fd），与探测提取的 hidden 合并。
type CheckinDef struct {
	Enabled bool              `yaml:"enabled"` // 默认 true
	Path    string            `yaml:"path"`    // 签到端点相对路径，默认 attendance.php
	Method  string            `yaml:"method"`  // GET|POST，默认 POST；探测命中表单时按表单 method
	Form    map[string]string `yaml:"form"`    // 固定表单字段（如 qdxq=fd）；与探测提取的 hidden 合并
	Headers map[string]string `yaml:"headers"` // 自定义请求头（Referer/UA/X-CSRF，绕 WAF/CSRF）
	UTLS    bool              `yaml:"utls"`    // 启用 uTLS（Chrome 指纹）拨号绕 TLS 指纹 WAF（如 SafeLine 雷池 468）
}

// DetailDef 描述种子详情页（details.php?id=...）。Enabled 默认 true（NexusPHP 通用）。
// Fields 为空时 EnsureDefaults 填入 DefaultNexusPHPDetail（详见 defaults.go）。
// 约定键: title/subtitle/poster/description/screenshots/mediainfo/imdb_id/douban_id/
//
//	category/size/codec/source/resolution/download/file_list
//
// screenshots 与 file_list 为多值字段（收集所有匹配）。
type DetailDef struct {
	Enabled bool                    `yaml:"enabled"`
	Fields  map[string]ElementQuery `yaml:"fields"`
}

// MailboxType 私信邮箱。
type MailboxType string

const (
	MailboxInbox   MailboxType = "inbox"
	MailboxSent    MailboxType = "sent"
	MailboxSystem  MailboxType = "system"
	MailboxCheater MailboxType = "cheater"
	MailboxReports MailboxType = "reports"
	MailboxStaff   MailboxType = "staff"
)

// AdminActions 是消息的写操作 URL/ID（可能为空）。
type AdminActions struct {
	ReplyURL  string `json:"reply_url,omitempty"`
	DeleteURL string `json:"delete_url,omitempty"`
	ItemID    string `json:"item_id,omitempty"`
}

// Message 是一条私信。
type Message struct {
	ID           uint64        `json:"id"`
	SiteID       uint          `json:"site_id"`
	SiteName     string        `json:"site_name"`
	Sender       string        `json:"sender"`
	Title        string        `json:"title"`
	Date         string        `json:"date"`
	URL          string        `json:"url"`
	IsRead       bool          `json:"is_read"`
	Content      string        `json:"content,omitempty"`
	Mailbox      MailboxType   `json:"mailbox"`
	Subtitle     string        `json:"subtitle,omitempty"`
	AdminActions *AdminActions `json:"admin_actions,omitempty"`
	FetchedAt    time.Time     `json:"fetched_at"`
}

// MessageReplyResult 是回复结果。
type MessageReplyResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// BatchResult 是批量操作结果。
type BatchResult struct {
	Success int      `json:"success"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// SiteSelectors 描述搜索结果页结构。
type SiteSelectors struct {
	Rows   ElementQuery            `yaml:"rows"`
	Fields map[string]ElementQuery `yaml:"fields"` // 约定键: title/subtitle/detail/download/size/seeders/leechers/completed/time/category/free
}

// UserInfoDef 描述用户资料页结构。
//
// NexusPHP 站统计字段分布在两处页面上，故分两套字段：
//   - Fields（基础页，默认 usercp.php）：uploaded/downloaded/ratio/seeding/leeching/bonus 等用户栏统计
//   - DetailFields（富资料页，默认 userdetails.php?id=<uid>，需先发现 uid）：level/join_date 等 rowhead 行
//
// 适配器 FetchUserInfo 会先抓基础页，再用发现到的 uid 抓富资料页合并；任一页失败 best-effort 不致命。
type UserInfoDef struct {
	Path               string                  `yaml:"path"`                  // 基础统计页，默认 usercp.php
	Fields             map[string]ElementQuery `yaml:"fields"`                // 基础页字段。约定键: name/level/level_id/numeric_user_id/avatar/personal_avatar/is_donor/uploaded/downloaded/ratio/true_uploaded/true_downloaded/true_ratio/seeding/leeching/seeding_size/uploads/uploads_add/adoptions/posts/snatches/total_traffic/unique_groups/perfect_flac/invited/bonus/bonus_per_hour/seeding_bonus/seeding_bonus_per_hour/rescue_seeding_bonus_per_hour/seeding_time/claim_count/max_claim/join_date/last_access_at
	DetailPath         string                  `yaml:"detail_path"`           // 富资料页（需 uid），默认 userdetails.php
	DetailFields       map[string]ElementQuery `yaml:"detail_fields"`         // 富资料页字段。约定键同 Fields（常用 level/join_date）
	Pages              []UserInfoPageDef       `yaml:"pages"`                 // 多步用户信息流程，按顺序抓取并合并字段
	UserTorrentListAPI string                  `yaml:"user_torrent_list_api"` // optional template with {id}, {type}, {page}
	VIPLevels          []string                `yaml:"vip_levels"`            // 视为 VIP 的等级名（好学 VIP 判定）
}

type UserInfoPageDef struct {
	Path      string                  `yaml:"path"`       // 相对路径；支持 {id}/{name}/$id$/$name$ 占位
	Params    map[string]string       `yaml:"params"`     // 查询参数，同样支持占位
	Headers   map[string]string       `yaml:"headers"`    // 额外请求头；常用于 AJAX 端点的 X-Requested-With
	Method    string                  `yaml:"method"`     // GET/POST；空为 GET
	Body      string                  `yaml:"body"`       // POST 请求体；常用于 JSON API 的 "{}"
	UseFields []string                `yaml:"use_fields"` // 复用 user_info.fields/detail_fields 中的字段名
	Fields    map[string]ElementQuery `yaml:"fields"`     // 本页专属字段
	Format    string                  `yaml:"format"`     // html/json/auto；空值按响应体自动判断
}

// NoLoginDef 描述「未登录」判定（多策略 OR）。
type NoLoginDef struct {
	HttpStatus []int    `yaml:"http_status"`                        // 这些状态码视为未登录（如 401/403）
	URLPattern []string `yaml:"url_pattern" binding:"dive,max=256"` // 跳转到含这些片段的 URL 视为未登录（login.php/takelogin）
	Selector   string   `yaml:"selector" binding:"max=512"`         // 页面存在该选择器视为未登录（登录表单）
	Content    []string `yaml:"content" binding:"dive,max=256"`     // 页面含这些文本视为未登录（"登录"/"login"）
}

// CategoryDef 描述一个站点分类。
type CategoryDef struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

// LoginDef 描述 NexusPHP 系账号登录端点（takelogin.php + 图形验证码）。
// 字段名/路径均按好学源码（include/takelogin.php、public/login.php、functions.php
// image_code/check_code）校准。
type LoginDef struct {
	Enabled     bool              `yaml:"enabled"`     // 是否启用账号登录（关闭则仅 cookie 登录）
	PagePath    string            `yaml:"page_path"`   // 登录页，默认 login.php
	ActionPath  string            `yaml:"action_path"` // POST 目标，默认 takelogin.php
	Form        LoginFormMapping  `yaml:"form"`
	Captcha     CaptchaDef        `yaml:"captcha"`
	ExtraFields map[string]string `yaml:"extra_fields"` // 固定额外表单字段（如 logout/login submit 值）
}

// LoginFormMapping 把逻辑字段名映射到站点实际 input name（默认值取自好学源码）。
type LoginFormMapping struct {
	Username    string `yaml:"username"`      // 默认 username
	Password    string `yaml:"password"`      // 默认 password
	ImageHash   string `yaml:"image_hash"`    // 默认 imagehash
	ImageString string `yaml:"imagestring"`   // 默认 imagestring
	TwoStepCode string `yaml:"two_step_code"` // 默认 two_step_code（2FA TOTP 6 位码）
}

// CaptchaDef 描述图形验证码抓取规则。
type CaptchaDef struct {
	GenPath           string `yaml:"gen_path"`            // 验证码图路径，默认 image.php（好学 image.php?action=regimage&imagehash=H&secret=S）
	HashInputSelector string `yaml:"hash_input_selector"` // 登录页 hidden imagehash 选择器，默认 input[name="imagehash"]
}
