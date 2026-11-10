// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type cloudflareSubscribePlugin struct{}

func NewCloudflareSubscribePlugin() Plugin { return cloudflareSubscribePlugin{} }

func (cloudflareSubscribePlugin) Manifest() Manifest {
	return Manifest{
		ID:          "cloudflare-subscribe",
		Name:        "Cloudflare订阅",
		Version:     "1.0.5",
		Author:      "KoWming / PTPatronus",
		Description: "迁移自 MoviePilot：拉取 Cloudflare hosts 订阅，清洗校验后生成可合并的 hosts 内容。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "action:execute", "notice:write"},
		Actions: []ActionSpec{
			{
				Name:        "update_hosts",
				Label:       "拉取订阅",
				Description: "按“名称|URL”配置拉取 hosts 订阅，过滤 HTML 和无效记录，输出合并后的 hosts。",
				InputSchema: []ConfigField{
					{Key: "subscriptions", Label: "订阅地址", Type: "textarea", Help: "每行：订阅名称|https://example.com/hosts；留空使用插件配置。"},
					{Key: "existing_hosts", Label: "已有 hosts", Type: "textarea", Help: "可粘贴当前 hosts，插件会移除旧订阅块并追加新订阅块。"},
				},
			},
			{
				Name:        "validate_hosts",
				Label:       "校验 hosts",
				Description: "校验一段 hosts 文本，返回有效记录和无效行。",
				InputSchema: []ConfigField{
					{Key: "hosts", Label: "hosts 内容", Type: "textarea", Required: true},
				},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "subscriptions", Label: "订阅地址", Type: "textarea", Group: "订阅", Help: "每行：订阅名称|订阅地址。"},
			{Key: "timeout_seconds", Label: "超时时间(秒)", Type: "number", Group: "网络", Default: 30},
			{Key: "max_retries", Label: "失败重试次数", Type: "number", Group: "网络", Default: 2},
			{Key: "allow_private_network", Label: "允许内网订阅源", Type: "switch", Group: "网络", Default: false},
			{Key: "notify", Label: "完成后通知", Type: "switch", Group: "通知", Default: false},
			{Key: "cron_update", Label: "定时拉取cron", Type: "text", Group: "定时", Default: "0 6 * * *", Help: "到点自动拉取订阅更新 hosts；留空=不定时。默认每天 06:00。"},
		},
		Schedule: []ScheduleSpec{
			{ID: "daily_update", Label: "每日拉取订阅", Action: "update_hosts", Cron: "0 6 * * *", CronConfigKey: "cron_update"},
		},
	}
}

func (cloudflareSubscribePlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"subscriptions":         "",
		"timeout_seconds":       30,
		"max_retries":           2,
		"notify":                false,
		"allow_private_network": false,
		"cron_update":           "0 6 * * *",
	}
}

func (cloudflareSubscribePlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (cloudflareSubscribePlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "update_hosts":
		cfg := rt.Config()
		rawSubs := strings.TrimSpace(pluginString(input["subscriptions"]))
		if rawSubs == "" {
			rawSubs = pluginString(cfg["subscriptions"])
		}
		subs, err := mpParseSubscriptions(rawSubs)
		if err != nil {
			return nil, err
		}
		timeout := time.Duration(pluginInt(cfg["timeout_seconds"], 30)) * time.Second
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		retries := pluginInt(cfg["max_retries"], 2)
		if retries < 0 {
			retries = 0
		}
		allowPrivate := asBool(cfg["allow_private_network"])
		existing := pluginString(input["existing_hosts"])
		finalHosts, results := mpFetchHostsSubscriptions(ctx, subs, timeout, retries, allowPrivate)
		merged := mpMergeSubscriptionHosts(existing, finalHosts)
		total := 0
		failures := 0
		for _, r := range results {
			total += pluginInt(r["records"], 0)
			if r["ok"] != true {
				failures++
			}
		}
		if asBool(cfg["notify"]) {
			level := "success"
			if failures > 0 {
				level = "warning"
			}
			_ = rt.Notice(ctx, NoticeInput{
				Title: "Cloudflare订阅更新完成",
				Body:  fmt.Sprintf("有效记录：%d，失败订阅：%d", total, failures),
				Level: level,
			})
		}
		rt.Log(logInfo, action, "cloudflare hosts subscription updated", map[string]any{"records": total, "failures": failures})
		return map[string]any{
			"hosts":         merged,
			"records":       total,
			"failures":      failures,
			"subscriptions": results,
		}, nil
	case "validate_hosts":
		records, invalid := mpParseHosts(pluginString(input["hosts"]))
		return map[string]any{"records": len(records), "valid": records, "invalid": invalid}, nil
	default:
		return nil, unknownActionErr(action)
	}
}
