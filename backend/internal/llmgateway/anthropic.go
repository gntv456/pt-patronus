package llmgateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hxpt/ptpatronus/internal/llm"
)

// StreamAnthropic 消费 agent 事件流，按 Anthropic messages SSE 写出。
// event 序列：message_start →（text/thinking/tool_use 内容块）→ message_delta(end_turn) → message_stop。
//
// 事件映射：EventText→text_delta；EventReasoning→thinking_delta；EventToolCall→
// tool_use 内容块（content_block_start + input_json_delta 全量 + content_block_stop）；
// EventToolResult 跳过；EventDone→收尾。
func StreamAnthropic(w http.ResponseWriter, model string, events <-chan llm.Event) error {
	flusher, _ := w.(http.Flusher)
	writeEv := func(evType string, payload map[string]any) error {
		payload["type"] = evType
		b, _ := json.Marshal(payload)
		// 客户端断开后写失败：返回 error 让主循环停止消费事件流。
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evType, b); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}

	if err := writeEv("message_start", map[string]any{
		"message": map[string]any{
			"id": oaChatID, "type": "message", "role": "assistant",
			"model": model, "content": []any{}, "stop_reason": nil,
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	}); err != nil {
		return nil // 客户端在首事件前就断开
	}

	const (
		blkNone = iota
		blkText
		blkThinking
	)
	curBlock := blkNone
	curIdx := -1
	nextIdx := 0

	openBlock := func(kind string) {
		curIdx = nextIdx
		nextIdx++
		cb := map[string]any{"type": kind}
		switch kind {
		case "text":
			cb["text"] = ""
		case "thinking":
			cb["thinking"] = ""
		}
		_ = writeEv("content_block_start", map[string]any{"index": curIdx, "content_block": cb})
	}
	closeBlock := func() {
		if curBlock != blkNone {
			_ = writeEv("content_block_stop", map[string]any{"index": curIdx})
			curBlock = blkNone
		}
	}
	end := func() {
		closeBlock()
		_ = writeEv("message_delta", map[string]any{
			"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
			"usage": map[string]any{"output_tokens": 0},
		})
		_ = writeEv("message_stop", map[string]any{})
	}

	for ev := range events {
		switch ev.Type {
		case llm.EventText:
			if ev.Text == "" {
				continue
			}
			if curBlock != blkText {
				closeBlock()
				openBlock("text")
				curBlock = blkText
			}
			if err := writeEv("content_block_delta", map[string]any{
				"index": curIdx,
				"delta": map[string]any{"type": "text_delta", "text": ev.Text},
			}); err != nil {
				return nil // 客户端断开：非服务端错误；agent 由 ctx 取消自行停止
			}
		case llm.EventReasoning:
			if ev.Text == "" {
				continue
			}
			if curBlock != blkThinking {
				closeBlock()
				openBlock("thinking")
				curBlock = blkThinking
			}
			if err := writeEv("content_block_delta", map[string]any{
				"index": curIdx,
				"delta": map[string]any{"type": "thinking_delta", "thinking": ev.Text},
			}); err != nil {
				return nil
			}
		case llm.EventToolCall:
			closeBlock()
			idx := nextIdx
			nextIdx++
			if err := writeEv("content_block_start", map[string]any{
				"index": idx,
				"content_block": map[string]any{
					"type": "tool_use", "id": ev.ToolCall.ID, "name": ev.ToolCall.Name, "input": map[string]any{},
				},
			}); err != nil {
				return nil
			}
			_ = writeEv("content_block_delta", map[string]any{
				"index": idx,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": ev.ToolCall.Arguments},
			})
			_ = writeEv("content_block_stop", map[string]any{"index": idx})
		case llm.EventToolResult:
			// 工具结果属下一轮；跳过。
		case llm.EventError:
			end()
			return fmt.Errorf("%s", ev.Error)
		case llm.EventDone:
			end()
			return nil
		}
	}
	end() // 流关闭无 done：正常收尾
	return nil
}
