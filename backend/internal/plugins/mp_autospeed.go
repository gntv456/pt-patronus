// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
)

type autoSpeedPlugin struct{}

func NewAutoSpeedPlugin() Plugin { return autoSpeedPlugin{} }

func (autoSpeedPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "auto-speed",
		Name:        "Vue-网络测速",
		Version:     "1.1.0",
		Author:      "KoWming,鱼丸粗面 / PTPatronus",
		Description: "迁移自 MoviePilot：执行轻量下载/上传测速，记录结果到插件日志。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "action:execute", "notice:write"},
		Actions: []ActionSpec{
			{
				Name:        "run_speedtest",
				Label:       "开始测速",
				Description: "使用下载 URL 和可选上传 URL 进行轻量测速；默认读取有限字节，适合快速判断网络质量。",
				InputSchema: []ConfigField{
					{Key: "download_url", Label: "下载测试 URL", Type: "text", Help: "留空使用插件配置。"},
					{Key: "upload_url", Label: "上传测试 URL", Type: "text", Help: "留空使用插件配置；为空则跳过上传。"},
				},
			},
			{Name: "presets", Label: "测速预设", Description: "查看可直接复制使用的测速端点。"},
		},
		ConfigSchema: []ConfigField{
			{Key: "download_url", Label: "下载测试 URL", Type: "text", Group: "测速", Default: "https://speed.cloudflare.com/__down?bytes=25000000"},
			{Key: "upload_url", Label: "上传测试 URL", Type: "text", Group: "测速", Default: "https://speed.cloudflare.com/__up"},
			{Key: "download_bytes", Label: "下载字节上限", Type: "number", Group: "测速", Default: 10000000},
			{Key: "upload_bytes", Label: "上传字节数", Type: "number", Group: "测速", Default: 2000000},
			{Key: "timeout_seconds", Label: "超时时间(秒)", Type: "number", Group: "网络", Default: 20},
			{Key: "allow_private_network", Label: "允许内网地址", Type: "switch", Group: "网络", Default: false},
			{Key: "notify", Label: "测速完成后通知", Type: "switch", Group: "通知", Default: false},
			{Key: "cron_speedtest", Label: "定时测速cron", Type: "text", Group: "定时", Default: "0 */6 * * *", Help: "到点自动跑一次测速并记入日志；留空=不定时。默认每 6 小时。"},
		},
		Schedule: []ScheduleSpec{
			{ID: "speedtest", Label: "定时测速", Action: "run_speedtest", Cron: "0 */6 * * *", CronConfigKey: "cron_speedtest"},
		},
	}
}

func (autoSpeedPlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"download_url":          "https://speed.cloudflare.com/__down?bytes=25000000",
		"upload_url":            "https://speed.cloudflare.com/__up",
		"download_bytes":        10000000,
		"upload_bytes":          2000000,
		"timeout_seconds":       20,
		"allow_private_network": false,
		"notify":                false,
		"cron_speedtest":        "0 */6 * * *",
	}
}

func (autoSpeedPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (autoSpeedPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	cfg := rt.Config()
	switch action {
	case "run_speedtest":
		out, err := mpRunSpeedtest(ctx, cfg, input)
		if err != nil {
			return nil, err
		}
		if asBool(cfg["notify"]) {
			_ = rt.Notice(ctx, NoticeInput{Title: "网络测速完成", Body: mpSpeedtestText(out), Level: "success"})
		}
		rt.Log(logInfo, action, "network speedtest completed", out)
		return out, nil
	case "presets":
		return map[string]any{
			"download": []string{
				"https://speed.cloudflare.com/__down?bytes=25000000",
				"https://speed.cloudflare.com/__down?bytes=100000000",
			},
			"upload": []string{"https://speed.cloudflare.com/__up"},
		}, nil
	default:
		return nil, unknownActionErr(action)
	}
}
