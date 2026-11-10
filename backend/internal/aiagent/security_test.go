// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/llm"
	"github.com/hxpt/ptpatronus/internal/mcp"
)

type failingMessageStore struct{}

func (failingMessageStore) ListByConv(uint) ([]models.AgentMessage, error) { return nil, nil }
func (failingMessageStore) Append(*models.AgentMessage) error              { return errors.New("db unavailable") }

func TestAgentToolWhitelistBlocksUnadvertisedCall(t *testing.T) {
	executed := false
	tools := []mcp.Tool{
		{Name: "safe", InputSchema: map[string]any{"type": "object"}, Handler: func(context.Context, map[string]any) (string, error) { return "ok", nil }},
		{Name: "danger", InputSchema: map[string]any{"type": "object"}, Handler: func(context.Context, map[string]any) (string, error) {
			executed = true
			return "changed", nil
		}},
	}
	plans := [][]llm.Event{
		{{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "danger", Arguments: `{}`}}, {Type: llm.EventDone}},
		{{Type: llm.EventText, Text: "blocked"}, {Type: llm.EventDone}},
	}
	cfgJSON, _ := json.Marshal(Config{Enabled: true, APIKeyEnc: "enc:k", MaxIterations: 3, SystemPrompt: "s", EnabledTools: []string{"safe"}})
	agent := New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider { return &fakeProvider{plans: plans} }, tools, &fakeMsgStore{}, &fakeConvStore{titles: map[uint]string{}}, &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}, fakeCrypto{})

	var result string
	for event := range agent.Run(context.Background(), 1, "run danger") {
		if event.Type == llm.EventToolResult {
			result = event.Text
		}
	}
	if executed {
		t.Fatal("disabled tool handler executed")
	}
	if !strings.Contains(result, "is not enabled") {
		t.Fatalf("tool result=%q", result)
	}
}

func TestAgentMalformedArgumentsDoNotExecuteTool(t *testing.T) {
	executed := false
	tools := []mcp.Tool{{Name: "write", InputSchema: map[string]any{"type": "object"}, Handler: func(context.Context, map[string]any) (string, error) {
		executed = true
		return "changed", nil
	}}}
	plans := [][]llm.Event{
		{{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "write", Arguments: `{bad`}}, {Type: llm.EventDone}},
		{{Type: llm.EventText, Text: "fixed"}, {Type: llm.EventDone}},
	}
	cfgJSON, _ := json.Marshal(Config{Enabled: true, APIKeyEnc: "enc:k", MaxIterations: 3, SystemPrompt: "s"})
	agent := New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider { return &fakeProvider{plans: plans} }, tools, &fakeMsgStore{}, &fakeConvStore{titles: map[uint]string{}}, &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}, fakeCrypto{})
	for range agent.Run(context.Background(), 1, "write") {
	}
	if executed {
		t.Fatal("tool executed with malformed arguments")
	}
}

func TestTrimMessagesDropsOrphanedToolPrefix(t *testing.T) {
	messages := []llm.Message{{Role: llm.RoleSystem, Content: "system"}, {Role: llm.RoleUser, Content: "old"}, {Role: llm.RoleTool, ToolCallID: "orphan", Content: "result"}}
	for i := 0; i < maxMessages-1; i++ {
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: "message"})
	}
	trimmed := trimMessages(messages, "gpt-4o-mini", 2048)
	if len(trimmed) < 2 {
		t.Fatalf("trimmed too far: %v", trimmed)
	}
	if trimmed[0].Role != llm.RoleSystem {
		t.Fatalf("first role=%s, want system", trimmed[0].Role)
	}
	if trimmed[1].Role == llm.RoleTool {
		t.Fatalf("orphaned tool message remains at context boundary: %+v", trimmed[1])
	}
}

func TestAgentStopsWhenUserMessageCannotBePersisted(t *testing.T) {
	cfgJSON, _ := json.Marshal(Config{Enabled: true, APIKeyEnc: "enc:k", MaxIterations: 3, SystemPrompt: "s"})
	providerCalled := false
	agent := New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider {
		providerCalled = true
		return &fakeProvider{}
	}, nil, failingMessageStore{}, &fakeConvStore{titles: map[uint]string{}}, &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}, fakeCrypto{})

	var errorText string
	for event := range agent.Run(context.Background(), 1, "hello") {
		if event.Type == llm.EventError {
			errorText = event.Error
		}
	}
	if !strings.Contains(errorText, "db unavailable") {
		t.Fatalf("error=%q", errorText)
	}
	if providerCalled {
		t.Fatal("provider ran after persistence failure")
	}
}

