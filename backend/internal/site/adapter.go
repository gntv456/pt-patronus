// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"context"
	"errors"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// Adapter 是一个平台 Schema 的抓取实现（如 NexusPHP / MTeam）。
// 一个 Adapter 实例服务该 schema 下所有站点定义；具体站点差异由 def（YAML）驱动。
type Adapter interface {
	// Schema 返回该适配器处理的平台标识（nexusphp / mteam / ...）。
	Schema() string

	// Search 在站点上搜索关键字，返回种子列表。
	// category 为媒体类型过滤（""=综合 不过滤；movie/tv/music 按各 schema 语义过滤：
	// MTeam 映射 mode，NexusPHP 发 cat[] + 后置过滤）。站点不支持该类型时退化为综合。
	Search(ctx context.Context, def *SiteMetadata, baseURL, cookie, keyword string, category string, page int) ([]Torrent, error)

	// FetchUserInfo 抓取并解析站点用户资料页。
	FetchUserInfo(ctx context.Context, def *SiteMetadata, baseURL, cookie string) (*UserInfo, error)
}

// CheckinResult 是签到结果。
type CheckinResult struct {
	Success bool    `json:"success"`
	Message string  `json:"message"`
	Bonus   float64 `json:"bonus"`
}

// CheckinCapable 是可选签到能力。适配器按需实现（type-assert 判断），不膨胀主 Adapter 接口。
type CheckinCapable interface {
	Checkin(ctx context.Context, def *SiteMetadata, baseURL, cookie string) (*CheckinResult, error)
}

// KeepAliveResult 是 cookie 保活探测结果。
//   - LoggedIn：当前 cookie 是否仍有效（失败需告警重新登录）
//   - RefreshedCookie：响应 Set-Cookie 轮换后的最新 cookie 串；为空表示未变化，无需回写
type KeepAliveResult struct {
	LoggedIn        bool
	RefreshedCookie string
}

// KeepAliveCapable 是可选 cookie 保活能力：做一次认证 GET 刷新服务端 session，
// 并返回轮换后的 cookie（若有）。后端版 WebViewCookiePollService。
type KeepAliveCapable interface {
	KeepAlive(ctx context.Context, def *SiteMetadata, baseURL, cookie string) (*KeepAliveResult, error)
}

// HRCapable 是可选 H&R（Hit & Run）列表抓取能力。适配器按需实现。
type HRCapable interface {
	FetchHR(ctx context.Context, def *SiteMetadata, baseURL, cookie string) ([]HRTorrent, error)
}

// SeedingListCapable 是可选「逐条做种列表」抓取能力。保种契约官种维度需逐条 title/labels 聚合，
// 故在 UserInfo 的整站汇总之外另开此能力。适配器按需实现（type-assert 判断）。
type SeedingListCapable interface {
	FetchSeedingList(ctx context.Context, def *SiteMetadata, baseURL, cookie string) ([]SeedingItem, error)
}

// FreeleechCapable 是可选「全站免费/促销」检测能力。抓首页横幅，判定 Active/Upcoming。
// 返回该站点的横幅列表（可能多条）；无促销时返回单条 NONE，抓取失败返回单条 FAILED。
type FreeleechCapable interface {
	FetchFreeleech(ctx context.Context, def *SiteMetadata, baseURL, cookie string) ([]SiteFreeleech, error)
}

// AnnouncementCapable 是可选站点公告抓取能力。返回首页公告区结构化条目。
type AnnouncementCapable interface {
	FetchAnnouncement(ctx context.Context, def *SiteMetadata, baseURL, cookie string) (*SiteAnnouncement, error)
}

// IndexInfo 是一次首页抓取同时得到的站免 + 公告——省一次 HTTP：原 FetchFreeleech 与
// FetchAnnouncement 各 GET 一次 index.php（URL 相同，仅解析逻辑不同）。
type IndexInfo struct {
	Freeleech    []SiteFreeleech
	Announcement *SiteAnnouncement
}

// IndexInfoCapable 是可选「首页聚合」能力：一次 GET 首页同时解析站免横幅 + 公告区。
// chain 仅在 Freeleech.Path 与 Announcement.Path 一致（默认 index.php）时用此能力合并请求；
// 适配器未实现、或两 path 不同时回退分别调用 FetchFreeleech + FetchAnnouncement。
// 仅首页系 schema（NexusPHP）实现；JSON API 系（MTeam）不实现。
type IndexInfoCapable interface {
	FetchIndexInfo(ctx context.Context, def *SiteMetadata, baseURL, cookie string) (*IndexInfo, error)
}

// HaremCapable 是可选「后宫」（邀请库存 + 被邀请人花名册）抓取能力。
type HaremCapable interface {
	FetchInviteStatus(ctx context.Context, def *SiteMetadata, baseURL, cookie string) (*InviteStatus, error)
	FetchInvitees(ctx context.Context, def *SiteMetadata, baseURL, cookie string) ([]InviteeInfo, error)
}

