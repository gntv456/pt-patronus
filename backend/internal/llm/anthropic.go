package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// anthropicProvider 原生 Claude（/v1/messages，SSE）。
// 解析 text_delta、thinking_delta（深度思考）、tool_use 的 input_json_delta。
type anthropicProvider struct{ cfg Config }

func (p *anthropicProvider) ChatStream(ctx context.Context, req Req) (<-chan Event, error) {
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	maxTok := orI(req.MaxTokens, p.cfg.MaxTokens, 2048)
	bodyMap := map[string]any{
		"model":       firstNonEmpty(req.Model, p.cfg.Model, "claude-sonnet-5"),
		"max_tokens":  maxTok,
		"messages":    anMessages(req.Messages),
		"stream":      true,
		"temperature": orF(req.Temperature, p.cfg.Temperature, 0.3),
	}
	if sys := anSystem(req.Messages); sys != "" {
		bodyMap["system"] = sys
	}
	if len(req.Tools) > 0 {
		bodyMap["tools"] = anTools(req.Tools)
	}
	// 深度思考：thinking budget。max_tokens 必须 > budget。
	if req.EnableThinking || p.cfg.EnableThinking {
		budget := 8000
		if maxTok <= budget {
			bodyMap["max_tokens"] = budget + 2048
		}
		bodyMap["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
		// 思考模式下 Claude 禁用 temperature，移除避免 400。
		delete(bodyMap, "temperature")
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		// 上游错误体可能回显凭据片段/内网路径：仅落服务端日志，对外 error 只带状态码。
		log.Printf("llm/anthropic upstream error HTTP %d: %s", resp.StatusCode, truncate(string(b), 300))
		return nil, fmt.Errorf("anthropic upstream HTTP %d", resp.StatusCode)
	}

	out := make(chan Event, 32)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		// 按 content block index 记类型；tool_use 积累 input JSON。
		blockType := map[int]string{}
		toolMeta := map[int]struct{ id, name, args string }{}

		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			var ev map[string]any
			if json.Unmarshal([]byte(data), &ev) != nil {
				continue
			}
			switch ev["type"] {
			case "content_block_start":
				idx := intOf(ev["index"])
				cb, _ := ev["content_block"].(map[string]any)
				t, _ := cb["type"].(string)
				blockType[idx] = t
				if t == "tool_use" {
					m := toolMeta[idx]
					m.id, _ = cb["id"].(string)
					m.name, _ = cb["name"].(string)
					toolMeta[idx] = m
				}
			case "content_block_delta":
				idx := intOf(ev["index"])
				delta, _ := ev["delta"].(map[string]any)
				switch delta["type"] {
				case "text_delta":
					send(out, Event{Type: EventText, Text: strOf(delta["text"])})
				case "thinking_delta":
					send(out, Event{Type: EventReasoning, Text: strOf(delta["thinking"])})
				case "input_json_delta":
					if m, ok := toolMeta[idx]; ok {
						m.args += strOf(delta["partial_json"])
						toolMeta[idx] = m
					}
				}
			case "content_block_stop":
				idx := intOf(ev["index"])
				if blockType[idx] == "tool_use" {
					if m, ok := toolMeta[idx]; ok {
						send(out, Event{Type: EventToolCall, ToolCall: ToolCall{ID: m.id, Name: m.name, Arguments: m.args}})
					}
				}
			case "message_stop":
				send(out, Event{Type: EventDone})
				return
			}
		}
		if err := sc.Err(); err != nil {
			send(out, Event{Type: EventError, Error: err.Error()})
			return
		}
		send(out, Event{Type: EventDone})
	}()
	return out, nil
}

// ===== Anthropic 请求结构 =====

// anMessages 把统一消息转 Anthropic messages（system 抽离；连续 tool 结果合并为单条 user）。
func anMessages(msgs []Message) []map[string]any {
	out := []map[string]any{}
	for _, m := range msgs {
		if m.Role == RoleSystem {
			continue
		}
		switch m.Role {
		case RoleUser:
			out = append(out, map[string]any{"role": "user", "content": m.Content})
		case RoleAssistant:
			content := []map[string]any{}
			if m.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				content = append(content, map[string]any{"type": "tool_use", "id": tc.ID, "name": tc.Name, "input": parseInput(tc.Arguments)})
			}
			if len(content) == 0 {
				continue
			}
			out = append(out, map[string]any{"role": "assistant", "content": content})
		case RoleTool:
			// 工具结果：合并进最近一条 user 消息（若有且同为 tool_result），否则新建 user。
			block := map[string]any{"type": "tool_result", "tool_use_id": m.ToolCallID, "content": m.Content}
			if len(out) > 0 && out[len(out)-1]["role"] == "user" {
				if c, ok := out[len(out)-1]["content"].([]map[string]any); ok {
					out[len(out)-1]["content"] = append(c, block)
					continue
				}
			}
			out = append(out, map[string]any{"role": "user", "content": []map[string]any{block}})
		}
	}
	return out
}

func anSystem(msgs []Message) string {
	var parts []string
	for _, m := range msgs {
		if m.Role == RoleSystem && m.Content != "" {
			parts = append(parts, m.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func anTools(tools []Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{"name": t.Name, "description": t.Description, "input_schema": t.InputSchema})
	}
	return out
}

// parseInput 把 tool_use 的 arguments JSON 串解析为对象（Claude 要 input 是对象）。
func parseInput(args string) any {
	var obj map[string]any
	if json.Unmarshal([]byte(args), &obj) == nil {
		return obj
	}
	return map[string]any{}
}

func intOf(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}
func strOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
