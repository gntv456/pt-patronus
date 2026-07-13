package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/aiagent"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/llmgateway"
)

// LLMGatewayHandler 把 AI Agent 以 OpenAI / Anthropic 兼容协议对外暴露。
// 进站鉴权 = Bearer/x-api-key 头比对 SystemConfig 中加密的访问令牌（外部客户端不能走 JWT）。
type LLMGatewayHandler struct {
	agent      *aiagent.Agent
	convCreate func(*models.AgentConversation) error
	kv         llmgateway.KVStore
	crypto     llmgateway.Crypto
}

// NewLLMGatewayHandler 构造。convCreate 由 app.go 绑 oper.AgentConversationOper.Create。
func NewLLMGatewayHandler(agent *aiagent.Agent,
	convCreate func(*models.AgentConversation) error,
	kv llmgateway.KVStore, crypto llmgateway.Crypto,
) *LLMGatewayHandler {
	return &LLMGatewayHandler{agent: agent, convCreate: convCreate, kv: kv, crypto: crypto}
}

// proxyMessage 兼容 OpenAI / Anthropic 的 messages[].content（string 或多模态数组）。
type proxyMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openAIReq struct {
	Model    string          `json:"model"`
	Messages []proxyMessage  `json:"messages"`
	Stream   bool            `json:"stream"`
}

type anthropicReq struct {
	Model    string          `json:"model"`
	System   any             `json:"system"`
	Messages []proxyMessage  `json:"messages"`
	Stream   bool            `json:"stream"`
}

// tokenFrom 取访问令牌：优先 Authorization: Bearer，其次 x-api-key（Anthropic 习惯）。
func (h *LLMGatewayHandler) tokenFrom(c *gin.Context) string {
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return strings.TrimSpace(c.GetHeader("x-api-key"))
}

// authorize 校验启用状态 + 访问令牌。失败已写 JSON 响应，返 false。
func (h *LLMGatewayHandler) authorize(c *gin.Context) bool {
	cfg := llmgateway.Load(h.kv)
	if !cfg.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "LLM gateway disabled", "type": "server_error"}})
		return false
	}
	want := llmgateway.AccessToken(cfg, h.crypto)
	tok := h.tokenFrom(c)
	if want == "" || tok == "" || tok != want {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "invalid access token", "type": "authentication_error"}})
		return false
	}
	return true
}

// agentReady 预检底层 AI Agent 是否已配 provider 与 key，避免「200 已流式再报错」。
func (h *LLMGatewayHandler) agentReady(c *gin.Context) bool {
	ac, _ := aiagent.Load(h.kv, h.crypto)
	if !ac.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "AI agent not enabled", "type": "server_error"}})
		return false
	}
	if strings.TrimSpace(ac.APIKeyEnc) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "AI agent has no API key", "type": "server_error"}})
		return false
	}
	return true
}

// OpenAIChat POST /openai/v1/chat/completions —— OpenAI 兼容流式。
func (h *LLMGatewayHandler) OpenAIChat(c *gin.Context) {
	if !h.authorize(c) || !h.agentReady(c) {
		return
	}
	var in openAIReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		return
	}
	userText := lastUserText(in.Messages)
	if strings.TrimSpace(userText) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "no user message", "type": "invalid_request_error"}})
		return
	}
	conv, err := h.newConversation()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error(), "type": "server_error"}})
		return
	}
	model := in.Model
	if strings.TrimSpace(model) == "" {
		model = "ptpatronus-agent"
	}
	h.streamSSE(c)
	_ = llmgateway.StreamOpenAI(c.Writer, model, h.agent.Run(c.Request.Context(), conv.ID, userText))
}

// AnthropicMessages POST /anthropic/v1/messages —— Anthropic 兼容流式。
func (h *LLMGatewayHandler) AnthropicMessages(c *gin.Context) {
	if !h.authorize(c) || !h.agentReady(c) {
		return
	}
	var in anthropicReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		return
	}
	userText := lastUserText(in.Messages)
	if strings.TrimSpace(userText) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "no user message", "type": "invalid_request_error"}})
		return
	}
	conv, err := h.newConversation()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error(), "type": "server_error"}})
		return
	}
	model := in.Model
	if strings.TrimSpace(model) == "" {
		model = "ptpatronus-agent"
	}
	h.streamSSE(c)
	_ = llmgateway.StreamAnthropic(c.Writer, model, h.agent.Run(c.Request.Context(), conv.ID, userText))
}

// newConversation 建空会话（MVP 每请求单轮；多轮 TODO：按 x-session-id 复用）。
func (h *LLMGatewayHandler) newConversation() (*models.AgentConversation, error) {
	conv := &models.AgentConversation{}
	if err := h.convCreate(conv); err != nil {
		return nil, err
	}
	return conv, nil
}

// streamSSE 写 SSE 响应头并立即 flush（复用 agent_chat.go 的范式）。
func (h *LLMGatewayHandler) streamSSE(c *gin.Context) {
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 关 Nginx 缓冲
	w.WriteHeader(http.StatusOK)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// lastUserText 取最后一条 role=user 的文本（content 可能是 string 或多模态数组）。
func lastUserText(msgs []proxyMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "user" {
			continue
		}
		return contentToText(msgs[i].Content)
	}
	return ""
}

func contentToText(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		var b strings.Builder
		for _, part := range c {
			if m, ok := part.(map[string]any); ok {
				if t, _ := m["type"].(string); t == "text" {
					if s, _ := m["text"].(string); s != "" {
						b.WriteString(s)
					}
				}
			}
		}
		return b.String()
	}
	return ""
}

// llmGatewayConfigReq 配置更新请求体（access_token 留空=不改）。
type llmGatewayConfigReq struct {
	Enabled        bool     `json:"enabled"`
	AccessToken    string   `json:"access_token"`
	AllowedModels  []string `json:"allowed_models"`
	MaxConcurrency int      `json:"max_concurrency"`
}

// GetConfig 读网关配置（脱敏）。
func (h *LLMGatewayHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, llmgateway.Redacted(llmgateway.Load(h.kv)))
}

// SetConfig 写网关配置。access_token 非空才加密覆盖。
func (h *LLMGatewayHandler) SetConfig(c *gin.Context) {
	var in llmGatewayConfigReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg := llmgateway.Load(h.kv)
	cfg.Enabled = in.Enabled
	cfg.AllowedModels = in.AllowedModels
	cfg.MaxConcurrency = in.MaxConcurrency
	if strings.TrimSpace(in.AccessToken) != "" {
		enc, err := h.crypto.Encrypt(strings.TrimSpace(in.AccessToken))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		cfg.AccessTokenEnc = enc
	}
	if err := llmgateway.Save(h.kv, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, llmgateway.Redacted(cfg))
}
