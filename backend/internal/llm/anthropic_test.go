package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAnthropicChatStream_ThinkingTextToolUse 喂 Anthropic SSE，断言 thinking + text + tool_use(input_json_delta 拼接)。
func TestAnthropicChatStream_ThinkingTextToolUse(t *testing.T) {
	body := strings.Join([]string{
		`event: message_start`, `data: {"type":"message_start"}`, ``,
		`event: content_block_start`, `data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`, ``,
		`event: content_block_delta`, `data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"深度"}}`, ``,
		`event: content_block_delta`, `data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"思考"}}`, ``,
		`event: content_block_stop`, `data: {"type":"content_block_stop","index":0}`, ``,
		`event: content_block_start`, `data: {"type":"content_block_start","index":1,"content_block":{"type":"text"}}`, ``,
		`event: content_block_delta`, `data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hi"}}`, ``,
		`event: content_block_stop`, `data: {"type":"content_block_stop","index":1}`, ``,
		`event: content_block_start`, `data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tu_1","name":"ping"}}`, ``,
		`event: content_block_delta`, `data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"a\":"}}`, ``,
		`event: content_block_delta`, `data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"1}"}}`, ``,
		`event: content_block_stop`, `data: {"type":"content_block_stop","index":2}`, ``,
		`event: message_stop`, `data: {"type":"message_stop"}`, ``,
	}, "\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := New(Config{Provider: "anthropic", BaseURL: srv.URL, APIKey: "k"})
	ch, err := p.ChatStream(context.Background(), Req{Messages: []Message{{Role: RoleUser, Content: "hi"}}, Model: "claude"})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	var text, reasoning string
	var calls []ToolCall
	for ev := range ch {
		switch ev.Type {
		case EventText:
			text += ev.Text
		case EventReasoning:
			reasoning += ev.Text
		case EventToolCall:
			calls = append(calls, ev.ToolCall)
		}
	}
	if reasoning != "深度思考" {
		t.Fatalf("reasoning: got %q", reasoning)
	}
	if text != "Hi" {
		t.Fatalf("text: got %q", text)
	}
	if len(calls) != 1 || calls[0].ID != "tu_1" || calls[0].Name != "ping" || calls[0].Arguments != `{"a":1}` {
		t.Fatalf("calls: %+v", calls)
	}
}
