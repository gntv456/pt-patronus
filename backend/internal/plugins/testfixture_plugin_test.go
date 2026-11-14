// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import "context"

// feature 插件已全部迁开放插件市场（manager.go NewManager 注释），宿主包内不再有
// Go 实现的 feature builtin。管理器/配置/市场等测试需要「已注册插件」当夹具时，
// 用下面两个最小 stub 复现市场插件的 Manifest 形状（调度声明 / 事件声明 / cron 配置键）。

// schedStubPlugin 带一条 cron 调度 + cron_update 配置覆盖键（语义同市场版
// cloudflare-subscribe 的 daily_update），供调度注册与坏配置降级测试使用。
type schedStubPlugin struct{}

func (schedStubPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "sched-stub",
		Name:        "调度夹具",
		Version:     "1.0.0",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"log:write"},
		Actions: []ActionSpec{
			{Name: "update", Label: "执行", Description: "空动作。"},
		},
		ConfigSchema: []ConfigField{
			{Key: "cron_update", Label: "定时cron", Type: "text", Default: "0 6 * * *"},
		},
		Schedule: []ScheduleSpec{
			{ID: "daily_update", Label: "每日执行", Action: "update", Cron: "0 6 * * *", CronConfigKey: "cron_update"},
		},
	}
}

func (schedStubPlugin) DefaultConfig() map[string]any {
	return map[string]any{"cron_update": "0 6 * * *"}
}

func (schedStubPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (schedStubPlugin) ExecuteAction(_ context.Context, _ Runtime, action string, _ map[string]any) (map[string]any, error) {
	if action != "update" {
		return nil, unknownActionErr(action)
	}
	return map[string]any{"ok": true}, nil
}

// eventStubPlugin 声明 plugin.test 事件并把它写进插件日志（语义同市场版
// event-audit），供事件总线投递与坏配置事件出口测试使用。
type eventStubPlugin struct{}

func (eventStubPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "event-stub",
		Name:        "事件夹具",
		Version:     "1.0.0",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"event:read", "log:write"},
		Events: []EventSpec{
			{Type: "plugin.test", Description: "插件系统测试事件"},
		},
	}
}

func (eventStubPlugin) DefaultConfig() map[string]any { return map[string]any{} }

func (eventStubPlugin) HandleEvent(ctx context.Context, rt Runtime, e Event) error {
	rt.Log("info", e.Type, "event received", nil)
	return nil
}

func (eventStubPlugin) ExecuteAction(_ context.Context, _ Runtime, action string, _ map[string]any) (map[string]any, error) {
	return nil, unknownActionErr(action)
}
