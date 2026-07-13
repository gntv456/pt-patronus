package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenAIChatStream_ParseReasoningContentToolCalls 用 httptest 喂一段 OpenAI 兼容 SSE，
// 断言解析出 reasoning + text + 增量拼接的 tool_call。
func TestOpenAIChatStream_ParseReasoningContentToolCalls(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"先思考一下"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":" world"}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"ping","arguments":""}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":1}"}}]}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := New(Config{Provider: "openai", BaseURL: srv.URL, APIKey: "k"})
	ch, err := p.ChatStream(context.Background(), Req{Messages: []Message{{Role: RoleUser, Content: "hi"}}, Model: "m"})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	var text, reasoning string
	var calls []ToolCall
	var sawDone bool
	for ev := range ch {
		switch ev.Type {
		case EventText:
			text += ev.Text
		case EventReasoning:
			reasoning += ev.Text
		case EventToolCall:
			calls = append(calls, ev.ToolCall)
		case EventDone:
			sawDone = true
		}
	}
	if text != "Hello world" {
		t.Fatalf("text: got %q", text)
	}
	if reasoning != "先思考一下" {
		t.Fatalf("reasoning: got %q", reasoning)
	}
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "ping" || calls[0].Arguments != `{"x":1}` {
		t.Fatalf("calls: %+v", calls)
	}
	if !sawDone {
		t.Fatal("missing done")
	}
}
