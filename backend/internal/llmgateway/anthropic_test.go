// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package llmgateway

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/llm"
)

func TestStreamAnthropic(t *testing.T) {
	events := make(chan llm.Event, 8)
	events <- llm.Event{Type: llm.EventText, Text: "Hi"}
	events <- llm.Event{Type: llm.EventReasoning, Text: "hmm"}
	events <- llm.Event{Type: llm.EventToolCall, ToolCall: llm.ToolCall{ID: "tu1", Name: "do", Arguments: `{}`}}
	events <- llm.Event{Type: llm.EventDone}
	close(events)

	rec := httptest.NewRecorder()
	if err := StreamAnthropic(rec, "ptpatronus-agent", events); err != nil {
		t.Fatalf("StreamAnthropic: %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: message_start",
		"event: content_block_start",
		`"type":"text_delta"`,
		`"type":"thinking_delta"`,
		`"type":"tool_use"`,
		`"type":"input_json_delta"`,
		"event: content_block_stop",
		"event: message_delta",
		`"stop_reason":"end_turn"`,
		"event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s\nbody: %s", want, body)
		}
	}
}

func TestStreamAnthropicErrorTerminates(t *testing.T) {
	events := make(chan llm.Event, 2)
	events <- llm.Event{Type: llm.EventError, Error: "boom"}
	close(events)
	rec := httptest.NewRecorder()
	if err := StreamAnthropic(rec, "m", events); err == nil || err.Error() != "boom" {
		t.Fatalf("want error boom, got %v", err)
	}
	if !strings.Contains(rec.Body.String(), "event: message_stop") {
		t.Error("error path must still emit message_stop")
	}
}
