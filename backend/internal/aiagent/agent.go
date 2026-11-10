// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"github.com/hxpt/ptpatronus/internal/core/safego"
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

// Agent 是 ReAct 智能体。Run 返回事件流通道（text/reasoning/tool_call/tool_result/confirm_request/done/error）。
// source 标记调用入口（协议端点 / 内置助手聊天），用于业务指标 ptpatronus_llm_requests_total 的来源标签。
type Agent struct {
	source      string
	newProvider ProviderFactory
	tools       *ToolRegistry
	msgs        MessageStore
	convs       ConversationStore
	kv          KVStore
	crypto      Crypto
	confirms    *confirmRegistry
	stats       *statsCollector
}

// New 构造。source 见 Agent 注释；tools 为全部可用工具（含 handler）。
func New(source string, f ProviderFactory, tools []mcp.Tool, msgs MessageStore, convs ConversationStore, kv KVStore, crypto Crypto) *Agent {
	if f == nil {
		f = llm.New
	}
	return &Agent{source: source, newProvider: f, tools: NewToolRegistry(tools), msgs: msgs, convs: convs, kv: kv, crypto: crypto, confirms: newConfirmRegistry(), stats: newStatsCollector()}
}

// Resolve 送达一次危险工具确认决断（由 POST /agent/conversations/:id/confirm 调用）。
// 返回是否命中在途确认（false=已过期/已取消/未知 token）。
func (a *Agent) Resolve(token string, approved bool) bool { return a.confirms.resolve(token, approved) }

// ToolCatalog 返回全部工具目录（去 handler，含风险等级），供白名单 UI 分组展示。
func (a *Agent) ToolCatalog() []ToolCatalogEntry { return a.tools.Catalog() }

// Stats 返回用量统计快照（供 GET /agent/stats）。
func (a *Agent) Stats() StatsSnapshot { return a.stats.snapshot() }

