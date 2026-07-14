// Package llm 实现 LLM provider 抽象：统一 OpenAI 兼容与 Anthropic Claude 的
// 流式 chat + tool-calling + 推理(reasoning_content / thinking)为一致的事件流。
// 仅依赖标准库（net/http 直读 SSE），不引入外部 SDK。
package llm

import "context"

// Role 消息角色（跨 provider 统一）。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall 是一次工具调用（assistant 发起）。
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // 原始 JSON 串（provider 增量拼接）
}

// Message 是跨 provider 统一的对话消息。
type Message struct {
	Role      Role       `json:"role"`
	Content   string     `json:"content,omitempty"`
	Reasoning string     `json:"reasoning,omitempty"`         // 思考过程（持久化/UI 用；不下发 provider）
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`         // role=assistant：发起的工具调用
	ToolCallID string    `json:"tool_call_id,omitempty"`       // role=tool：回应哪个调用
	Name      string     `json:"name,omitempty"`               // role=tool：工具名
}

// Tool 是下发给 provider 的工具 schema（不含 handler——handler 由 agent 层持）。
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"` // JSON Schema
}

// Req 是一次 chat 请求。
type Req struct {
	Messages       []Message
	Tools          []Tool
	Model          string
	Temperature    float64
	MaxTokens      int
	EnableThinking bool
}

// EventType 事件类型。
type EventType string

const (
	EventText          EventType = "text"           // 文本增量
	EventReasoning     EventType = "reasoning"     // 思考增量（深度思考）
	EventToolCall      EventType = "tool_call"     // 一次完整的工具调用（流末发）
	EventToolResult    EventType = "tool_result"    // 工具执行结果（agent 层产出）
	EventConfirmRequest EventType = "confirm_request" // 危险工具执行前请求用户确认（agent 层产出）
	EventUsage         EventType = "usage"         // 本轮 token 用量（provider 流末产出，供统计）
	EventDone          EventType = "done"          // 本轮 provider 响应结束
	EventError         EventType = "error"         // 错误（流内）
)

// ConfirmRequest 是危险工具执行前的确认请求（携带令牌与风险摘要）。
// 工具名/参数/id 复用所在 Event 的 ToolCall 字段。Token 供 /confirm 端点回送决断。
type ConfirmRequest struct {
	Token   string `json:"token"`
	Risk    string `json:"risk"`
	Summary string `json:"summary"` // = 工具 RiskNote，空则回退 Description
}

// Usage 是单次 ChatStream 调用的 token 用量（provider 流末产出）。
// input_tokens 含完整上下文（每轮重发也会计费，求和即账单口径）。
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Event 是 provider 产出的事件。
type Event struct {
	Type     EventType        `json:"type"`
	Text     string           `json:"text,omitempty"`      // text/reasoning
	ToolCall ToolCall         `json:"tool_call,omitempty"`  // tool_call
	Confirm  *ConfirmRequest  `json:"confirm,omitempty"`    // confirm_request
	Usage    *Usage           `json:"usage,omitempty"`      // usage
	Error    string           `json:"error,omitempty"`      // error
}

// Config 是 provider 连接配置（APIKey 已解密）。
type Config struct {
	Provider       string  // "openai" | "anthropic"
	BaseURL        string
	APIKey         string
	Model          string
	Temperature    float64
	MaxTokens      int
	EnableThinking bool
}

// Provider 是一个 LLM provider。ChatStream 返回事件流通道；
// 立即错误（如 URL 非法）走 err，流内错误走 EventError。
type Provider interface {
	ChatStream(ctx context.Context, req Req) (<-chan Event, error)
}

// New 按 config 返回 provider 实现。未知 provider 默认 OpenAI 兼容。
func New(cfg Config) Provider {
	switch cfg.Provider {
	case "anthropic":
		return &anthropicProvider{cfg: cfg}
	default:
		return &openaiProvider{cfg: cfg}
	}
}