// TestConfirmRegistry 覆盖 register/resolve/cancel/未知 token/重复注册。
func TestConfirmRegistry(t *testing.T) {
	r := newConfirmRegistry()

	// register → resolve(true) 送达 true
	ch := r.register("t1")
	if ch == nil {
		t.Fatal("register returned nil chan")
	}
	if !r.resolve("t1", true) {
		t.Fatal("resolve should hit pending")
	}
	if v := <-ch; !v {
		t.Fatal("want approved=true delivered")
	}
	// 已 resolve 的 token 再次 resolve → miss（不 panic）
	if r.resolve("t1", true) {
		t.Fatal("double resolve should miss")
	}
	// 未知 token → miss
	if r.resolve("nope", false) {
		t.Fatal("unknown token should miss")
	}
	// cancel 后 resolve → miss（cancel 仅 housekeeping，不发值）
	r.register("t2")
	r.cancel("t2")
	if r.resolve("t2", true) {
		t.Fatal("resolve after cancel should miss")
	}
	// 重复 register 同 token → nil（避免覆盖在途确认）
	r.register("t3")
	if r.register("t3") != nil {
		t.Fatal("duplicate register should return nil")
	}
}

// newConfirmTestAgent 构造一个挂了 destructive 工具 "boom" 的 agent（共用 fake 依赖）。
func newConfirmTestAgent(t *testing.T, plans [][]llm.Event, executed *bool) *Agent {
	t.Helper()
	tools := []mcp.Tool{
		{Name: "boom", Description: "destructive boom", Risk: mcp.RiskDestructive, RiskNote: "会爆炸", InputSchema: map[string]any{"type": "object"}, Handler: func(context.Context, map[string]any) (string, error) {
			*executed = true
			return "done", nil
		}},
	}
	cfgJSON, _ := json.Marshal(Config{Enabled: true, APIKeyEnc: "enc:k", MaxIterations: 3, SystemPrompt: "s"})
	kv := &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}
	return New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider { return &fakeProvider{plans: plans} }, tools, &fakeMsgStore{}, &fakeConvStore{titles: map[uint]string{}}, kv, fakeCrypto{})
}

// TestAgent_DestructiveApprovedExecutes 用户批准 → 工具执行。
func TestAgent_DestructiveApprovedExecutes(t *testing.T) {
	executed := false
	plans := [][]llm.Event{
		{{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "boom", Arguments: `{}`}}, {Type: llm.EventDone}},
		{{Type: llm.EventText, Text: "完成"}, {Type: llm.EventDone}},
	}
	a := newConfirmTestAgent(t, plans, &executed)

	var toolResult string
	var sawConfirm, sawDone bool
	for ev := range a.Run(context.Background(), 1, "炸") {
		switch ev.Type {
		case llm.EventConfirmRequest:
			sawConfirm = true
			a.Resolve(ev.Confirm.Token, true) // 批准
		case llm.EventToolResult:
			toolResult = ev.Text
		case llm.EventDone:
			sawDone = true
		}
	}
	if !sawConfirm {
		t.Fatal("missing confirm_request")
	}
	if !executed {
		t.Fatal("approved destructive not executed")
	}
	if toolResult != "done" {
		t.Fatalf("tool result=%q", toolResult)
	}
	if !sawDone {
		t.Fatal("missing done")
	}
}

// TestAgent_DestructiveDeniedNotExecuted 用户拒绝 → 工具不执行，回灌「未确认」。
func TestAgent_DestructiveDeniedNotExecuted(t *testing.T) {
	executed := false
	plans := [][]llm.Event{
		{{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "boom", Arguments: `{}`}}, {Type: llm.EventDone}},
		{{Type: llm.EventText, Text: "好吧"}, {Type: llm.EventDone}},
	}
	a := newConfirmTestAgent(t, plans, &executed)

	var toolResult string
	for ev := range a.Run(context.Background(), 1, "炸") {
		switch ev.Type {
		case llm.EventConfirmRequest:
			a.Resolve(ev.Confirm.Token, false) // 拒绝
		case llm.EventToolResult:
			toolResult = ev.Text
		}
	}
	if executed {
		t.Fatal("denied destructive executed")
	}
	if !strings.Contains(toolResult, "未确认") {
		t.Fatalf("expected denial tool result, got %q", toolResult)
	}
}