// Run 执行一轮对话：持久化用户消息 → 加载历史 → ReAct 循环（工具调用→回灌→再请求）。
// convID 必须已存在（由 API handler 先建空会话）。
func (a *Agent) Run(ctx context.Context, convID uint, userText string) <-chan llm.Event {
	out := make(chan llm.Event, 64)
	safego.Go("agent:64", func() {
		defer close(out)
		ac, lc, loadErr := Load(a.kv, a.crypto)
		// 用量统计：仅在实际发起 LLM 调用时计数（配置未启用/无 key 的早返回不计）。
		start := time.Now()
		status := "ok"
		var inTok, outTok, toolCalls, toolErrors int
		counted := false
		defer func() {
			if !counted {
				return
			}
			a.stats.record(UsageRecord{
				ConversationID: convID,
				Provider:       lc.Provider,
				Model:          lc.Model,
				InputTokens:    inTok,
				OutputTokens:   outTok,
				CostUSD:        estimateCost(lc.Model, inTok, outTok),
				DurationMS:     time.Since(start).Milliseconds(),
				Status:         status,
				ToolCalls:      toolCalls,
				ToolErrors:     toolErrors,
				Timestamp:      start,
			})
			// Prometheus 口径与 stats 同源同刻：ok=正常收尾，error=其余（error/cancelled/ratelimit/maxiter）
			if status == "ok" {
				bizmetrics.LLMRequestInc(a.source, bizmetrics.LLMResultOK)
			} else {
				bizmetrics.LLMRequestInc(a.source, bizmetrics.LLMResultError)
			}
			bizmetrics.LLMTokensAdd(inTok, outTok)
		}()
		if errors.Is(loadErr, ErrConfigUnreadable) {
			// 读失败时 ac 是默认值（Enabled=false）：往下判就成了「未启用」这句假话。
			// loadErr 自身已带哨兵前缀，不能再叠一次。
			emit(out, llm.Event{Type: llm.EventError, Error: loadErr.Error() + unreadableConfigHint})
			return
		}
		if !ac.Enabled {
			emit(out, llm.Event{Type: llm.EventError, Error: "AI 助手未启用，请先在「设置 → AI 助手」配置 LLM provider 与 API key"})
			return
		}
		if loadErr != nil {
			emit(out, llm.Event{Type: llm.EventError, Error: ErrKeyUnreadable.Error() + "（主密钥可能已更换），请在「设置 → AI 助手」重新保存 API key"})
			return
		}
		if lc.APIKey == "" {
			emit(out, llm.Event{Type: llm.EventError, Error: "未配置 API key"})
			return
		}

		// 1. 持久化用户消息
		if err := a.msgs.Append(&models.AgentMessage{ConversationID: convID, Role: "user", Content: userText}); err != nil {
			emit(out, llm.Event{Type: llm.EventError, Error: "保存用户消息失败: " + err.Error()})
			return
		}
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

		// 3. 系统提示 + 裁剪（按模型 token 窗口预算，保留 system + 最近若干条）
		msgs = append([]llm.Message{{Role: llm.RoleSystem, Content: ac.SystemPrompt}}, msgs...)
		msgs = trimMessages(msgs, lc.Model, lc.MaxTokens)
		toolSchemas := a.tools.Schemas(ac.EnabledTools)
		provider := withRetry(a.newProvider(lc)) // 429/5xx/网络瞬时错误自动指数退避重试
		counted = true                           // 即将发起 LLM 调用，开始计入统计

		// 4. ReAct 循环
		for iter := 0; iter < ac.MaxIterations; iter++ {
			// 客户端断开（ctx 取消）则立即停止：避免继续调用计费 LLM 与执行写类工具，
			// 并防止 out channel 因无人消费而阻塞悬挂 goroutine。
			select {
			case <-ctx.Done():
				status = "cancelled"
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
				if strings.Contains(err.Error(), "429") {
					status = "ratelimit"
				} else {
					status = "error"
				}
				emit(out, llm.Event{Type: llm.EventError, Error: "LLM 调用失败：" + err.Error()})
				return
			}
			content, reasoning, calls, uIn, uOut, sawErr := consumeStream(stream, out)
			// 即使 sawErr 也累加本轮已收到的 EventUsage：上游限流/内容错误常在 done 前中断，
			// usage 可能仅部分到达（如只有 input），仍计入 stats 以反映真实消耗，而非强制归零。
			inTok += uIn
			outTok += uOut
			if sawErr {
				status = "error"
				return // 错误已在 consumeStream 转发；上方累加的 usage 由 defer 记入 stats
			}
			// 持久化本轮 assistant（含中间 tool-calling 轮）
			tcJSON, _ := json.Marshal(calls)
			if err := a.msgs.Append(&models.AgentMessage{ConversationID: convID, Role: "assistant", Content: content, Reasoning: reasoning, ToolCalls: string(tcJSON)}); err != nil {
				emit(out, llm.Event{Type: llm.EventError, Error: "保存助手消息失败: " + err.Error()})
				return
			}
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
				toolCalls++
				args, perr := parseArgs(tc.Arguments)
				var full string
				if perr != nil {
					// 参数解析失败：回灌错误让模型重试，而非以零值参数执行副作用工具。
					full = "ERROR: " + perr.Error()
				} else if destructive, note := a.tools.destructiveMeta(tc.Name); destructive {
					// 危险工具：执行前经 SSE 往返取得用户确认；未确认则不执行并回灌说明。
					approved, reason := a.confirmDestructive(ctx, out, convID, tc, note)
					if !approved {
						if ctx.Err() != nil {
							// 客户端断开/停止：补一条 tool 结果（保持 tool_call 序列完整，
							// 避免下轮 provider 因 assistant(tool_calls) 缺父 tool 结果而 400），
							// 行为与非破坏路径一致；取消不计入 toolErrors。
							full = "已取消：用户中断对话，此危险工具未执行。"
							emit(out, llm.Event{Type: llm.EventToolResult, ToolCall: tc, Text: full})
							// 这条落库就是上面注释说的「保序列完整」，它失败等于那道防线没立住：
							// 下一轮 provider 会因 assistant(tool_calls) 缺父 tool 结果而 400，
							// 用户只看到「问一句就报错」且无从归因。按本文件既有口径出声（见下方同款判定）。
							if err := a.msgs.Append(&models.AgentMessage{ConversationID: convID, Role: "tool", ToolCallID: tc.ID, Name: tc.Name, Content: full}); err != nil {
								emit(out, llm.Event{Type: llm.EventError, Error: "保存工具结果失败: " + err.Error() + "（这条会话的历史已不完整，请新建会话后重试）"})
							}
							status = "cancelled"
							emit(out, llm.Event{Type: llm.EventError, Error: "对话已取消"})
							return
						}
						full = "用户未确认此危险操作（" + reason + "），未执行。"
					} else {
						var execErr error
						full, execErr = a.tools.Exec(ctx, tc.Name, args, ac.EnabledTools)
						if execErr != nil {
							full = "ERROR: " + execErr.Error()
						}
					}
				} else {
					var execErr error
					full, execErr = a.tools.Exec(ctx, tc.Name, args, ac.EnabledTools)
					if execErr != nil {
						full = "ERROR: " + execErr.Error()
					}
				}
				if strings.HasPrefix(full, "ERROR:") {
					toolErrors++ // 仅解析/执行错误计入；拒绝确认不算工具错误
				}
				emit(out, llm.Event{Type: llm.EventToolResult, ToolCall: tc, Text: full}) // UI 看全量
				if err := a.msgs.Append(&models.AgentMessage{ConversationID: convID, Role: "tool", ToolCallID: tc.ID, Name: tc.Name, Content: full}); err != nil {
					emit(out, llm.Event{Type: llm.EventError, Error: "保存工具结果失败: " + err.Error()})
					return
				}
				msgs = append(msgs, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Name: tc.Name, Content: truncateResult(full)}) // LLM 截断
			}
			_ = a.convs.Touch(convID)
		}
		status = "maxiter"
		emit(out, llm.Event{Type: llm.EventError, Error: fmt.Sprintf("达到最大思考轮数(%d)，请缩小问题或重试", ac.MaxIterations)})
	})
	return out
}

