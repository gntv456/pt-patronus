package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/llm"
	"github.com/hxpt/ptpatronus/internal/mcp"
)

// MessageStore 抽象 oper.AgentMessageOper（破 aiagent→oper 依赖）。
type MessageStore interface {
	ListByConv(convID uint) ([]models.AgentMessage, error)
	Append(m *models.AgentMessage) error
}

// ConversationStore 抽象 oper.AgentConversationOper。
type ConversationStore interface {
	SetTitle(id uint, title string) error
	Touch(id uint) error
}

// ProviderFactory 按 llm.Config 构造 provider（默认 llm.New，测试可注入 fake）。
type ProviderFactory func(llm.Config) llm.Provider

// Agent 是 ReAct 智能体。Run 返回事件流通道（text/reasoning/tool_call/tool_result/done/error）。
type Agent struct {
	newProvider ProviderFactory
	tools       *ToolRegistry
	msgs        MessageStore
	convs       ConversationStore
	kv          KVStore
	crypto      Crypto
}

// New 构造。tools 为全部可用工具（含 handler）。
func New(f ProviderFactory, tools []mcp.Tool, msgs MessageStore, convs ConversationStore, kv KVStore, crypto Crypto) *Agent {
	if f == nil {
		f = llm.New
	}
	return &Agent{newProvider: f, tools: NewToolRegistry(tools), msgs: msgs, convs: convs, kv: kv, crypto: crypto}
}

// Run 执行一轮对话：持久化用户消息 → 加载历史 → ReAct 循环（工具调用→回灌→再请求）。
// convID 必须已存在（由 API handler 先建空会话）。
func (a *Agent) Run(ctx context.Context, convID uint, userText string) <-chan llm.Event {
	out := make(chan llm.Event, 64)
	go func() {
		defer close(out)
		ac, lc := Load(a.kv, a.crypto)
		if !ac.Enabled {
			emit(out, llm.Event{Type: llm.EventError, Error: "AI 助手未启用，请先在「设置 → AI 助手」配置 LLM provider 与 API key"})
			return
		}
		if lc.APIKey == "" {
			emit(out, llm.Event{Type: llm.EventError, Error: "未配置 API key"})
			return
		}

		// 1. 持久化用户消息
		_ = a.msgs.Append(&models.AgentMessage{ConversationID: convID, Role: "user", Content: userText})
		_ = a.convs.Touch(convID)

		// 2. 加载历史（含刚追加的 user）→ 统一消息
		hist, err := a.msgs.ListByConv(convID)
		if err != nil {
			// 历史加载失败则不应继续：上下文缺失会让 LLM 只看到 system prompt 空答。
			emit(out, llm.Event{Type: llm.EventError, Error: "加载历史消息失败：" + err.Error()})
			return
		}
		firstTurn := len(hist) == 1
		if firstTurn {
			// 首条消息：先用截断兜底标题；首轮回合后 LLM 总结覆盖。
			_ = a.convs.SetTitle(convID, titleFrom(userText))
		}
		msgs := toLLMMessages(hist)

		// 3. 系统提示 + 裁剪
		msgs = append([]llm.Message{{Role: llm.RoleSystem, Content: ac.SystemPrompt}}, msgs...)
		msgs = trimMessages(msgs)
		toolSchemas := a.tools.Schemas(ac.EnabledTools)
		provider := a.newProvider(lc)

		// 4. ReAct 循环
		for iter := 0; iter < ac.MaxIterations; iter++ {
			// 客户端断开（ctx 取消）则立即停止：避免继续调用计费 LLM 与执行写类工具，
			// 并防止 out channel 因无人消费而阻塞悬挂 goroutine。
			select {
			case <-ctx.Done():
				emit(out, llm.Event{Type: llm.EventError, Error: "对话已取消"})
				return
			default:
			}
			stream, err := provider.ChatStream(ctx, llm.Req{
				Messages: msgs, Tools: toolSchemas,
				Model: lc.Model, Temperature: lc.Temperature,
				MaxTokens: lc.MaxTokens, EnableThinking: lc.EnableThinking,
			})
			if err != nil {
				emit(out, llm.Event{Type: llm.EventError, Error: "LLM 调用失败：" + err.Error()})
				return
			}
			content, reasoning, calls, sawErr := consumeStream(stream, out)
			if sawErr {
				return // 错误已在 consumeStream 转发
			}
			// 持久化本轮 assistant（含中间 tool-calling 轮）
			tcJSON, _ := json.Marshal(calls)
			_ = a.msgs.Append(&models.AgentMessage{ConversationID: convID, Role: "assistant", Content: content, Reasoning: reasoning, ToolCalls: string(tcJSON)})
			_ = a.convs.Touch(convID)

			if len(calls) == 0 {
				emit(out, llm.Event{Type: llm.EventDone})
				if firstTurn {
					a.summarizeTitle(ctx, provider, lc, convID, userText, content) // best-effort，不阻塞 done
				}
				return // 最终答案
			}
			// 回灌 assistant（带 tool_calls）给 provider
			msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: content, ToolCalls: calls})
			// 执行每个工具 → 回灌结果（UI/DB 存全量；LLM 上下文截断）
			for _, tc := range calls {
				args, perr := parseArgs(tc.Arguments)
				var full string
				if perr != nil {
					// 参数解析失败：回灌错误让模型重试，而非以零值参数执行副作用工具。
					full = "ERROR: " + perr.Error()
				} else {
					var execErr error
					full, execErr = a.tools.Exec(ctx, tc.Name, args)
					if execErr != nil {
						full = "ERROR: " + execErr.Error()
					}
				}
				emit(out, llm.Event{Type: llm.EventToolResult, ToolCall: tc, Text: full}) // UI 看全量
				_ = a.msgs.Append(&models.AgentMessage{ConversationID: convID, Role: "tool", ToolCallID: tc.ID, Name: tc.Name, Content: full}) // DB 全量
				msgs = append(msgs, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Name: tc.Name, Content: truncateResult(full)}) // LLM 截断
			}
			_ = a.convs.Touch(convID)
		}
		emit(out, llm.Event{Type: llm.EventError, Error: fmt.Sprintf("达到最大思考轮数(%d)，请缩小问题或重试", ac.MaxIterations)})
	}()
	return out
}

