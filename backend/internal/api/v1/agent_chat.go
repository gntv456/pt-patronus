package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/aiagent"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/llm"
)

// AgentChatHandler 处理 AI 助手的 SSE 对话 + 会话历史 + provider 配置。
type AgentChatHandler struct {
	agent     *aiagent.Agent
	convList  func() ([]models.AgentConversation, error)
	convCreate func(*models.AgentConversation) error
	convDelete func(uint) error
	msgList   func(uint) ([]models.AgentMessage, error)
	kv        aiagent.KVStore
	crypto    aiagent.Crypto
}

// NewAgentChatHandler 构造。conv/msg 回调由 app.go 绑 oper。
func NewAgentChatHandler(agent *aiagent.Agent,
	convList func() ([]models.AgentConversation, error),
	convCreate func(*models.AgentConversation) error,
	convDelete func(uint) error,
	msgList func(uint) ([]models.AgentMessage, error),
	kv aiagent.KVStore, crypto aiagent.Crypto,
) *AgentChatHandler {
	return &AgentChatHandler{agent: agent, convList: convList, convCreate: convCreate, convDelete: convDelete, msgList: msgList, kv: kv, crypto: crypto}
}

type agentChatReq struct {
	ConversationID uint   `json:"conversation_id"`
	Message        string `json:"message" binding:"max=8000"`
}

// Chat SSE：流式返回 token/reasoning/tool_call/tool_result/done/error。
func (h *AgentChatHandler) Chat(c *gin.Context) {
	var in agentChatReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(in.Message) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message required"})
		return
	}
	convID := in.ConversationID
	if convID == 0 {
		conv := &models.AgentConversation{}
		if err := h.convCreate(conv); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		convID = conv.ID
	}

	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 关 Nginx 缓冲，token 即时到前端
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	// 先回会话 id（新建时前端据此记录）
	if !writeSSE(w, "conversation", map[string]any{"id": convID}) {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}

	stream := h.agent.Run(c.Request.Context(), convID, in.Message)
	for ev := range stream {
		var event string
		var data any
		switch ev.Type {
		case llm.EventText:
			event, data = "token", map[string]any{"content": ev.Text}
		case llm.EventReasoning:
			event, data = "reasoning", map[string]any{"content": ev.Text}
		case llm.EventToolCall:
			event, data = "tool_call", map[string]any{"id": ev.ToolCall.ID, "name": ev.ToolCall.Name, "arguments": ev.ToolCall.Arguments}
		case llm.EventToolResult:
			event, data = "tool_result", map[string]any{"id": ev.ToolCall.ID, "name": ev.ToolCall.Name, "result": ev.Text}
		case llm.EventDone:
			event, data = "done", map[string]any{}
		case llm.EventError:
			event, data = "error", map[string]any{"message": ev.Error}
		default:
			continue
		}
		if !writeSSE(w, event, data) {
			return // 客户端断开
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// writeSSE 写一条 SSE 事件；写失败（客户端断开）返回 false。
func writeSSE(w http.ResponseWriter, event string, data any) bool {
	b, err := json.Marshal(data)
	if err != nil {
		return true
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b)); err != nil {
		return false
	}
	return true
}

// Conversations 列会话。
func (h *AgentChatHandler) Conversations(c *gin.Context) {
	cs, err := h.convList()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conversations": cs})
}

// Messages 单会话消息（含 reasoning/tool_calls）。
func (h *AgentChatHandler) Messages(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ms, err := h.msgList(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": ms})
}

// DeleteConversation 删会话。
func (h *AgentChatHandler) DeleteConversation(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.convDelete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// agentConfigReq 配置更新请求体（api_key 留空=不改）。
type agentConfigReq struct {
	Enabled        bool     `json:"enabled"`
	Provider       string   `json:"provider"`
	BaseURL        string   `json:"base_url"`
	APIKey         string   `json:"api_key"` // 明文（仅写入时；GET 不返）
	Model          string   `json:"model"`
	Temperature    float64  `json:"temperature"`
	MaxTokens      int      `json:"max_tokens"`
	EnableThinking bool     `json:"enable_thinking"`
	MaxIterations  int      `json:"max_iterations"`
	SystemPrompt   string   `json:"system_prompt"`
	EnabledTools   []string `json:"enabled_tools"`
}

// GetConfig 读 provider 配置（脱敏，api_key 仅留 _configured）。
func (h *AgentChatHandler) GetConfig(c *gin.Context) {
	ac, _ := aiagent.Load(h.kv, h.crypto)
	c.JSON(http.StatusOK, aiagent.Redacted(ac))
}

// SetConfig 写 provider 配置。api_key 非空才加密覆盖。
func (h *AgentChatHandler) SetConfig(c *gin.Context) {
	var in agentConfigReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing, _ := aiagent.Load(h.kv, h.crypto)
	cfg := existing
	cfg.Enabled = in.Enabled
	cfg.Provider = in.Provider
	cfg.BaseURL = in.BaseURL
	cfg.Model = in.Model
	cfg.Temperature = in.Temperature
	cfg.MaxTokens = in.MaxTokens
	cfg.EnableThinking = in.EnableThinking
	cfg.MaxIterations = in.MaxIterations
	cfg.SystemPrompt = in.SystemPrompt
	cfg.EnabledTools = in.EnabledTools
	if strings.TrimSpace(in.APIKey) != "" {
		enc, err := h.crypto.Encrypt(strings.TrimSpace(in.APIKey))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		cfg.APIKeyEnc = enc
	}
	if err := aiagent.SaveConfig(h.kv, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, aiagent.Redacted(cfg))
}
