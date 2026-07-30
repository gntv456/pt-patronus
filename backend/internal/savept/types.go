// Package savept 实现「PT监护室」：聚合第三方 PT 站点健康监控站 savept.icu 的公开数据，
// 展示各 PT 站的存续状态（健康/病危/已关站）、建站年份、站庆、抢救时长，并与用户本地已配置的
// 站点做匹配（已绑定/可添加/未收录）。独立于站点登录，纯公开页抓取 + 正则解析。
package savept

import "time"

// SiteHealthStatus 站点健康状态。
type SiteHealthStatus string

const (
	HealthHealthy  SiteHealthStatus = "healthy"  // 正常运营
	HealthCritical SiteHealthStatus = "critical" // 病危/抢救中
	HealthClosed   SiteHealthStatus = "closed"   // 已关站/死亡
	HealthUnknown  SiteHealthStatus = "unknown"
)

// SiteType 站点类型（内站/外站）。
type SiteType string

const (
	TypeInternal SiteType = "internal"
	TypeExternal SiteType = "external"
	TypeUnknown  SiteType = "unknown"
)

// AlertType 监护室公告类型。
type AlertType string

const (
	AlertDanger      AlertType = "danger"      // 病危通知（抢救中）
	AlertObituary    AlertType = "obituary"    // 死亡讣告（已关站）
	AlertWarning     AlertType = "warning"     // 恢复公告
	AlertInfo        AlertType = "info"        // 站庆预告
	AlertCelebration AlertType = "celebration" // 今日站庆
)

// MatchType 本地站点匹配类型（优先级从高到低）。
type MatchType string

const (
	MatchNone        MatchType = "none"         // 未收录
	MatchURLExact    MatchType = "url_exact"    // URL 完全匹配 → 已绑定
	MatchDomainExact MatchType = "domain_exact" // 域名匹配 → 已绑定
	MatchNameExact   MatchType = "name_exact"   // 名称完全匹配 → 可添加
	MatchNameFuzzy   MatchType = "name_fuzzy"   // 名称模糊匹配 → 可添加
)

// SaveptAlert 是一条监护室公告（病危/恢复/站庆/讣告）。
type SaveptAlert struct {
	Type    AlertType `json:"type"`
	Title   string    `json:"title"`
	Content string    `json:"content"`
}

// SaveptSiteCard 是一张监护室站点卡片。
type SaveptSiteCard struct {
	Name            string           `json:"name"`
	URL             string           `json:"url"`
	Domain          string           `json:"domain"`
	Icon            string           `json:"icon,omitempty"`
	Year            string           `json:"year"` // 建站年份（"未知"若无）
	Status          SiteHealthStatus `json:"status"`
	StatusText      string           `json:"status_text,omitempty"`
	SiteType        SiteType         `json:"site_type"`
	AnniversaryText string           `json:"anniversary_text,omitempty"` // 站庆提示
	AnniversaryFlag string           `json:"anniversary_flag,omitempty"` // 站庆标记（"yes"/"no"）
	DurationText    string           `json:"duration_text,omitempty"`    // 运营时长
	FaultDuration   string           `json:"fault_duration,omitempty"`   // 抢救时长（病危时填充）
	OpenTime        string           `json:"open_time,omitempty"`        // 开站时间
	CloseTime       string           `json:"close_time,omitempty"`       // 关站时间（已关站填充）
	Search          string           `json:"search,omitempty"`           // 站点搜索/注册入口
	// 本地匹配结果（与用户站点比对后填充）
	MatchType     MatchType `json:"match_type"`
	LocalSiteID   uint      `json:"local_site_id,omitempty"`
	LocalSiteName string    `json:"local_site_name,omitempty"`
}

// SaveptSummary 是监护室汇总统计。
type SaveptSummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Critical  int `json:"critical"`
	Closed    int `json:"closed"`
	Bound     int `json:"bound"`     // 已绑定（本地存在）
	Available int `json:"available"` // 可添加（名称匹配但本地未配）
	// 细分统计（喂前端 ICU 头部生命体征）
	Internal      int `json:"internal"`       // 内站
	External      int `json:"external"`       // 外站
	TodayAnniv    int `json:"today_anniv"`    // 今日站庆
	Years         int `json:"years"`          // 建站年份数（去重）
	MpOwned       int `json:"mp_owned"`       // = Bound
	MpAvailable   int `json:"mp_available"`   // = Available
	MpUnsupported int `json:"mp_unsupported"` // 未收录
}

// Result 是一次监护室抓取 + 匹配的完整结果。
type Result struct {
	Summary   SaveptSummary    `json:"summary"`
	Alerts    []SaveptAlert    `json:"alerts"`
	Cards     []SaveptSiteCard `json:"cards"`
	FetchedAt time.Time        `json:"fetched_at"`
	Cached    bool             `json:"cached"`
	Stale     bool             `json:"stale"`            // 降级：抓取失败时返回的缓存快照（可能已过期）
	Notice    string           `json:"notice,omitempty"` // 降级/提示文案（如 CF 拦截）
}
