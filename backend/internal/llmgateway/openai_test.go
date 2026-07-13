package llmgateway

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/llm"
)

func TestStreamOpenAI(t *testing.T) {
	events := make(chan llm.Event, 8)
	events <- llm.Event{Type: llm.EventText, Text: "Hello "}
	events <- llm.Event{Type: llm.EventText, Text: "world"}
	events <- llm.Event{Type: llm.EventReasoning, Text: "think"}
	events <- llm.Event{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "call_1", Name: "search", Arguments: `{"q":"x"}`}}
	events <- llm.Event{Type: llm.EventDone}
	close(events)

	rec := httptest.NewRecorder()
	if err := StreamOpenAI(rec, "ptpatronus-agent", events); err != nil {
		t.Fatalf("StreamOpenAI: %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"object":"chat.completion.chunk"`,
		`"content":"Hello "`,
		`"content":"world"`,
		`"reasoning_content":"think"`,
		`"tool_calls"`,
		`"id":"call_1"`,
		`"index":0`,
		`"name":"search"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s\nbody: %s", want, body)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]") {
		t.Errorf("missing [DONE] terminator; tail=%q", body[len(body)-40:])
	}
}

func TestStreamOpenAIErrorTerminates(t *testing.T) {
	events := make(chan llm.Event, 2)
	events <- llm.Event{Type: llm.EventError, Error: "boom"}
	close(events)
	rec := httptest.NewRecorder()
	err := StreamOpenAI(rec, "m", events)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("want error boom, got %v", err)
	}
	if !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Error("error path must still emit [DONE]")
	}
}
