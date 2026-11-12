// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package forumsignin 实现论坛/PT 站论坛区的每日签到自动化（）。
//
// 架构同 internal/plugins/groupchat：站点 handler 注册表 + 复用 httpclient。
// 与 groupchat 的区别：这些论坛（NodeSeek/SSDForum 等非 PT 论坛，及 PT 站的论坛区）
// 不一定在宿主 PT 站点列表中，故凭据（cookie/base）由插件配置承载（每行 key|base|cookie[|path]），
// 而非复用 SiteList/SiteCookie。签到端点基于社区逆向契约，httptest mock 覆盖，真实联调留用户。
//
// 本包自包含，不 import plugins 包（避免循环）；plugins 层提供 Runtime 适配器。
package forumsignin

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// Runtime 是签到引擎需要的运行时能力子集（plugins.Runtime 通过适配器满足）。
type Runtime interface {
	Log(level, event, msg string, data map[string]any)
	Notice(ctx context.Context, title, body, level string)
	KVGet(key string, out any) error
	KVSet(key string, val any) error
	KVDelete(key string) error
}

// Ctx 是单次论坛签到的上下文。
type Ctx struct {
	HC      *httpclient.Client
	Key     string // 站点标识（handler.Key()）
	BaseURL string // 论坛根地址（用户配置，如 https://www.nodeseek.com）
	Cookie  string // 论坛 cookie（用户登录后复制）
	Path    string // 签到路径覆盖（空→handler 默认）
	Cfg     map[string]any
}

// Feedback 是签到结果。
type Feedback struct {
	Success bool   // 是否签到成功（含「今日已签到」）
	Message string // 结果消息（如「签到成功，获得 +N 魔力」）
	Reward  string // 奖励摘要（可选）
	Raw     string // 原始响应（调试/兜底）
}

// Handler 是论坛签到处理器契约。
type Handler interface {
	Key() string   // 配置行第一列匹配键（如 "nodeseek"）
	Label() string // 展示名（如 "NodeSeek"）
	Signin(ctx context.Context, c Ctx) (*Feedback, error)
}

const (
	logInfo    = "info"
	logWarning = "warning"
	logError   = "error"
)
