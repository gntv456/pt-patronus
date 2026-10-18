package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/aiagent"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/llm"
)

// AgentChatHandler 处理 AI 助手的 SSE 对话 + 会话历史 + provider 配置。
type AgentChatHandler struct {
	agent      *aiagent.Agent
	convList   func() ([]models.AgentConversation, error)
	convCount  func() (int64, error)
	convCreate func(*models.AgentConversation) error
	convDelete func(uint) error
	convGet    func(uint) (*models.AgentConversation, error)
	msgList    func(uint) ([]models.AgentMessage, error)
	kv         aiagent.KVStore
	crypto     aiagent.Crypto
}

// NewAgentChatHandler 构造。conv/msg 回调由 app.go 绑 oper。
func NewAgentChatHandler(agent *aiagent.Agent,
	convList func() ([]models.AgentConversation, error),
	convCount func() (int64, error),
	convCreate func(*models.AgentConversation) error,
	convDelete func(uint) error,
	convGet func(uint) (*models.AgentConversation, error),
	msgList func(uint) ([]models.AgentMessage, error),
	kv aiagent.KVStore, crypto aiagent.Crypto,
) *AgentChatHandler {
	return &AgentChatHandler{agent: agent, convList: convList, convCount: convCount, convCreate: convCreate, convDelete: convDelete, convGet: convGet, msgList: msgList, kv: kv, crypto: crypto}
}

// errorSafeCount 让缺注入的计数口以错误收场（fail-closed），而不是报 0。
func errorSafeCount(f func() (int64, error)) (int64, error) {
	if f == nil {
		return 0, errNoConvCounter
	}
	return f()
}

var errNoConvCounter = errors.New("conversation counter unavailable")

type agentChatReq struct {
	ConversationID uint   `json:"conversation_id"`
	Message        string `json:"message" binding:"max=8000"`
}

// Chat SSE：流式返回 token/reasoning/tool_call/tool_result/done/error。
func (h *AgentChatHandler) Chat(c *gin.Context) {
	var in agentChatReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if strings.TrimSpace(in.Message) == "" {
		errcode.Respond(c, errcode.AgentMsgRequired)
		return
	}
	// 建会话之前先预检配置：否则每点一次「发送」都会先落一条空会话、再在流里报
	// 「未启用」，用户什么也没得到却在会话列表里攒了垃圾条目。
	switch h.agent.Ready() {
	case aiagent.ReadyDisabled:
		errcode.Respond(c, errcode.AgentDisabled)
		return
	case aiagent.ReadyConfigUnreadable:
		// 判在 Disabled 之前才有意义：读失败时默认值的 Enabled 就是 false。
		errcode.Respond(c, errcode.AgentConfigUnreadable)
		return
	case aiagent.ReadyNoKey:
		errcode.Respond(c, errcode.AgentNoAPIKey)
		return
	case aiagent.ReadyKeyUnreadable:
		errcode.Respond(c, errcode.AgentKeyUnreadable)
		return
	}
	// 建会话之前先预检配置：否则每点一次「发送」都会先落一条空会话、再在流里报
	// 「未启用」，用户什么也没得到却在会话列表里攒了垃圾条目。
	convID := in.ConversationID
	if convID == 0 {
		conv := &models.AgentConversation{}
		if err := h.convCreate(conv); err != nil {
			errcode.Respond(c, errcode.Internal.WithErr(err))
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
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// total/truncated 与清单同批给出（口径同收件箱 / 整理历史 / 运行历史）：
	// 计数读不出时保守报「可能被截断」并省略 total。
	total, cntErr := errorSafeCount(h.convCount)
	truncated := cntErr != nil || total > int64(len(cs))
	resp := gin.H{"conversations": cs, "limit": oper.AgentConvListLimit, "truncated": truncated}
	if cntErr == nil {
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}

// Messages 单会话消息（含 reasoning/tool_calls）。
func (h *AgentChatHandler) Messages(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ms, err := h.msgList(id)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
	if !requireExists(c, h.convGet, id, errcode.AgentConversationNotFound) {
		return
	}
	if err := h.convDelete(id); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	token := fmt.Sprintf("%d:%s", convID, in.ToolCallID)
	if !h.agent.Resolve(token, in.Approved) {
		errcode.Respond(c, errcode.AgentConfirmGone)
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
	ac, _, err := aiagent.Load(h.kv, h.crypto)
	if errors.Is(err, aiagent.ErrConfigUnreadable) {
		// 从前这里把错误丢掉、回 200 + 默认值 + api_key_configured:false，
		// 面板于是显示「还没配过」——用户重填一遍就把真配置覆盖掉。
		errcode.Respond(c, errcode.AgentConfigUnreadable.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, aiagent.Redacted(ac))
}

// SetConfig 写 provider 配置。api_key 非空才加密覆盖。
// respondAgentSecretWriteErr AI 助手 API Key **密封失败**的出口（#395）。
//
// 这一支与「存储写不进去」是完全不同的两件事，不能共用一个 500：
//
//	密封失败   → 主密钥（SECRET_KEY）那侧：换过密钥、密文格式不对、随机源故障。
//	              用户该去确认 SECRET_KEY —— 重试一万次也不会好。
//	存储写失败 → kv.Set 写不进去（磁盘满/连接断/锁）。用户该腾空间/查库。
//
// 旧形状两处都是 Internal.WithErr ⇒ 前端 Assistant.vue 的 catch 直接把它当 err.message 弹出来，
// 用户看到「服务内部错误，请稍后重试」⇒ 反复点保存，而主密钥不会因为重试而恢复。
// 文案范式与 WebhookSecretUnavailable 同一族（那条码早就点名「可能更换过主密钥」）。
func respondAgentSecretWriteErr(c *gin.Context, err error) {
	errcode.Respond(c, errcode.AgentSecretWriteFailed.WithDetailKeepMessage(err))
}

// respondAgentConfigWriteErr AI 助手配置**存储写失败**的出口（#395）。
func respondAgentConfigWriteErr(c *gin.Context, err error) {
	errcode.Respond(c, errcode.SaveFailed.WithDetailKeepMessage(err))
}
func (h *AgentChatHandler) SetConfig(c *gin.Context) {
	var in agentConfigReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	existing, _, err := aiagent.Load(h.kv, h.crypto)
	if errors.Is(err, aiagent.ErrConfigUnreadable) {
		// 这份 existing 是合并基准：读失败时它是默认值、APIKeyEnc 为空，
		// 用户只改个 model 点保存也会把还在的 API Key 抹掉。读不出来就不写。
		errcode.Respond(c, errcode.AgentConfigUnreadable.WithErr(err))
		return
	}
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
			// #395 密封失败是主密钥侧的问题，与「存储写不进去」不是一回事。
			respondAgentSecretWriteErr(c, err)
			return
		}
		cfg.APIKeyEnc = enc
	}
	if err := aiagent.SaveConfig(h.kv, cfg); err != nil {
		respondAgentConfigWriteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, aiagent.Redacted(cfg))
}
