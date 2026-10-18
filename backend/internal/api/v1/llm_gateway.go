package v1

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/aiagent"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
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
	semMu      sync.Mutex
	sem        chan struct{} // 按 MaxConcurrency 限制的并发信号量
	sessionsMu sync.Mutex
	sessions   map[string]sessionEntry // x-session-id → convID（多轮对话复用同一会话）
}

// NewLLMGatewayHandler 构造。convCreate 由 app.go 绑 oper.AgentConversationOper.Create。
func NewLLMGatewayHandler(agent *aiagent.Agent,
	convCreate func(*models.AgentConversation) error,
	kv llmgateway.KVStore, crypto llmgateway.Crypto,
) *LLMGatewayHandler {
	return &LLMGatewayHandler{agent: agent, convCreate: convCreate, kv: kv, crypto: crypto,
		sessions: map[string]sessionEntry{}}
}

// proxyMessage 兼容 OpenAI / Anthropic 的 messages[].content（string 或多模态数组）。
type proxyMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openAIReq struct {
	Model    string         `json:"model"`
	Messages []proxyMessage `json:"messages"`
	Stream   bool           `json:"stream"`
}

type anthropicReq struct {
	Model    string         `json:"model"`
	System   any            `json:"system"`
	Messages []proxyMessage `json:"messages"`
	Stream   bool           `json:"stream"`
}

// tokenFrom 取访问令牌：优先 Authorization: Bearer，其次 x-api-key（Anthropic 习惯）。
func (h *LLMGatewayHandler) tokenFrom(c *gin.Context) string {
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return strings.TrimSpace(c.GetHeader("x-api-key"))
}

// authorize 校验启用状态 + 访问令牌（常量时间比较防时序侧信道）。成功返回配置供后续 ModelAllowed 用。
func (h *LLMGatewayHandler) authorize(c *gin.Context) (llmgateway.Config, bool) {
	cfg := llmgateway.Load(h.kv)
	if !cfg.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "LLM gateway disabled", "type": "server_error"}})
		return cfg, false
	}
	want := llmgateway.AccessToken(cfg, h.crypto)
	tok := h.tokenFrom(c)
	// 常量时间比较：token 可驱动全部工具（含写操作），需防逐字节时序还原。
	if want == "" || tok == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(want)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "invalid access token", "type": "authentication_error"}})
		return cfg, false
	}
	return cfg, true
}

// semFor 返回按当前 MaxConcurrency 大小的并发信号量；配置变更时按新容量重建
// （在途请求仍释放到旧 chan 并被 GC，仅配置切换瞬态可能短暂越界，可接受）。
func (h *LLMGatewayHandler) semFor(n int) chan struct{} {
	if n <= 0 {
		n = 4
	}
	h.semMu.Lock()
	defer h.semMu.Unlock()
	if h.sem == nil || cap(h.sem) != n {
		h.sem = make(chan struct{}, n)
	}
	return h.sem
}

// agentReady 预检底层 AI Agent 是否已配 provider 与 key，避免「200 已流式再报错」。
func (h *LLMGatewayHandler) agentReady(c *gin.Context) bool {
	ac, _, err := aiagent.Load(h.kv, h.crypto)
	if errors.Is(err, aiagent.ErrConfigUnreadable) {
		// 读失败时 ac 是默认值（Enabled=false、没有 key），照下面的判定走就会对外宣告
		// 「AI agent not enabled」——第三方客户端以为是自己的接入没配好，真实故障却在服务端。
		slog.Warn("llm gateway: agent config unreadable", "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"message": aiagent.ErrConfigUnreadable.Error() + "（服务端读不出 AI 助手配置，稍后重试或检查系统设置库）",
			"type":    "server_error",
		}})
		return false
	}
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
	cfg, ok := h.authorize(c)
	if !ok || !h.agentReady(c) {
		return
	}
	var in openAIReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		return
	}
	if !cfg.ModelAllowed(in.Model) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "model not allowed: " + in.Model, "type": "invalid_request_error"}})
		return
	}
	userText := lastUserText(in.Messages)
	if strings.TrimSpace(userText) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "no user message", "type": "invalid_request_error"}})
		return
	}
	// 并发限流：超 MaxConcurrency 返 429，防外部客户端并发刷爆底层 LLM 额度 / 拖垮进程。
	sem := h.semFor(cfg.MaxConcurrency)
	select {
	case sem <- struct{}{}:
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"message": "gateway at max concurrency", "type": "rate_limit_error"}})
		return
	}
	defer func() { <-sem }()

	convID, err := h.convForSession(c.GetHeader("x-session-id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "internal error", "type": "server_error"}})
		return
	}
	model := in.Model
	if strings.TrimSpace(model) == "" {
		model = "ptpatronus-agent"
	}
	h.streamSSE(c)
	logStreamFailure("openai", convID, llmgateway.StreamOpenAI(c.Writer, model, h.agent.Run(c.Request.Context(), convID, userText)))
}

