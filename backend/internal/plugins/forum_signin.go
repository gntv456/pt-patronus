// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/plugins/forumsignin"
)

// forum-signin：论坛/PT 站论坛区每日签到（NodeSeek/蜂巢/飞牛/柠檬/国语视界/SSDForum）。
// 凭据由插件配置承载（每行 key|base|cookie[|path]），不走 PT 站点列表。
// 对标 group-chat-zone：thin plugin 入口 + forumsignin 子包自包含（避循环 import）。

// fsRTAdapter 把 plugins.Runtime 适配成 forumsignin.Runtime（同 gcRTAdapter 模式）。
type fsRTAdapter struct{ rt Runtime }

func (a fsRTAdapter) Log(level, event, msg string, data map[string]any) {
	a.rt.Log(level, event, msg, data)
}
func (a fsRTAdapter) Notice(ctx context.Context, title, body, level string) {
	_ = a.rt.Notice(ctx, NoticeInput{Title: title, Body: body, Level: level})
}
func (a fsRTAdapter) KVGet(key string, out any) error { return a.rt.KVGet(key, out) }
func (a fsRTAdapter) KVSet(key string, val any) error { return a.rt.KVSet(key, val) }
func (a fsRTAdapter) KVDelete(key string) error       { return a.rt.KVDelete(key) }

type forumSigninPlugin struct {
	hc *httpclient.Client
}

// NewForumSigninPlugin 构造。
func NewForumSigninPlugin() Plugin { return &forumSigninPlugin{} }

func (p *forumSigninPlugin) client() (*httpclient.Client, error) {
	if p.hc != nil {
		return p.hc, nil
	}
	c, err := httpclient.New(httpclient.Options{FollowRedirect: true, Timeout: 20 * time.Second})
	if err != nil {
		return nil, err
	}
	p.hc = c
	return c, nil
}

func forumKeysHelp() string {
	parts := make([]string, 0, len(forumsignin.Supported()))
	for _, s := range forumsignin.Supported() {
		parts = append(parts, s.Key+"（"+s.Label+"）")
	}
	return "可选 key：" + strings.Join(parts, "、") + "。"
}

func (p *forumSigninPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "ptp-forum-signin",
		Name:        "论坛签到",
		Version:     "1.0.0",
		APIVersion:  "1",
		AppVersion:  ">=0.1.0",
		Author:      "PTPatronus Community",
		Description: "【论坛每日签到】支持 NodeSeek / 蜂巢 / 飞牛 / 柠檬 / 国语视界 / SSDForum 等论坛区签到。凭据在下方「论坛列表」按 key|base|cookie[|path] 每行一个配置，定时或手动触发。",
		License:     "MIT",
		Keywords:    []string{"forum", "signin", "attendance", "nodeseek", "checkin"},
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "log:write", "notice:write", "kv:read", "kv:write"},
		Actions: []ActionSpec{
			{Name: "run", Label: "立即签到", Description: "对所有已配置论坛立即执行一次签到，返回每站结果。"},
			{Name: "labels", Label: "支持站点列表", Description: "返回当前支持的全部论坛 key 与名称。"},
		},
		ConfigSchema: []ConfigField{
			{Key: "forums", Label: "论坛列表", Type: "textarea", Group: "站点", Required: true, Help: "每行一个论坛：key|base|cookie[|path]。cookie 为整段登录 cookie（可含除换行外任意字符）。" + forumKeysHelp()},
			{Key: "notify", Label: "签到结果通知", Type: "switch", Group: "通用", Default: true},
			{Key: "retry_count", Label: "失败重试次数", Type: "number", Group: "通用", Default: 1},
			{Key: "retry_interval", Label: "重试间隔(分钟)", Type: "number", Group: "通用", Default: 10},
			{Key: "cron", Label: "定时表达式", Type: "text", Group: "调度", Default: "0 8 * * *", Help: "默认每天 8 点自动签到。留空则不自动，可手动触发。"},
		},
		Schedule: []ScheduleSpec{
			{ID: "signin", Label: "每日论坛签到", Action: "run", Cron: "0 8 * * *", CronConfigKey: "cron"},
		},
		Contributes: &ContributionSpec{
			Commands: []CommandContribution{{ID: "ptp-forum-signin.run", Label: "论坛签到：立即签到", Action: "run"}},
			Menus:    []MenuContribution{{Location: "plugins/actions", Command: "ptp-forum-signin.run"}},
		},
		ResourceAccess: &ResourceAccessSpec{
			NetHosts: []string{"用户配置的各论坛域名（如 nodeseek.com 及各 PT 站论坛区）"},
		},
	}
}

func (p *forumSigninPlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"forums": "", "notify": true, "retry_count": 1, "retry_interval": 10, "cron": "0 8 * * *",
	}
}

func (p *forumSigninPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (p *forumSigninPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "labels":
		return map[string]any{"sites": forumsignin.Supported()}, nil
	case "run", "run_now":
		hc, err := p.client()
		if err != nil {
			return nil, err
		}
		results := forumsignin.NewEngine(fsRTAdapter{rt: rt}, hc, rt.Config()).Run(ctx)
		if results == nil {
			results = []forumsignin.SiteResult{}
		}
		return map[string]any{"ok": true, "results": results}, nil
	default:
		return nil, unknownActionErr(action)
	}
}
