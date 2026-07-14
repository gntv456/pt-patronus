package aiagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/llm"
)

// flakyProvider 前 failUntil 次返回错误（HTTP 状态码或网络错误），之后返回一条 ok 文本流。
type flakyProvider struct {
	failUntil int
	status    int // >0 → 模拟 "openai upstream HTTP %d"；0 → 模拟网络错误
	calls     int
}

func (f *flakyProvider) ChatStream(_ context.Context, _ llm.Req) (<-chan llm.Event, error) {
	f.calls++
	if f.calls <= f.failUntil {
		if f.status > 0 {
			return nil, fmt.Errorf("openai upstream HTTP %d", f.status)
		}
		return nil, fakeNetErr{} // net.Error → 可重试
	}
	ch := make(chan llm.Event, 2)
	go func() {
		ch <- llm.Event{Type: llm.EventText, Text: "ok"}
		ch <- llm.Event{Type: llm.EventDone}
		close(ch)
	}()
	return ch, nil
}

// fakeNetErr 实现 net.Error（带 Timeout/Temporary），用于测试网络错误重试分类。
type fakeNetErr struct{}

func (fakeNetErr) Error() string   { return "fake network error" }
func (fakeNetErr) Timeout() bool   { return true }
func (fakeNetErr) Temporary() bool { return true }

func drain(stream <-chan llm.Event) string {
	var s string
	for ev := range stream {
		if ev.Type == llm.EventText {
			s += ev.Text
		}
	}
	return s
}

func TestIsRetryableLLMError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("openai upstream HTTP 429"), true},
		{fmt.Errorf("anthropic upstream HTTP 503"), true},
		{fmt.Errorf("openai upstream HTTP 400"), false},  // 4xx 非法请求
		{fmt.Errorf("openai upstream HTTP 401"), false},  // 鉴权失败
		{fakeNetErr{}, true},                             // 网络
		{context.Canceled, false},                        // ctx 取消
		{errors.New("some other error"), false},          // 未知/JSON 错误
		{nil, false},
	}
	for i, c := range cases {
		if got := isRetryableLLMError(c.err); got != c.want {
			t.Errorf("case %d: isRetryable(%v)=%v want %v", i, c.err, got, c.want)
		}
	}
}

func TestRetryProviderRetriesOn429(t *testing.T) {
	fp := &flakyProvider{failUntil: 2, status: 429}
	rp := withRetryCfg(fp, 3, time.Millisecond) // 小退避加速
	stream, err := rp.ChatStream(context.Background(), llm.Req{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := drain(stream); got != "ok" {
		t.Fatalf("stream=%q", got)
	}
	if fp.calls != 3 { // 首次失败 + 2 次重试成功
		t.Fatalf("calls=%d want 3", fp.calls)
	}
}

func TestRetryProviderRetriesOnNetworkError(t *testing.T) {
	fp := &flakyProvider{failUntil: 1, status: 0} // 网络错误
	rp := withRetryCfg(fp, 3, time.Millisecond)
	stream, err := rp.ChatStream(context.Background(), llm.Req{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	drain(stream)
	if fp.calls != 2 {
		t.Fatalf("calls=%d want 2", fp.calls)
	}
}

func TestRetryProviderNoRetryOn400(t *testing.T) {
	fp := &flakyProvider{failUntil: 99, status: 400}
	rp := withRetryCfg(fp, 3, time.Millisecond)
	_, err := rp.ChatStream(context.Background(), llm.Req{})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if fp.calls != 1 { // 不可重试 → 不重试
		t.Fatalf("calls=%d want 1", fp.calls)
	}
}

func TestRetryProviderExhaustsAndWraps(t *testing.T) {
	fp := &flakyProvider{failUntil: 99, status: 429}
	rp := withRetryCfg(fp, 2, time.Millisecond) // 额外重试 2 次
	_, err := rp.ChatStream(context.Background(), llm.Req{})
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if !strings.Contains(err.Error(), "重试 2 次仍失败") {
		t.Fatalf("error=%q", err.Error())
	}
	if fp.calls != 3 { // 首次 + 2 次重试
		t.Fatalf("calls=%d want 3", fp.calls)
	}
}

func TestRetryProviderStopsOnCtxCancel(t *testing.T) {
	fp := &flakyProvider{failUntil: 99, status: 429}
	rp := withRetryCfg(fp, 5, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	_, err := rp.ChatStream(ctx, llm.Req{})
	if err == nil {
		t.Fatal("expected error on ctx cancel")
	}
	// 不应打满 5 次重试（被 ctx 中断）
	if fp.calls >= 5 {
		t.Fatalf("calls=%d, should be cut short by ctx cancel", fp.calls)
	}
}

// —— Token 窗口裁剪 ——

func TestContextBudgetModelAware(t *testing.T) {
	if b := contextBudget("claude-sonnet-5", 2048); b < 100000 {
		t.Errorf("claude budget=%d, want large", b)
	}
	if b := contextBudget("gpt-4o-mini", 2048); b < 60000 {
		t.Errorf("gpt-4o budget=%d, want >=60k", b)
	}
	if b := contextBudget("unknown-model", 2048); b < 8000 || b > 32000 {
		t.Errorf("unknown budget=%d, want 8k..32k", b)
	}
	// max_tokens 占预留：大输出应压缩输入预算
	bSmall := contextBudget("gpt-4o-mini", 2048)
	bLarge := contextBudget("gpt-4o-mini", 60000)
	if bLarge >= bSmall {
		t.Errorf("larger max_tokens should shrink input budget: %d vs %d", bLarge, bSmall)
	}
}

func TestTrimByTokenBudget(t *testing.T) {
	// 构造超出预算的长上下文：system + 多条大 user 消息。
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: "sys"}}
	big := strings.Repeat("中", 6000) // ~2000 token/条
	for i := 0; i < 20; i++ {
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: big})
	}
	trimmed := trimMessages(msgs, "qwen-max", 2048) // 32k 窗口 → 预算 ~28k；每条 ~2000 tok，20 条必超
	// 应保留 system 且裁掉最早的若干条
	if len(trimmed) < 2 {
		t.Fatalf("trimmed too far: %d", len(trimmed))
	}
	if trimmed[0].Role != llm.RoleSystem {
		t.Fatalf("first must be system, got %s", trimmed[0].Role)
	}
	// 最后一条必须保留（最近上下文最重要）
	if trimmed[len(trimmed)-1].Content != big {
		t.Fatal("last message must be retained")
	}
	// token 估算不应超预算太多（含 system）
	budget := contextBudget("qwen-max", 2048)
	total := 0
	for _, m := range trimmed {
		total += estimateTokens(m)
	}
	if total > budget+10 { // 允许单条取整误差
		t.Fatalf("trimmed total tokens %d > budget %d", total, budget)
	}
}

func TestTrimKeepsAllWhenUnderBudget(t *testing.T) {
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: "sys"}}
	for i := 0; i < 5; i++ {
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: "hi"})
	}
	trimmed := trimMessages(msgs, "gpt-4o-mini", 2048)
	if len(trimmed) != 6 { // system + 5，预算充裕全留
		t.Fatalf("len=%d want 6", len(trimmed))
	}
}
