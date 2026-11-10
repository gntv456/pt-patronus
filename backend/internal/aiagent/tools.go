// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hxpt/ptpatronus/internal/llm"
	"github.com/hxpt/ptpatronus/internal/mcp"
)

// maxToolResult 工具结果送入上下文的最大字符数（rune 计），超出截断。
// 对齐中文 token 密度：中文 1 字≈1 token。列表类工具（sites.list 78 站精简后 ~6k、
// dashboard/seed 列表）在旧值 1200 下必然被拦腰截断——模型只见前几条就得出
// 「结果被截断」的结论（曾致站点重复检查答不出）。12000 在 32k 窗口下可容纳
// 一次全量列表 + 对话历史，仍防单条结果吞掉全部预算。
const maxToolResult = 12000

// toolExecTimeout 单工具执行超时。
const toolExecTimeout = 30 * time.Second

// ToolRegistry 持有全部 agent 工具（mcp.Tool 含 handler），提供 schema 投影与受控执行。
type ToolRegistry struct{ tools []mcp.Tool }

// NewToolRegistry 构造。
func NewToolRegistry(tools []mcp.Tool) *ToolRegistry { return &ToolRegistry{tools: tools} }

// ToolCatalogEntry 是工具目录条目（去 handler，含风险等级），供白名单 UI 分组展示。
type ToolCatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Risk        string `json:"risk"`      // read | write | destructive
	RiskNote    string `json:"risk_note"` // destructive 工具的危险说明
}

// Catalog 返回全部工具的目录（去 handler，含风险等级），供白名单 UI。
func (r *ToolRegistry) Catalog() []ToolCatalogEntry {
	out := make([]ToolCatalogEntry, 0, len(r.tools))
	for _, t := range r.tools {
		risk := t.Risk
		if risk == "" {
			risk = mcp.RiskWrite // 兜底（classifyToolRisks 理论已全标）
		}
		out = append(out, ToolCatalogEntry{Name: t.Name, Description: t.Description, Risk: risk, RiskNote: t.RiskNote})
	}
	return out
}

// Schemas 返回下发给 provider 的工具 schema（去 handler；按白名单过滤，空=全开）。
func (r *ToolRegistry) Schemas(whitelist []string) []llm.Tool {
	out := make([]llm.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		if !toolAllowed(t.Name, whitelist) {
			continue
		}
		out = append(out, llm.Tool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return out
}

// Exec 执行命名工具：参数解析 + 超时。返回**完整**结果（不截断——UI/DB 看全量；
// 送入 LLM 上下文时由 agent 层按需截断，控制 token）。
// 工具不存在/执行错均回灌给模型自适应。
func (r *ToolRegistry) Exec(ctx context.Context, name string, args map[string]any, whitelist []string) (string, error) {
	if !toolAllowed(name, whitelist) {
		return "", fmt.Errorf("tool %q is not enabled", name)
	}
	if ctx.Err() != nil {
		return "", ctx.Err() // 客户端已断开：不再执行任何（含写类）工具，避免悬空副作用
	}
	for _, t := range r.tools {
		if t.Name != name {
			continue
		}
		timeout := toolExecTimeout
		if t.Timeout > 0 {
			timeout = t.Timeout // 慢操作（library_scan/refresh/subscribe_check/transfer_redo/cloud_generate_strm/rss_check/workflow_run）可设 60s
		}
		tctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return t.Handler(tctx, args)
	}
	return "", fmt.Errorf("tool %q not found", name)
}

func toolAllowed(name string, whitelist []string) bool {
	if len(whitelist) == 0 {
		return true
	}
	for _, allowed := range whitelist {
		if allowed == name {
			return true
		}
	}
	return false
}

// destructiveMeta 返回命名工具是否标记为 destructive 及其危险说明。
// 说明优先用 RiskNote，为空回退 Description；未注册或非 destructive 返回 (false, "")。
// agent 执行层据此对 destructive 工具做二次确认闸门。
func (r *ToolRegistry) destructiveMeta(name string) (bool, string) {
	for _, t := range r.tools {
		if t.Name != name {
			continue
		}
		if t.Risk == mcp.RiskDestructive {
			note := t.RiskNote
			if note == "" {
				note = t.Description
			}
			return true, note
		}
		return false, ""
	}
	return false, ""
}

// truncateResult rune 感知截断超长工具结果（dashboard/seed 列表常见）。
func truncateResult(s string) string {
	return truncate(s, maxToolResult)
}

// truncate rune 感知截断到 n 字符，超出加省略号。
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n…(结果已截断，如需更多请缩小查询范围)"
}

// parseArgs 把工具调用的 arguments JSON 串解析为 map。空串返回空 map（合法，工具可用默认值）；
// 非空但损坏返回 error，由 agent 回灌给模型自适应重试——避免副作用工具以零值参数执行
// （如 downloader_id=0 导致误删/误订阅）。仅当 arguments 确为空对象时才落空 map。
func parseArgs(args string) (map[string]any, error) {
	if args == "" {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(args), &out); err != nil {
		return nil, fmt.Errorf("arguments 非法 JSON: %w", err)
	}
	return out, nil
}
