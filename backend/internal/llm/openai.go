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
	"github.com/hxpt/ptpatronus/internal/core/safego"
)

// openaiProvider 兼容 OpenAI / DeepSeek / Qwen / Ollama / one-api 中转等。
// 解析 delta.content、delta.reasoning_content（推理模型）、增量 delta.tool_calls。
type openaiProvider struct{ cfg Config }

func (p *openaiProvider) ChatStream(ctx context.Context, req Req) (<-chan Event, error) {
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	body, err := json.Marshal(oaRequest{
		Model:         firstNonEmpty(req.Model, p.cfg.Model, "gpt-4o-mini"),
		Messages:      oaMessages(req.Messages),
		Tools:         oaTools(req.Tools),
		Temp:          orF(req.Temperature, p.cfg.Temperature, 0.3),
		MaxTok:        orI(req.MaxTokens, p.cfg.MaxTokens, 2048),
		Stream:        true,
		StreamOptions: oaStreamOptions{IncludeUsage: true}, // 末块带 usage 供统计
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 5 * time.Minute} // 流式长连接
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		// 上游错误体可能回显凭据片段/内网路径：仅落服务端日志，对外 error 只带状态码，
		// 避免经 agent.EventError 透传到前端 SSE / 外部网关客户端。
		log.Printf("llm/openai upstream error HTTP %d: %s", resp.StatusCode, truncate(string(b), 300))
		return nil, fmt.Errorf("openai upstream HTTP %d", resp.StatusCode)
	}

	out := make(chan Event, 32)
	safego.Go("openai:60", func() {
		defer resp.Body.Close()
		defer close(out)
		type tcAcc struct {
			id, name string
			args     strings.Builder
		}
		tools := map[int]*tcAcc{}
		var order []int

		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // 大 tool 结果增量
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			var chunk oaChunk
			if json.Unmarshal([]byte(data), &chunk) != nil {
				continue
			}
			for _, ch := range chunk.Choices {
				d := ch.Delta
				if d.ReasoningContent != "" {
					send(out, Event{Type: EventReasoning, Text: d.ReasoningContent})
				}
				if d.Content != "" {
					send(out, Event{Type: EventText, Text: d.Content})
				}
				for _, t := range d.ToolCalls {
					acc, ok := tools[t.Index]
					if !ok {
						acc = &tcAcc{}
						tools[t.Index] = acc
						order = append(order, t.Index)
					}
					if t.ID != "" {
						acc.id = t.ID
					}
					if t.Function.Name != "" {
						acc.name = t.Function.Name
					}
					if t.Function.Arguments != "" {
						acc.args.WriteString(t.Function.Arguments)
					}
				}
			}
			// include_usage 开启时，末块 choices 为空、带 usage（统计用，不下发前端）。
			if chunk.Usage != nil {
				send(out, Event{Type: EventUsage, Usage: &Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}})
			}
		}
		if err := sc.Err(); err != nil {
			send(out, Event{Type: EventError, Error: err.Error()})
			return
		}
		for _, i := range order {
			acc := tools[i]
			send(out, Event{Type: EventToolCall, ToolCall: ToolCall{ID: acc.id, Name: acc.name, Arguments: acc.args.String()}})
		}
		send(out, Event{Type: EventDone})
	})
	return out, nil
}

// ===== OpenAI 请求/响应结构 =====

type oaRequest struct {
	Model         string          `json:"model"`
	Messages      []oaMessage     `json:"messages"`
	Tools         []oaTool        `json:"tools,omitempty"`
	Temp          float64         `json:"temperature,omitempty"`
	MaxTok        int             `json:"max_tokens,omitempty"`
	Stream        bool            `json:"stream"`
	StreamOptions oaStreamOptions `json:"stream_options,omitempty"`
}

// oaStreamOptions 控制流式响应附加信息（include_usage 让末块带 token 用量）。
type oaStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content,omitempty"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type oaTool struct {
	Type     string   `json:"type"`
	Function oaToolFn `json:"function"`
}
type oaToolFn struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type oaToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function oaToolCallFn `json:"function"`
}
type oaToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *oaUsage `json:"usage"`
}

// oaUsage 仅 include_usage 开启时在末块出现。
type oaUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func oaMessages(msgs []Message) []oaMessage {
	out := make([]oaMessage, 0, len(msgs))
	for _, m := range msgs {
		om := oaMessage{Role: string(m.Role), Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, oaToolCall{ID: tc.ID, Type: "function", Function: oaToolCallFn{Name: tc.Name, Arguments: tc.Arguments}})
		}
		out = append(out, om)
	}
	return out
}

func oaTools(tools []Tool) []oaTool {
	out := make([]oaTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, oaTool{Type: "function", Function: oaToolFn{Name: t.Name, Description: t.Description, Parameters: t.InputSchema}})
	}
	return out
}

// ===== 共享小工具 =====

func send(ch chan<- Event, e Event) {
	select {
	case ch <- e:
	case <-time.After(30 * time.Second): // 消费方卡死保护
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func orF(vals ...float64) float64 { // 第一个 > 0
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}
func orI(vals ...int) int {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
