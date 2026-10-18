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

// toolFailStore 只让 role=tool 的追加失败：用户消息那条必须成功，否则流程根本走不到
// 被验的那个取消分支（会红在更早的出口上，等于没验到这条）。
type toolFailStore struct {
	*fakeMsgStore
	err error
}

func (s *toolFailStore) ListByConv(convID uint) ([]models.AgentMessage, error) {
	return s.fakeMsgStore.ListByConv(convID)
}

func (s *toolFailStore) Append(m *models.AgentMessage) error {
	if m.Role == "tool" {
		return s.err
	}
	return s.fakeMsgStore.Append(m)
}

// TestAgent_CancelledToolAppendFailureIsSurfaced destructive 工具在确认期被取消时，
// 代码注释写明「补一条 tool 结果以保持 tool_call 序列完整，否则下轮 provider 400」——
// 而那次补写从前是 `_ = a.msgs.Append(...)`，错误被丢：防线自己坏了没人知道，
// 用户下一句直接吃到 provider 400 且没有任何一句话指向原因。
// 同一个判定在本文件另一处（非破坏路径）是 `if err != nil { emit 错误; return }`，
// 抄第二遍就漂移了。
func TestAgent_CancelledToolAppendFailureIsSurfaced(t *testing.T) {
	plans := [][]llm.Event{
		{{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "boom", Arguments: `{}`}}, {Type: llm.EventDone}},
	}
	tools := []mcp.Tool{
		{Name: "boom", Description: "destructive boom", Risk: mcp.RiskDestructive, RiskNote: "会爆炸", InputSchema: map[string]any{"type": "object"},
			Handler: func(context.Context, map[string]any) (string, error) {
				t.Error("取消后不应执行危险工具")
				return "done", nil
			}},
	}
	cfgJSON, _ := json.Marshal(Config{Enabled: true, APIKeyEnc: "enc:k", MaxIterations: 3, SystemPrompt: "s"})
	kv := &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}
	store := &toolFailStore{fakeMsgStore: &fakeMsgStore{}, err: errors.New("database is locked")}

	a := New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider { return &fakeProvider{plans: plans} },
		tools, store, &fakeConvStore{titles: map[uint]string{}}, kv, fakeCrypto{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var errs []string
	sawToolResult := false
	for ev := range a.Run(ctx, 1, "炸一下") {
		switch ev.Type {
		case llm.EventConfirmRequest:
			cancel() // 用户在确认框上还来不及点，就关了页面 / 点了停止
		case llm.EventToolResult:
			sawToolResult = true
		case llm.EventError:
			errs = append(errs, ev.Error)
		}
	}

	joined := strings.Join(errs, " | ")
	if !sawToolResult {
		t.Fatal("取消时没补 tool 结果事件（前置分支没走到，后面的断言都是空转）")
	}
	if !strings.Contains(joined, "保存工具结果失败") {
		t.Errorf("落库失败必须出声（旧行为是 _ = 丢掉，只剩「对话已取消」，下一轮 400 无从归因）, got %q", joined)
	}
	if !strings.Contains(joined, "database is locked") {
		t.Errorf("要带上真实原因, got %q", joined)
	}
	if !strings.Contains(joined, "新建会话") {
		t.Errorf("要给可操作动作（这条会话已不可继续）, got %q", joined)
	}
	if !strings.Contains(joined, "对话已取消") {
		t.Errorf("取消本身仍要如实说明, got %q", joined)
	}
}
