// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package aiagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"github.com/hxpt/ptpatronus/internal/llm"
	"github.com/hxpt/ptpatronus/internal/mcp"
)

// newSensitiveTestAgent 构造一个挂了 sensitive 工具 "read_secret" 的 agent。
func newSensitiveTestAgent(t *testing.T, plans [][]llm.Event, executed *bool) *Agent {
	t.Helper()
	tools := []mcp.Tool{
		{Name: "read_secret", Description: "Read sensitive data", Risk: mcp.RiskSensitive, RiskNote: "读取敏感数据", InputSchema: map[string]any{"type": "object"}, Handler: func(context.Context, map[string]any) (string, error) {
			*executed = true
			return "secret-value", nil
		}},
	}
	cfgJSON, _ := json.Marshal(Config{Enabled: true, APIKeyEnc: "enc:k", MaxIterations: 3, SystemPrompt: "s"})
	kv := &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}
	return New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider { return &fakeProvider{plans: plans} }, tools, &fakeMsgStore{}, &fakeConvStore{titles: map[uint]string{}}, kv, fakeCrypto{})
}

// sensitive 工具必须走确认闸门；拒绝后 handler 不执行。
func TestAgent_SensitiveDeniedNotExecuted(t *testing.T) {
	executed := false
	plans := [][]llm.Event{
		{{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "read_secret", Arguments: `{}`}}, {Type: llm.EventDone}},
		{{Type: llm.EventText, Text: "ok"}, {Type: llm.EventDone}},
	}
	a := newSensitiveTestAgent(t, plans, &executed)
	var confirmRisk string
	for ev := range a.Run(context.Background(), 1, "读") {
		if ev.Type == llm.EventConfirmRequest {
			confirmRisk = ev.Confirm.Risk
			a.Resolve(ev.Confirm.Token, false) // 拒绝
		}
	}
	if confirmRisk != mcp.RiskSensitive {
		t.Fatalf("confirm risk=%q, want %q", confirmRisk, mcp.RiskSensitive)
	}
	if executed {
		t.Fatal("denied sensitive tool handler executed")
	}
}

// 批准后 sensitive 工具执行，确认事件携带 sensitive 风险等级。
func TestAgent_SensitiveApprovedExecutes(t *testing.T) {
	executed := false
	plans := [][]llm.Event{
		{{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "read_secret", Arguments: `{}`}}, {Type: llm.EventDone}},
		{{Type: llm.EventText, Text: "ok"}, {Type: llm.EventDone}},
	}
	a := newSensitiveTestAgent(t, plans, &executed)
	var confirmRisk, toolResult string
	for ev := range a.Run(context.Background(), 1, "读") {
		switch ev.Type {
		case llm.EventConfirmRequest:
			confirmRisk = ev.Confirm.Risk
			a.Resolve(ev.Confirm.Token, true)
		case llm.EventToolResult:
			toolResult = ev.Text
		}
	}
	if confirmRisk != mcp.RiskSensitive || !executed || toolResult != "secret-value" {
		t.Fatalf("risk=%q executed=%v result=%q", confirmRisk, executed, toolResult)
	}
}
