// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/aiagent"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// tgBotStatusFn/tgBotReloadFn 由 app_runtime 注入（闭包捕获 tgCancel/startTgBot）。
type tgBotStatusFn func() map[string]any
type tgBotReloadFn func()

// registerTgBotTools 把 TG bot 面暴露给插件（#34 二梯队：ptp-tgbot-bridge 承接
// 配置/状态/健康检查面；bot 轮询本体留在宿主进程——它依赖 in-process agentSvc
// 流式会话，进程外重写风险高，能力面打样模式）。
func registerTgBotTools(
	reg *plugins.HostToolRegistry,
	agentSvc *aiagent.Agent,
	tgStatus tgBotStatusFn,
	tgReload tgBotReloadFn,
) {
	if agentSvc == nil {
		return
	}
	reg.Register(plugins.HostTool{
		Name: "agent_chat", Category: "agent", Permission: "agent:read",
		Description: "Send a text to the built-in AI agent (single-shot, " +
			"aggregated non-streaming reply). Requires AI configured.",
		InputSchema: objSchema(map[string]any{
			"text": strProp("user message"),
		}, []string{"text"}),
		Timeout: 120 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		text := strings.TrimSpace(argString(a, "text", ""))
		if text == "" {
			return nil, errors.New("text 必填")
		}
		events := agentSvc.Run(ctx, 0, text)
		var sb strings.Builder
		for ev := range events {
			switch ev.Type {
			case "text":
				sb.WriteString(ev.Text)
			case "error":
				return nil, errors.New(ev.Error)
			}
		}
		if strings.TrimSpace(sb.String()) == "" {
			return nil, errors.New("agent 未返回内容（检查 AI 配置）")
		}
		return map[string]any{"reply": sb.String()}, nil
	})
	if tgStatus != nil {
		reg.Register(plugins.HostTool{
			Name: "tgbot_status", Category: "notify", Permission: "notify:read",
			Description: "TG bot runtime status (enabled config / running / safe-mode).",
			InputSchema: noArgsSchema(),
		}, func(ctx context.Context, _ map[string]any) (any, error) {
			return tgStatus(), nil
		})
	}
	if tgReload != nil {
		reg.Register(plugins.HostTool{
			Name: "tgbot_reload", Category: "notify", Permission: "notify:write",
			Description: "Apply TG bot config changes (stop+restart " +
				"polling; no-op when disabled).",
			InputSchema: noArgsSchema(),
		}, func(ctx context.Context, _ map[string]any) (any, error) {
			tgReload()
			return map[string]any{"reloaded": true}, nil
		})
	}
}
