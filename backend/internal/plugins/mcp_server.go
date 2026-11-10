// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"strings"

	"github.com/hxpt/ptpatronus/internal/mcp"
)

// mcp-server：MCP（Model Context Protocol）服务端，把宿主能力（站点/订阅/下载器/搜索…）
// 暴露为 MCP 工具，供 AI Agent / IDE / 自动化客户端经 JSON-RPC 调用。
// 对标 MoviePilot MCP Server（cyt-666）。
//
// 协议传输由 internal/mcp 包 + 公开路由 /api/v1/mcp 承载；本插件负责配置 UI（启用/令牌）：
// 服务端在每次请求时经 SettingsGetter 读取本插件配置，单一真源、热改即时生效。

// MCPPluginID 是 MCP 服务端插件的稳定 id（app.go 的 SettingsGetter 与路由据此读配置）。
const MCPPluginID = "ptp-mcp-server"

// ParseMCPSettings 从插件 config map 解析 mcp.Settings（供 app.go 的 SettingsGetter 调用）。
func ParseMCPSettings(cfg map[string]any) mcp.Settings {
	if cfg == nil {
		return mcp.Settings{}
	}
	return mcp.Settings{
		Enabled: boolVal(cfg["enabled"]),
		Token:   strings.TrimSpace(pluginString(cfg["token"])),
	}
}

// mcpEndpointURL 由对外基地址生成客户端接入 URL。base_url 的说明里给的示例是
// 「完整地址 https://host:8088/api/v1/mcp」，但用户也常只填站点根/反代前缀；
// 两种都要得到同一个可用 URL（从前一律再拼一遍 /api/v1/mcp，得到
// /api/v1/mcp/api/v1/mcp——界面照着填就连不上）。
func mcpEndpointURL(raw string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		return "/api/v1/mcp"
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/api/v1/mcp") {
		return base
	}
	return base + "/api/v1/mcp"
}

type mcpServerPlugin struct {
	server *mcp.Server // 可空（仅 app.go 装配时注入，供 test/list action 用）
}

// NewMCPServerPlugin 构造。server 可为 nil（manager 注册时未注入；action 降级）。
func NewMCPServerPlugin(server *mcp.Server) Plugin { return &mcpServerPlugin{server: server} }

func (p *mcpServerPlugin) Manifest() Manifest {
	return Manifest{
		ID:          MCPPluginID,
		Name:        "MCP 服务端",
		Version:     "1.0.0",
		APIVersion:  "1",
		AppVersion:  ">=0.1.0",
		Author:      "PTPatronus Community",
		Description: "【MCP 协议】开启后宿主在 /api/v1/mcp 暴露 JSON-RPC MCP 服务端，把站点/订阅/下载器/搜索等能力暴露为 MCP 工具，供 AI Agent / IDE（Claude Desktop、Cursor 等）或自动化客户端调用。建议设置鉴权令牌。",
		License:     "MIT",
		Keywords:    []string{"mcp", "agent", "ai", "json-rpc", "tools"},
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"log:write"},
		Actions: []ActionSpec{
			{Name: "tools", Label: "查看工具清单", Description: "返回当前暴露的 MCP 工具列表。"},
			{Name: "endpoint", Label: "查看接入地址", Description: "返回 MCP 服务端接入地址与鉴权方式说明。"},
		},
		ConfigSchema: []ConfigField{
			{Key: "enabled", Label: "启用 MCP 服务端", Type: "switch", Group: "服务", Default: false, Help: "开启后 /api/v1/mcp 对外提供 MCP JSON-RPC。"},
			{Key: "token", Label: "鉴权令牌", Type: "text", Group: "服务", Help: "Bearer Token；客户端用 Authorization: Bearer <token> 调用。必填：留空时服务拒绝匿名调用（401）。"},
			{Key: "base_url", Label: "对外基地址", Type: "text", Group: "服务", Help: "用于生成客户端接入 URL（如 https://host:8088/api/v1/mcp）。不影响实际监听。"},
		},
		Contributes: &ContributionSpec{
			Commands: []CommandContribution{{ID: "ptp-mcp-server.tools", Label: "MCP：查看工具清单", Action: "tools"}},
		},
		ResourceAccess: &ResourceAccessSpec{
			NetHosts: []string{"对外的 MCP 客户端（AI Agent / IDE）经基地址连入宿主"},
		},
	}
}

func (p *mcpServerPlugin) DefaultConfig() map[string]any {
	return map[string]any{"enabled": false, "token": "", "base_url": ""}
}

func (p *mcpServerPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (p *mcpServerPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "tools":
		if p.server == nil {
			return map[string]any{"tools": []any{}}, nil
		}
		list := p.server.ToolList()
		out := make([]map[string]any, 0, len(list))
		for _, t := range list {
			out = append(out, map[string]any{"name": t.Name, "description": t.Description})
		}
		return map[string]any{"tools": out}, nil
	case "endpoint":
		return map[string]any{
			"endpoint":  mcpEndpointURL(pluginString(rt.Config()["base_url"])),
			"transport": "streamable-http (POST JSON-RPC 2.0)",
			"auth":      "Authorization: Bearer <token>",
			"tools":     len(p.server.ToolList()),
		}, nil
	default:
		return nil, unknownActionErr(action)
	}
}
