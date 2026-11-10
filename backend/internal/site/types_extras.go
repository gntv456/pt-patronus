// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"strconv"
	"strings"
	"time"
)

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
