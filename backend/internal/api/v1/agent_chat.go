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
	agent      *aiagent.Agent
	convList   func() ([]models.AgentConversation, error)
	convCreate func(*models.AgentConversation) error
	convDelete func(uint) error
	msgList    func(uint) ([]models.AgentMessage, error)
	kv         aiagent.KVStore
	crypto     aiagent.Crypto
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
		case llm.EventConfirmRequest:
			token, risk, summary := "", "", ""
			if ev.Confirm != nil {
				token, risk, summary = ev.Confirm.Token, ev.Confirm.Risk, ev.Confirm.Summary
			}
			event, data = "confirm_request", map[string]any{"id": ev.ToolCall.ID, "name": ev.ToolCall.Name, "arguments": ev.ToolCall.Arguments, "token": token, "risk": risk, "summary": summary}
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

// agentConfirmReq 危险工具确认决断请求体。
type agentConfirmReq struct {
	ToolCallID string `json:"tool_call_id" binding:"required"`
	Approved   bool   `json:"approved"`
}

// Confirm 送达一次危险工具确认决断（用户在 confirm_request 弹窗选择后调用）。
// token = convID:toolCallID；无在途确认返回 404。
func (h *AgentChatHandler) Confirm(c *gin.Context) {
	convID, ok := parseID(c)
	if !ok {
		return
	}
	var in agentConfirmReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token := fmt.Sprintf("%d:%s", convID, in.ToolCallID)
	if !h.agent.Resolve(token, in.Approved) {
		c.JSON(http.StatusNotFound, gin.H{"error": "无在途确认（已过期、已取消或已处理）"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"resolved": true, "approved": in.Approved})
}

// Tools 返回全部工具目录（含 read/write/destructive 风险等级），供白名单 UI 分组勾选。
func (h *AgentChatHandler) Tools(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"tools": h.agent.ToolCatalog()})
}

// Stats 返回用量统计快照（请求数/Token/费用/延迟/错误/429/工具成功率）。
func (h *AgentChatHandler) Stats(c *gin.Context) {
	c.JSON(http.StatusOK, h.agent.Stats())
}

// agentConfigReq 配置更新请求体（api_key 留空=不改）。
type agentConfigReq struct {
	Enabled        bool     `json:"enabled"`
	Provider       string   `json:"provider" binding:"required,oneof=openai anthropic"`
	BaseURL        string   `json:"base_url" binding:"required,url,max=2048"`
	APIKey         string   `json:"api_key" binding:"max=8192"` // 明文（仅写入时；GET 不返）
	Model          string   `json:"model" binding:"required,max=256"`
	Temperature    float64  `json:"temperature" binding:"min=0,max=2"`
	MaxTokens      int      `json:"max_tokens" binding:"min=1,max=200000"`
	EnableThinking bool     `json:"enable_thinking"`
	MaxIterations  int      `json:"max_iterations" binding:"min=1,max=30"`
	SystemPrompt   string   `json:"system_prompt" binding:"required,max=20000"`
	EnabledTools   []string `json:"enabled_tools" binding:"max=512,dive,required,max=128"`
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