// TestAgent_DestructiveCancelledOnCtxCancel 确认等待期间 ctx 取消 → 不执行、报错。
func TestAgent_DestructiveCancelledOnCtxCancel(t *testing.T) {
	executed := false
	plans := [][]llm.Event{
		{{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "boom", Arguments: `{}`}}, {Type: llm.EventDone}},
	}
	a := newConfirmTestAgent(t, plans, &executed)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // 兜底：即使未在闸门处触发，函数返回时也释放
	var sawErr bool
	for ev := range a.Run(ctx, 1, "炸") {
		if ev.Type == llm.EventConfirmRequest {
			cancel() // 在闸门处取消（不 resolve）
		}
		if ev.Type == llm.EventError {
			sawErr = true
		}
	}
	if executed {
		t.Fatal("executed despite ctx cancel at gate")
	}
	if !sawErr {
		t.Fatal("expected error event on cancel")
	}
}

// TestToolRegistryCatalog 验证目录透出风险等级、destructive 带说明、未标注兜底 write。
func TestToolRegistryCatalog(t *testing.T) {
	reg := NewToolRegistry([]mcp.Tool{
		{Name: "torrents_delete", Description: "del", Risk: mcp.RiskDestructive, RiskNote: "删种子"},
		{Name: "sites.list", Description: "list", Risk: mcp.RiskRead},
		{Name: "subscribe_create", Description: "create", Risk: mcp.RiskWrite},
		{Name: "legacy", Description: "no risk tag"}, // 未标注 → 兜底 write
	})
	cat := reg.Catalog()
	if len(cat) != 4 {
		t.Fatalf("catalog len=%d", len(cat))
	}
	byName := map[string]ToolCatalogEntry{}
	for _, e := range cat {
		byName[e.Name] = e
	}
	if byName["torrents_delete"].Risk != mcp.RiskDestructive || byName["torrents_delete"].RiskNote != "删种子" {
		t.Errorf("destructive entry wrong: %+v", byName["torrents_delete"])
	}
	if byName["sites.list"].Risk != mcp.RiskRead {
		t.Errorf("sites.list risk=%q", byName["sites.list"].Risk)
	}
	if byName["legacy"].Risk != mcp.RiskWrite {
		t.Errorf("untagged should default to write, got %q", byName["legacy"].Risk)
	}
}

// TestAgent_DestructiveCancelledEmitsToolResult 验证 destructive 工具确认期 ctx 取消时，
// 仍补一条 tool 结果（emit + DB 持久化），保持 tool_call 序列完整——
// 否则下轮 provider 会因 assistant(tool_calls) 缺父 tool 结果而 400。
func TestAgent_DestructiveCancelledEmitsToolResult(t *testing.T) {
	executed := false
	store := &fakeMsgStore{}
	tools := []mcp.Tool{
		{Name: "boom", Description: "destructive boom", Risk: mcp.RiskDestructive, RiskNote: "会爆炸", InputSchema: map[string]any{"type": "object"}, Handler: func(context.Context, map[string]any) (string, error) {
			executed = true
			return "done", nil
		}},
	}
	plans := [][]llm.Event{
		{{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "boom", Arguments: `{}`}}, {Type: llm.EventDone}},
	}
	cfgJSON, _ := json.Marshal(Config{Enabled: true, APIKeyEnc: "enc:k", MaxIterations: 3, SystemPrompt: "s"})
	kv := &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}
	a := New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider { return &fakeProvider{plans: plans} }, tools, store, &fakeConvStore{titles: map[uint]string{}}, kv, fakeCrypto{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sawToolResult, sawErr bool
	var toolResultText string
	for ev := range a.Run(ctx, 1, "炸") {
		switch ev.Type {
		case llm.EventConfirmRequest:
			cancel()
		case llm.EventToolResult:
			sawToolResult = true
			toolResultText = ev.Text
		case llm.EventError:
			sawErr = true
		}
	}
	if executed {
		t.Fatal("destructive tool should not execute on cancel")
	}
	if !sawErr {
		t.Fatal("expected error event on cancel")
	}
	if !sawToolResult {
		t.Fatal("expected tool_result emitted on cancel to keep tool_call sequence intact")
	}
	if !strings.Contains(toolResultText, "已取消") {
		t.Fatalf("tool result=%q, want contains 已取消", toolResultText)
	}
	// DB 应有对应 tool 消息（ToolCallID=c1），内容含"已取消"
	var hasToolMsg bool
	for _, m := range store.msgs {
		if m.Role == "tool" && m.ToolCallID == "c1" {
			hasToolMsg = true
			if !strings.Contains(m.Content, "已取消") {
				t.Fatalf("db tool content=%q", m.Content)
			}
		}
	}
	if !hasToolMsg {
		t.Fatal("expected tool message persisted in DB to keep sequence complete")
	}
}