// MessageCapable 是可选站点私信能力（含写操作：回复/删除/标记已读）。
// CSRF 可选：def.Messages.CSRFField 为空时直接 POST（修正原版默认 reply 必失败的 bug）。
type MessageCapable interface {
	FetchMessages(ctx context.Context, def *SiteMetadata, baseURL, cookie string, mailbox MailboxType) ([]Message, error)
	FetchMessageContent(ctx context.Context, def *SiteMetadata, baseURL, cookie, msgURL string) (string, error)
	ReplyMessage(ctx context.Context, def *SiteMetadata, baseURL, cookie, msgURL, content string) (*MessageReplyResult, error)
	DeleteMessage(ctx context.Context, def *SiteMetadata, baseURL, cookie, deleteURL string) error
	MarkRead(ctx context.Context, def *SiteMetadata, baseURL, cookie string, itemIDs []string) (*BatchResult, error)
}

// TorrentDetailCapable 是可选「种子详情页」抓取能力。给定 details.php?id=... 页面 URL，
// 抓取并解析简介/截图/MediaInfo/外链 ID 等。NexusPHP（HTML）与 MTeam（JSON）实现。
// detailURL 由 chain 校验与站点 baseURL 同主机后透传（防用 A 站 cookie 探测 B 站）。
type TorrentDetailCapable interface {
	FetchTorrentDetail(ctx context.Context, def *SiteMetadata, baseURL, cookie, detailURL string) (*TorrentDetail, error)
}

// LoginCredentials 是账号登录凭据。
type LoginCredentials struct {
	Username string
	Password string
}

// CaptchaChallenge 是 FetchLoginPage（Step A）的结果：探测登录页得到验证码与模式。
//
// 好学的图形验证码走 DB（regimages 表）校验、不绑 session（见好学源码 functions.php
// check_code：SELECT WHERE imagehash=? AND imagestring=?），故 Step A 取到的 imagehash
// 可跨请求用于 Step B，无需共享 session cookie。
type CaptchaChallenge struct {
	ImageHash     string            // 登录页 hidden input 的 imagehash；无验证码时为空
	ImageB64      string            // 验证码图 base64（data:image/png;base64,...）；无验证码时为空
	ExtraForm     map[string]string // 登录页其它须回带的 hidden 字段（如 secret/returnto）
	NeedCaptcha   bool              // 本次是否需要图形验证码（页面探测，对应好学 $iv=='yes'）
	ChallengeMode bool              // 探测到挑战响应模式（无 password 字段名）→ 账号登录不可用，引导 cookie 登录
}

// LoginSession 是 PostLogin（Step B）成功的结果。
type LoginSession struct {
	Cookie   string    // 登录后从 302 Set-Cookie 解析出的全部 cookie 串
	UserInfo *UserInfo // 顺带抓取的用户资料（含 Level，供 VIP 判定）
}

// LoginHTTPDoer 是登录两步流使用的 HTTP 子接口（httpclient.Client 已实现）。
// 抽出最小接口便于注入 fake 测试；每条登录用独立 client（独立 jar）隔离并发。
type LoginHTTPDoer interface {
	Get(ctx context.Context, rawURL, cookieStr string, ssrf bool) (*httpclient.Response, error)
	PostForm(ctx context.Context, rawURL, cookieStr, formBody string, ssrf bool) (*httpclient.Response, error)
}

// LoginCapable 是可选「账号密码登录」能力。NexusPHP 实现；type-assert 判断。
// 好学登录分两步以应对图形验证码（后端无法自动破解）：
//
//	Step A: FetchLoginPage  GET 登录页，探测是否需验证码 / 是否挑战响应模式，取 imagehash + 验证码图
//	Step B: PostLogin       POST takelogin.php（带 username/password/imagehash/imagestring/two_step_code）
//
// hc 由调用方注入（同一条登录的 A/B 用同一 hc）；DB 校验的 imagehash 无需 session 续接。
type LoginCapable interface {
	FetchLoginPage(ctx context.Context, hc LoginHTTPDoer, def *SiteMetadata, baseURL string) (*CaptchaChallenge, error)
	PostLogin(ctx context.Context, hc LoginHTTPDoer, def *SiteMetadata, baseURL string,
		creds LoginCredentials, ch *CaptchaChallenge, imagestring, twoStepCode string) (*LoginSession, error)
}

// 账号登录相关错误。
var (
	// ErrNotSupported 适配器不支持该操作（签到/消息等）；chain 侧以 errors.Is 识别并跳过落库。
	ErrNotSupported           = errors.New("operation not supported by adapter")
	ErrLoginNotSupported      = errors.New("site: account login not supported")
	ErrLoginFailed            = errors.New("site: login failed (bad credentials/captcha/2fa)")
	ErrChallengeMode          = errors.New("site: challenge-response auth mode, use cookie login instead")
	ErrSeedingListUnsupported = errors.New("site: seeding list per-row parse unsupported")
)
