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
const maxToolResult = 2000

// toolExecTimeout 单工具执行超时。
const toolExecTimeout = 30 * time.Second

// ToolRegistry 持有全部 agent 工具（mcp.Tool 含 handler），提供 schema 投影与受控执行。
type ToolRegistry struct{ tools []mcp.Tool }

// NewToolRegistry 构造。
func NewToolRegistry(tools []mcp.Tool) *ToolRegistry { return &ToolRegistry{tools: tools} }

// Schemas 返回下发给 provider 的工具 schema（去 handler；按白名单过滤，空=全开）。
func (r *ToolRegistry) Schemas(whitelist []string) []llm.Tool {
	allowAll := len(whitelist) == 0
	set := make(map[string]bool, len(whitelist))
	for _, w := range whitelist {
		set[w] = true
	}
	out := make([]llm.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		if !allowAll && !set[t.Name] {
			continue
		}
		out = append(out, llm.Tool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return out
}

// Exec 执行命名工具：参数解析 + 超时。返回**完整**结果（不截断——UI/DB 看全量；
// 送入 LLM 上下文时由 agent 层按需截断，控制 token）。
// 工具不存在/执行错均回灌给模型自适应。
func (r *ToolRegistry) Exec(ctx context.Context, name string, args map[string]any) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err() // 客户端已断开：不再执行任何（含写类）工具，避免悬空副作用
	}
	for _, t := range r.tools {
		if t.Name != name {
			continue
		}
		tctx, cancel := context.WithTimeout(ctx, toolExecTimeout)
		defer cancel()
		return t.Handler(tctx, args)
	}
	return "", fmt.Errorf("tool %q not found", name)
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