// AnthropicMessages POST /anthropic/v1/messages —— Anthropic 兼容流式。
func (h *LLMGatewayHandler) AnthropicMessages(c *gin.Context) {
	cfg, ok := h.authorize(c)
	if !ok || !h.agentReady(c) {
		return
	}
	var in anthropicReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		return
	}
	if !cfg.ModelAllowed(in.Model) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "model not allowed: " + in.Model, "type": "invalid_request_error"}})
		return
	}
	userText := lastUserText(in.Messages)
	if strings.TrimSpace(userText) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "no user message", "type": "invalid_request_error"}})
		return
	}
	sem := h.semFor(cfg.MaxConcurrency)
	select {
	case sem <- struct{}{}:
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"message": "gateway at max concurrency", "type": "rate_limit_error"}})
		return
	}
	defer func() { <-sem }()

	convID, err := h.convForSession(c.GetHeader("x-session-id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "internal error", "type": "server_error"}})
		return
	}
	model := in.Model
	if strings.TrimSpace(model) == "" {
		model = "ptpatronus-agent"
	}
	h.streamSSE(c)
	logStreamFailure("anthropic", convID, llmgateway.StreamAnthropic(c.Writer, model, h.agent.Run(c.Request.Context(), convID, userText)))
}

// newConversation 建空会话。多轮复用见 convForSession（按 x-session-id 复用同一会话，
// agent.Run 会按 convID 用 ListByConv 加载历史消息，实现跨请求上下文连续）。
func (h *LLMGatewayHandler) newConversation() (*models.AgentConversation, error) {
	conv := &models.AgentConversation{}
	if err := h.convCreate(conv); err != nil {
		return nil, err
	}
	return conv, nil
}

// sessionTTL 是多轮会话的最长空闲保留时长；超过则该 session 重建会话（防无限堆积）。
const sessionTTL = 30 * time.Minute

// sessionEntry 记录 x-session-id → 会话 ID 与最后活跃时间（TTL 过期重建）。
type sessionEntry struct {
	convID uint
	last   time.Time
}

// convForSession 多轮对话核心：同 x-session-id 在 TTL 内复用同一 convID（agent.Run 按 convID
// 加载历史），无 session 头则每次建新会话（兼容旧行为）。替代了原「每请求单轮」的局限。
func (h *LLMGatewayHandler) convForSession(sessionID string) (uint, error) {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		conv, err := h.newConversation()
		if err != nil {
			return 0, err
		}
		return conv.ID, nil
	}
	h.sessionsMu.Lock()
	defer h.sessionsMu.Unlock()
	if h.sessions == nil {
		h.sessions = map[string]sessionEntry{}
	}
	now := time.Now()
	if e, ok := h.sessions[sid]; ok && now.Sub(e.last) < sessionTTL {
		h.sessions[sid] = sessionEntry{convID: e.convID, last: now}
		return e.convID, nil
	}
	conv, err := h.newConversation()
	if err != nil {
		return 0, err
	}
	h.sessions[sid] = sessionEntry{convID: conv.ID, last: now}
	return conv.ID, nil
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

// logStreamFailure 留一条网关失败痕迹。响应已经流式发出，状态码改不了，
// 日志是这条出口唯一还能说话的地方（从前两处 `_ = Stream…` 把它整个丢掉 ⇒
// 运维侧看不到「谁的哪一次请求失败了」，只能听用户说「模型不回话」）。
func logStreamFailure(api string, convID uint, err error) {
	if err != nil {
		slog.Warn("llm gateway stream failed", "api", api, "conversation_id", convID, "err", err)
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	cfg := llmgateway.Load(h.kv)
	cfg.Enabled = in.Enabled
	cfg.AllowedModels = in.AllowedModels
	cfg.MaxConcurrency = in.MaxConcurrency
	if strings.TrimSpace(in.AccessToken) != "" {
		enc, err := h.crypto.Encrypt(strings.TrimSpace(in.AccessToken))
		if err != nil {
			errcode.Respond(c, errcode.Internal.WithErr(err))
			return
		}
		cfg.AccessTokenEnc = enc
	}
	if err := llmgateway.Save(h.kv, cfg); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, llmgateway.Redacted(cfg))
}
