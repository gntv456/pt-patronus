package aiagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/llm"
	"github.com/hxpt/ptpatronus/internal/mcp"
)

// fakes —— 内存实现各接口。

type fakeProvider struct {
	round int
	plans [][]llm.Event
}

func (f *fakeProvider) ChatStream(_ context.Context, _ llm.Req) (<-chan llm.Event, error) {
	ch := make(chan llm.Event, 16)
	var evs []llm.Event
	if f.round < len(f.plans) {
		evs = f.plans[f.round]
	} else {
		evs = []llm.Event{{Type: llm.EventDone}} // 额外调用（如标题总结）兜底空响应
	}
	f.round++
	go func() {
		for _, e := range evs {
			ch <- e
		}
		close(ch)
	}()
	return ch, nil
}

type fakeMsgStore struct{ msgs []models.AgentMessage }

func (s *fakeMsgStore) ListByConv(_ uint) ([]models.AgentMessage, error) { return s.msgs, nil }
func (s *fakeMsgStore) Append(m *models.AgentMessage) error              { s.msgs = append(s.msgs, *m); return nil }

type fakeConvStore struct{ titles map[uint]string }

func (c *fakeConvStore) SetTitle(id uint, t string) error { c.titles[id] = t; return nil }
func (c *fakeConvStore) Touch(_ uint) error               { return nil }

type fakeKV struct{ m map[string]string }

func (k *fakeKV) Get(key string) (string, error) { return k.m[key], nil }
func (k *fakeKV) Set(key string, v string) error { k.m[key] = v; return nil }

type fakeCrypto struct{}

func (fakeCrypto) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (fakeCrypto) Decrypt(c string) (string, error) { return strings.TrimPrefix(c, "enc:"), nil }

// 工具：ping 返回 pong；fail 总是报错。
func testTools() []mcp.Tool {
	return []mcp.Tool{
		{Name: "ping", Description: "ping", InputSchema: map[string]any{"type": "object"}, Handler: func(_ context.Context, _ map[string]any) (string, error) {
			return "pong", nil
		}},
		{Name: "fail", Description: "fail", InputSchema: map[string]any{"type": "object"}, Handler: func(_ context.Context, _ map[string]any) (string, error) {
			return "", errSentinel
		}},
	}
}

var errSentinel = sentinel{}

type sentinel struct{}

func (sentinel) Error() string { return "boom" }

// TestAgent_ReActLoop 工具调用→回灌→最终文本。
func TestAgent_ReActLoop(t *testing.T) {
	plan := [][]llm.Event{
		// 第 1 轮：先思考，再发起 ping 工具调用
		{llm.Event{Type: llm.EventReasoning, Text: "先查"}, llm.Event{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "ping", Arguments: `{}`}}, llm.Event{Type: llm.EventDone}},
		// 第 2 轮：用工具结果给出最终答案
		{llm.Event{Type: llm.EventText, Text: "pong收到的"}, llm.Event{Type: llm.EventDone}},
	}
	store := &fakeMsgStore{}
	cfgJSON, _ := json.Marshal(Config{Enabled: true, Provider: "openai", APIKeyEnc: "enc:k", MaxIterations: 5, SystemPrompt: "sys"})
	kv := &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}
	a := New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider { return &fakeProvider{plans: plan} }, testTools(), store, &fakeConvStore{titles: map[uint]string{}}, kv, fakeCrypto{})

	var texts, reasoning string
	var toolCalls, toolResults []string
	var sawDone, sawErr bool
	for ev := range a.Run(context.Background(), 1, "ping一下") {
		switch ev.Type {
		case llm.EventText:
			texts += ev.Text
		case llm.EventReasoning:
			reasoning += ev.Text
		case llm.EventToolCall:
			toolCalls = append(toolCalls, ev.ToolCall.Name)
		case llm.EventToolResult:
			toolResults = append(toolResults, ev.Text)
		case llm.EventDone:
			sawDone = true
		case llm.EventError:
			sawErr = true
		}
	}
	if sawErr {
		t.Fatal("unexpected error event")
	}
	if !sawDone {
		t.Fatal("missing done")
	}
	if len(toolCalls) != 1 || toolCalls[0] != "ping" {
		t.Fatalf("toolCalls: %v", toolCalls)
	}
	if len(toolResults) != 1 || toolResults[0] != "pong" {
		t.Fatalf("toolResults: %v", toolResults)
	}
	if texts != "pong收到的" {
		t.Fatalf("text: got %q", texts)
	}
	if reasoning != "先查" {
		t.Fatalf("reasoning: got %q", reasoning)
	}
	// 持久化：user + assistant(tool) + tool(pong) + assistant(final)
	roles := []string{}
	for _, m := range store.msgs {
		roles = append(roles, m.Role)
	}
	if len(roles) != 4 || roles[0] != "user" || roles[2] != "tool" {
		t.Fatalf("persisted roles: %v", roles)
	}
}