// consumeStream 消费 provider 事件流：转发 text/reasoning/tool_call/error 到 out，
// 累积本轮 assistant 的 content/reasoning/tool_calls 与 token 用量。
// usage（EventUsage）仅供统计、不下发 out；返回 sawErr 表示是否出现错误（终态）。
// 错误处理契约：EventError 不 break 循环——继续读到 stream 关闭，以便捕获**错误之后**可能
// 到达的 EventUsage（部分 provider 在异常关闭前补发 usage），让 stats 能反映已消耗的 token。
func consumeStream(stream <-chan llm.Event, out chan<- llm.Event) (content, reasoning string, calls []llm.ToolCall, inTok, outTok int, sawErr bool) {
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
		case llm.EventUsage:
			// 防御：仅当本次 usage 携带非零值才更新，避免异常空 usage 覆盖先前已收到的部分 usage。
			if ev.Usage != nil && (ev.Usage.InputTokens > 0 || ev.Usage.OutputTokens > 0) {
				inTok = ev.Usage.InputTokens
				outTok = ev.Usage.OutputTokens
			}
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

// confirmDestructive 在执行 destructive 工具前经 SSE 往返请求用户确认。
// 已 emit 一条 confirm_request 事件（携带 token/风险/摘要）；客户端弹窗后
// POST /agent/conversations/:id/confirm → Agent.Resolve 唤醒本阻塞。
// 返回 (approved, reason)：approved=false 时 reason 说明未执行原因（拒绝/取消/超时/冲突）。
func (a *Agent) confirmDestructive(ctx context.Context, out chan<- llm.Event, convID uint, tc llm.ToolCall, note string) (bool, string) {
	token := fmt.Sprintf("%d:%s", convID, tc.ID)
	ch := a.confirms.register(token)
	if ch == nil {
		return false, "确认令牌冲突"
	}
	defer a.confirms.cancel(token) // 兜底清理（已 resolve 则 no-op）
	emit(out, llm.Event{
		Type:     llm.EventConfirmRequest,
		ToolCall: tc,
		Confirm:  &llm.ConfirmRequest{Token: token, Risk: mcp.RiskDestructive, Summary: note},
	})
	tctx, cancel := context.WithTimeout(ctx, confirmTimeout)
	defer cancel()
	select {
	case approved := <-ch:
		if !approved {
			return false, "用户拒绝"
		}
		return true, ""
	case <-tctx.Done():
		// 窄窗口竞态防御：resolve 可能恰在 tctx 到期瞬间送达 approved=true
		// （resolve 已 delete pending 并向 buffered channel 非阻塞 send，POST /confirm 返回 true，
		// 用户看到「已确认」），但本 select 同时命中 ch 与 tctx.Done()，Go 随机选分支可能误判超时。
		// 故超时分支先非阻塞再读一次 ch：若有 approved=true 则视为已批准继续执行，避免「已确认未执行」。
		select {
		case approved := <-ch:
			if approved {
				return true, ""
			}
			return false, "用户拒绝"
		default:
		}
		if ctx.Err() != nil {
			return false, "对话已取消"
		}
		return false, "确认超时"
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