// consumeStream 消费 provider 事件流：转发 text/reasoning/tool_call/error 到 out，
// 累积本轮 assistant 的 content/reasoning/tool_calls。返回 sawErr 表示是否出现错误（终态）。
func consumeStream(stream <-chan llm.Event, out chan<- llm.Event) (content, reasoning string, calls []llm.ToolCall, sawErr bool) {
	for ev := range stream {
		switch ev.Type {
		case llm.EventText:
			content += ev.Text
			emit(out, ev)
		case llm.EventReasoning:
			reasoning += ev.Text
			emit(out, ev)
		case llm.EventToolCall:
			calls = append(calls, ev.ToolCall)
			emit(out, ev)
		case llm.EventError:
			sawErr = true
			emit(out, ev)
		case llm.EventDone:
			// 流正常结束
		}
	}
	return
}

// toLLMMessages 把持久化消息还原为 provider 消息（含中间 tool 轮，保证 provider 上下文完整）。
func toLLMMessages(ms []models.AgentMessage) []llm.Message {
	out := make([]llm.Message, 0, len(ms))
	for _, m := range ms {
		switch m.Role {
		case "user":
			out = append(out, llm.Message{Role: llm.RoleUser, Content: m.Content})
		case "assistant":
			var calls []llm.ToolCall
			if m.ToolCalls != "" {
				_ = json.Unmarshal([]byte(m.ToolCalls), &calls)
			}
			out = append(out, llm.Message{Role: llm.RoleAssistant, Content: m.Content, Reasoning: m.Reasoning, ToolCalls: calls})
		case "tool":
			// 重建给 provider 时截断（DB 存全量，但 LLM 上下文需受限）。
			out = append(out, llm.Message{Role: llm.RoleTool, ToolCallID: m.ToolCallID, Name: m.Name, Content: truncateResult(m.Content)})
		}
	}
	return out
}

// summarizeTitle 首轮回合后用 LLM 生成简短会话标题（≤12 字），best-effort 覆盖兜底标题。
// done 已发后调用，不阻塞前端；15s 超时；失败/空/过长则保留兜底。
func (a *Agent) summarizeTitle(ctx context.Context, provider llm.Provider, lc llm.Config, convID uint, userText, assistantText string) {
	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	prompt := "请用不超过12个汉字为下面这段对话生成一个标题，只输出标题文本，不要引号、不要标点、不要「」。\n\n用户：" + userText + "\n助手：" + truncate(assistantText, 400)
	stream, err := provider.ChatStream(tctx, llm.Req{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}},
		Model:    lc.Model, Temperature: 0.2, MaxTokens: 40, EnableThinking: false,
	})
	if err != nil {
		return
	}
	var title string
	for ev := range stream {
		if ev.Type == llm.EventText {
			title += ev.Text
		}
	}
	title = strings.Trim(strings.TrimSpace(title), "「」\"'`。., \n\r\t")
	if rs := []rune(title); len(rs) > 0 && len(rs) <= 40 {
		_ = a.convs.SetTitle(convID, title)
	}
}

// emit 非阻塞发送事件（消费方卡死 30s 后丢弃，避免 goroutine 泄漏）。
func emit(ch chan<- llm.Event, e llm.Event) {
	select {
	case ch <- e:
	case <-time.After(30 * time.Second):
	}
}

// titleFrom 取首条用户消息前 24 rune 作为会话标题。
func titleFrom(s string) string {
	r := []rune(s)
	if len(r) <= 24 {
		return s
	}
	return string(r[:24]) + "…"
}