// TestAgent_ToolErrorFedBack 工具报错→回灌错误→模型正常结束（不崩）。
func TestAgent_ToolErrorFedBack(t *testing.T) {
	plan := [][]llm.Event{
		{llm.Event{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c1", Name: "fail", Arguments: `{}`}}, llm.Event{Type: llm.EventDone}},
		{llm.Event{Type: llm.EventText, Text: "知道了"}, llm.Event{Type: llm.EventDone}},
	}
	store := &fakeMsgStore{}
	cfgJSON, _ := json.Marshal(Config{Enabled: true, APIKeyEnc: "enc:k", MaxIterations: 5, SystemPrompt: "s"})
	kv := &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}
	a := New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider { return &fakeProvider{plans: plan} }, testTools(), store, &fakeConvStore{titles: map[uint]string{}}, kv, fakeCrypto{})

	var toolResults []string
	var sawDone bool
	for ev := range a.Run(context.Background(), 1, "fail") {
		switch ev.Type {
		case llm.EventToolResult:
			toolResults = append(toolResults, ev.Text)
		case llm.EventDone:
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatal("missing done after tool error")
	}
	if len(toolResults) != 1 || !strings.HasPrefix(toolResults[0], "ERROR:") {
		t.Fatalf("expected ERROR tool result, got %v", toolResults)
	}
}

// TestAgent_MaxIterations 触顶退出。
func TestAgent_MaxIterations(t *testing.T) {
	// 每轮都发 tool_call，永不结束 → 触顶。
	loop := []llm.Event{llm.Event{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "c", Name: "ping", Arguments: `{}`}}, llm.Event{Type: llm.EventDone}}
	plans := [][]llm.Event{}
	for i := 0; i < 5; i++ {
		plans = append(plans, loop)
	}
	cfgJSON, _ := json.Marshal(Config{Enabled: true, APIKeyEnc: "enc:k", MaxIterations: 3, SystemPrompt: "s"})
	kv := &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}
	a := New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider { return &fakeProvider{plans: plans} }, testTools(), &fakeMsgStore{}, &fakeConvStore{titles: map[uint]string{}}, kv, fakeCrypto{})
	var sawErr bool
	for ev := range a.Run(context.Background(), 1, "go") {
		if ev.Type == llm.EventError {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("expected max-iterations error")
	}
}

// TestAgent_StreamErrorPreservesUsage 流发 usage 后再出错，
// 验证 stats 仍记录已收到的部分 usage（不因 sawErr 强制归零）。
func TestAgent_StreamErrorPreservesUsage(t *testing.T) {
	plans := [][]llm.Event{
		{
			llm.Event{Type: llm.EventUsage, Usage: &llm.Usage{InputTokens: 1234, OutputTokens: 56}},
			llm.Event{Type: llm.EventError, Error: "upstream 500"},
		},
	}
	cfgJSON, _ := json.Marshal(Config{Enabled: true, APIKeyEnc: "enc:k", MaxIterations: 3, SystemPrompt: "s"})
	kv := &fakeKV{m: map[string]string{configKey: string(cfgJSON)}}
	a := New(bizmetrics.LLMSourceAssistant, func(llm.Config) llm.Provider { return &fakeProvider{plans: plans} }, testTools(), &fakeMsgStore{}, &fakeConvStore{titles: map[uint]string{}}, kv, fakeCrypto{})

	for range a.Run(context.Background(), 1, "go") {
	}

	snap := a.Stats()
	if len(snap.Recent) != 1 {
		t.Fatalf("stats recent len=%d want 1", len(snap.Recent))
	}
	r := snap.Recent[0]
	if r.Status != "error" {
		t.Fatalf("status=%q want error", r.Status)
	}
	if r.InputTokens != 1234 || r.OutputTokens != 56 {
		t.Fatalf("tokens in=%d out=%d, want 1234/56 (preserved despite stream error)", r.InputTokens, r.OutputTokens)
	}
}
