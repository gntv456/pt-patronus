package v1

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/core/safego"
)

// WebhookHandler 处理进站 Webhook（公开接收）+ 管理端 CRUD。
type WebhookHandler struct {
	op     *oper.WebhookOper
	wfCh   *chain.WorkflowChain
	bus    *event.Bus
	notice *notice.Service
}

func NewWebhookHandler(op *oper.WebhookOper, wfCh *chain.WorkflowChain, bus *event.Bus, ns *notice.Service) *WebhookHandler {
	return &WebhookHandler{op: op, wfCh: wfCh, bus: bus, notice: ns}
}

// webhookReq 管理端创建/编辑请求体。Secret 为明文（由 oper 加密入库；空=不设/保留）。
type webhookReq struct {
	Name             string `json:"name" binding:"max=128"`
	TargetWorkflowID uint   `json:"target_workflow_id"`
	Secret           string `json:"secret" binding:"max=256"`
	Enabled          bool   `json:"enabled"`
}

// genToken 生成 32 字符 hex 随机 token（URL 密钥）。
func genToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Receive 公开接收端点：POST /webhooks/:token。
// 校验 token → (可选)HMAC 签名校验 → 解析 payload → 触发目标工作流 + 广播 "webhook" 事件 + 通知。
func (h *WebhookHandler) Receive(c *gin.Context) {
	token := c.Param("token")
	wh, err := h.op.GetByToken(token)
	if err != nil || wh == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "无效的 webhook"})
		return
	}

	// 读原始 body（上限 1MB，防滥用）
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取请求体失败"})
		return
	}

	// 可选 HMAC 签名校验（X-Signature / X-Hub-Signature-256 = hmac-sha256(body, secret) hex）
	if secret := h.op.DecryptedSecret(wh); secret != "" {
		sig := strings.TrimPrefix(c.GetHeader("X-Signature"), "sha256=")
		if sig == "" {
			sig = strings.TrimPrefix(c.GetHeader("X-Hub-Signature-256"), "sha256=")
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(raw)
		want := hex.EncodeToString(mac.Sum(nil))
		if sig == "" || !hmac.Equal([]byte(sig), []byte(want)) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "签名校验失败"})
			return
		}
	}

	// 解析 payload（JSON 优先，否则包成 {raw: "..."}）
	payload := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			payload = map[string]any{"raw": string(raw)}
		}
	}
	payload["webhook_id"] = wh.ID
	payload["webhook_name"] = wh.Name

	// 直接触发目标工作流（异步，不阻塞响应）
	triggered := false
	if wh.TargetWorkflowID != 0 {
		triggered = true
		target := wh.TargetWorkflowID
		wname := wh.Name
		reqCtx := c.Request.Context()
		safego.Go("webhook:101", func() {
			// HTTP 请求在 handler 返回后必然取消；后台工作流应保留请求值，
			// 但由自身超时控制，不能继承这次请求的取消信号。
			bg, cancel := context.WithTimeout(context.WithoutCancel(reqCtx), 10*time.Minute)
			defer cancel()
			if _, err := h.wfCh.Run(bg, target, "webhook", payload); err != nil {
				slog.Warn("webhook trigger workflow failed", "workflow_id", target, "err", err, "webhook", wname)
			}
		})
	}

	// 广播 "webhook" 事件（event 型工作流可监听 event_type=webhook）
	if h.bus != nil {
		h.bus.Publish(c.Request.Context(), "webhook", payload)
	}

	// 落通知中心（一次性，不去重）
	if h.notice != nil {
		_ = h.notice.Notice(c.Request.Context(), notice.Input{
			Source: notice.SourceWebhook, Title: "Webhook 触发：" + wh.Name,
			Body: fmt.Sprintf("已接收请求（%d 字节）%s", len(raw), func() string {
				if triggered {
					return "，已触发工作流"
				}
				return ""
			}()), Level: "info",
		}, false)
	}

	_ = h.op.MarkTriggered(wh.ID, string(raw))
	c.JSON(http.StatusOK, gin.H{"ok": true, "triggered_workflow": triggered})
}

// List 全部 webhook（管理端）。
func (h *WebhookHandler) List(c *gin.Context) {
	ws, err := h.op.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"webhooks": ws})
}

// Create 新建（自动生成 token）。
func (h *WebhookHandler) Create(c *gin.Context) {
	var in webhookReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称必填"})
		return
	}
	w := &models.Webhook{
		Name: in.Name, Token: genToken(), TargetWorkflowID: in.TargetWorkflowID, Enabled: true,
	}
	if err := h.op.Create(w, in.Secret); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out, err := h.op.Get(w.ID)
	if err != nil {
		out = w // re-fetch 失败：用刚创建的对象（含 ID），避免返回 200 null
	}
	c.JSON(http.StatusOK, out)
}

// Update 编辑（secret 空=保留）。
func (h *WebhookHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	w, err := h.op.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook 不存在"})
		return
	}
	var in webhookReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Name != "" {
		w.Name = in.Name
	}
	w.TargetWorkflowID = in.TargetWorkflowID
	w.Enabled = in.Enabled
	if err := h.op.Update(w, in.Secret); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out, err := h.op.Get(id)
	if err != nil {
		out = w // re-fetch 失败：用已更新的对象，避免返回 200 null
	}
	c.JSON(http.StatusOK, out)
}

// Rotate 重生成 token（旧 URL 立即失效）。
func (h *WebhookHandler) Rotate(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	w, err := h.op.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook 不存在"})
		return
	}
	w.Token = genToken()
	if err := h.op.Update(w, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": w.Token})
}

// Delete 删除。
func (h *WebhookHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.op.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}
