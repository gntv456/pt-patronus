package llmgateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hxpt/ptpatronus/internal/llm"
)

const oaChatID = "chatcmpl-ptpatronus"

// StreamOpenAI 消费 agent 事件流，按 OpenAI chat.completion.chunk SSE 写出。
// model 原样回显（兼容客户端 model 字段）。返回首个错误（正常 done 返 nil）。
//
// 事件映射：EventText→delta.content；EventReasoning→delta.reasoning_content；
// EventToolCall→delta.tool_calls[index]（一次完整调用一个 chunk，index 递增）；
// EventToolResult 跳过（OpenAI 协议里工具结果属下一轮请求）；EventDone→finish_reason=stop + [DONE]。
func StreamOpenAI(w http.ResponseWriter, model string, events <-chan llm.Event) error {
	flusher, _ := w.(http.Flusher)
	created := time.Now().Unix()
	tcIdx := 0 // tool_calls 增量下标

	writeChunk := func(delta map[string]any, finish *string) error {
		choice := map[string]any{"index": 0, "delta": delta}
		if finish != nil {
			choice["finish_reason"] = *finish
		} else {
			choice["finish_reason"] = nil
		}
		chunk := map[string]any{
			"id":      oaChatID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []map[string]any{choice},
		}
		b, _ := json.Marshal(chunk)
		// 客户端断开后写失败：立即停止消费事件流，避免驱动 agent 跑完全部迭代（持续计费）。
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}
	finish := func() error {
		stop := "stop"
		if err := writeChunk(map[string]any{}, &stop); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: [DONE]\n\n"); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}

	for ev := range events {
		switch ev.Type {
		case llm.EventText:
			if ev.Text != "" {
				if err := writeChunk(map[string]any{"content": ev.Text}, nil); err != nil {
					return nil // 客户端断开：非服务端错误；agent 由 ctx 取消自行停止
				}
			}
		case llm.EventReasoning:
			if ev.Text != "" {
				if err := writeChunk(map[string]any{"reasoning_content": ev.Text}, nil); err != nil {
					return nil
				}
			}
		case llm.EventToolCall:
			tc := map[string]any{
				"index": tcIdx, "id": ev.ToolCall.ID, "type": "function",
				"function": map[string]any{"name": ev.ToolCall.Name, "arguments": ev.ToolCall.Arguments},
			}
			tcIdx++
			if err := writeChunk(map[string]any{"tool_calls": []map[string]any{tc}}, nil); err != nil {
				return nil
			}
		case llm.EventToolResult:
			// 工具结果不回传客户端（属下一轮请求）。
		case llm.EventError:
			// 错误作为最后一段 content 写出；此时客户端多半还在，写失败也无碍（本来就要返回）。
			writeChunk(map[string]any{"content": ev.Error}, nil)
			finish()
			return fmt.Errorf("%s", ev.Error)
		case llm.EventDone:
			finish()
			return nil
		}
	}
	finish() // 流关闭但无 done：正常收尾
	return nil
}
