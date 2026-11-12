// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package groupchat 实现 14 站 PT 喊话(群聊区)自动化，
//
// 架构：站点 handler 注册表 + 复用站点系统基础设施(httpclient + cookie 解密)。
// 14/14 站点纯 HTTP(原版 Playwright 路径是死代码)。NexusPHP 基类提供默认 Send/FetchFeedback，
// 各特殊站按需覆写。本包自包含，不 import plugins 包(避免循环)；plugins 层提供 Runtime 适配器。
package groupchat

import (
	"context"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// SiteRef 站点只读视图(不含密钥)。
type SiteRef struct {
	ID       uint
	Name     string
	BaseURL  string
	Username string // 站点账号(feedback 匹配 @username 用，可空)
}

// Runtime 是 groupchat 引擎需要的运行时能力子集。
// plugins.Runtime 通过适配器满足本接口(方法签名一致或适配)。
type Runtime interface {
	Log(level, event, msg string, data map[string]any)
	Notice(ctx context.Context, title, body, level string)
	KVGet(key string, out any) error
	KVSet(key string, val any) error
	KVDelete(key string) error
	SiteList() []SiteRef
	SiteCookie(id uint) (string, error)
	// SetConfig 合并写回 ConfigJSON 的单个键并触发调度重排(动态 cron 用，如织梦 zm_cron)。
	SetConfig(key string, val any) error
}

// Ctx 是单次站点交互的上下文。
type Ctx struct {
	HC       *httpclient.Client
	Site     SiteRef
	Cookie   string
	Cfg      map[string]any // 插件配置子集(feedback_timeout/use_proxy 等)
	Username string         // 当前用户名(可空)
}

// Reward 表示一条喊话反馈奖励。
type Reward struct {
	Type     string // 上传量/下载量/魔力值/工分/VIP/彩虹ID/电力/象草/青蛙/火花/啤酒瓶
	Text     string // 原始文本(供通知展示)
	Amount   string // 数量(可选)
	Negative bool   // 负向(如"损失")
}

// Feedback 是 Send 后抓取的反馈。
type Feedback struct {
	Sent    bool
	Rewards []Reward
	Raw     string         // 原始反馈文本(调试/兜底)
	Extra   map[string]any // 站点特化(blessing_status 等)
}

// Handler 是站点喊话处理器契约。
// Send 发送喊话并返回即时反馈(响应里能解析的部分，如 LongPT 的 JSON、青蛙的 ul/li、
// NexusPHP 响应页里的 shoutbox)。需要二次抓取/轮询的站额外实现 Poller。
type Handler interface {
	Match(s SiteRef) bool
	Send(ctx context.Context, c Ctx, message string) (*Feedback, error)
}

// Poller 可选：发送后需二次抓取反馈的站(NexusPHP 系重拉找 @user 行、好学前一行系统提示、
// 藏宝阁/PTLGS 轮询)。引擎在 get_feedback 开启且 handler 实现 Poller 时调用。
type Poller interface {
	PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error)
}

// 可选能力接口 —— 有则引擎在合适时机调用，无则跳过。
type DailyBonusBuyer interface { // 青蛙：每日福利兑换
	BuyDailyBonus(ctx context.Context, c Ctx) (string, error)
}
type LotteryPlayer interface { // LongPT：每日抽奖
	PlayLottery(ctx context.Context, c Ctx) (string, error)
}
type BlessingMedal interface { // 13City：诸神赐福勋章
	EnsureBlessing(ctx context.Context, c Ctx) (string, error)
}
type MailTimeFetcher interface { // 织梦：抓最新电力赠送邮件时间(独立调度锚点)
	LatestMailTime(ctx context.Context, c Ctx) (time.Time, error)
}

const (
	logInfo    = "info"
	logWarning = "warning"
	logError   = "error"
)
