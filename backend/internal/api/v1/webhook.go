package v1

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
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
		errcode.Respond(c, errcode.WebhookInvalid)
		return
	}

	// 读原始 body（上限 1MB，防滥用）
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		errcode.Respond(c, errcode.WebhookBadPayload)
		return
	}

	// 可选 HMAC 签名校验（X-Signature / X-Hub-Signature-256 = hmac-sha256(body, secret) hex）
	secret, secErr := h.op.DecryptedSecret(wh)
	if secErr != nil {
		// 配过密钥却解不开 ≠ 没配密钥：fail-closed，否则主密钥轮换后这个端点静默裸奔。
		slog.Error("webhook secret unreadable, rejecting request", "webhook", wh.Name, "err", secErr)
		errcode.Respond(c, errcode.WebhookSecretUnavailable.WithErr(secErr))
		return
	}
	if secret != "" {
		sig := strings.TrimPrefix(c.GetHeader("X-Signature"), "sha256=")
		if sig == "" {
			sig = strings.TrimPrefix(c.GetHeader("X-Hub-Signature-256"), "sha256=")
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(raw)
		want := hex.EncodeToString(mac.Sum(nil))
		if sig == "" || !hmac.Equal([]byte(sig), []byte(want)) {
			errcode.Respond(c, errcode.WebhookBadSignature)
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

	// 先记账面，再决定通知怎么说。面板读的是 trigger_count / last_triggered_at，
	// 这条写失败从前被 `_ =` 丢掉：通知照旧说「已接收」，面板却停在上一次触发，
	// 用户只会去查外部系统有没有打进来，不会怀疑这张表没写进去。
	recordErr := h.op.MarkTriggered(wh.ID, string(raw))
	if recordErr != nil {
		slog.Warn("webhook 触发记录落库失败：面板的触发次数/最近触发时间仍是上一次的值",
			"webhook_id", wh.ID, "webhook", wh.Name, "err", recordErr)
	}
	if h.notice != nil {
		_ = h.notice.Notice(c.Request.Context(), webhookTriggerNotice(wh.Name, len(raw), triggered, recordErr), false)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "triggered_workflow": triggered})
}

// webhookTriggerNotice 把「这次触发往通知中心写什么」收成一处（判定 + 文案），
// 这样「账面没记上」这一态可以被直接证伪，而不必造一个会写失败的数据库。
func webhookTriggerNotice(name string, rawLen int, triggered bool, recordErr error) notice.Input {
	body := fmt.Sprintf("已接收请求（%d 字节）", rawLen)
	if triggered {
		body += "，已触发工作流"
	}
	level := "info"
	if recordErr != nil {
		body += "；但「触发次数 / 最近触发时间」没能记进面板：" + recordErr.Error() +
			"（面板上看到的仍是上一次触发，别据此判断外部系统没打进来）"
		level = "warn"
	}
	return notice.Input{
		Source: notice.SourceWebhook,
		Title:  "Webhook 触发：" + name,
		Body:   body,
		Level:  level,
	}
}

// List 全部 webhook（管理端）。
func (h *WebhookHandler) List(c *gin.Context) {
	ws, err := h.op.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"webhooks": ws})
}

// Create 新建（自动生成 token）。
func (h *WebhookHandler) Create(c *gin.Context) {
	var in webhookReq
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	if in.Name == "" {
		errcode.Respond(c, errcode.WebhookNameRequired)
		return
	}
	w := &models.Webhook{
		Name: in.Name, Token: genToken(), TargetWorkflowID: in.TargetWorkflowID, Enabled: true,
	}
	if err := h.op.Create(w, in.Secret); err != nil {
		// #385 与 Update / Rotate 同一判据：密文写失败（主密钥侧）与库写失败都归
		// SaveFailed，并把原因带进响应体。旧形状 Internal.WithErr 把原因丢进日志、
		// 界面只显示「服务内部错误，请稍后重试」——而 Create 是最常走的那条新建路径。
		respondWebhookWriteErr(c, err)
		return
	}
	out, err := h.op.Get(w.ID)
	if err != nil {
		out = w // re-fetch 失败：用刚创建的对象（含 ID），避免返回 200 null
	}
	// token 已打 json:"-" 不随模型回传——创建时一次性下发（管理员据此拼接收 URL，
	// 之后仅 rotate 可再取；List 永不回明文）。
	c.JSON(http.StatusOK, gin.H{"webhook": out, "token": w.Token})
}

// respondWebhookWriteErr Webhook 写出口（Update / Rotate）的失败分类。
//
// #371 这两处从前是 Internal.WithErr(err)，界面只显示「服务内部错误，请稍后重试」。
// 而 op.Update 里可能失败的两步，用户都至少能判断方向：
//
//	Encrypt 失败 → 主密钥（SECRET_KEY）那侧的问题。SaveFailed 的文案本来就点名了
//	               「若提示主密钥相关，请检查 SECRET_KEY 是否被更换」，就是给这种场景写的，
//	               但写出口从没走到它 ⇒ 用户反复点保存，重试一万次也不会好。
//	db.Save 失败 → 库写不进去（磁盘满/连接断/锁），同样不该说成「内部错误」这种无从下手的话。
//
// 附带把原因带进响应体（旧形状是 WithErr 只进日志，用户与运维都拿不到线索）。
// 读侧不归这里管：按 id 取记录仍走 #345 的 respondRecordError（不存在 → 404）。
func respondWebhookWriteErr(c *gin.Context, err error) {
	errcode.Respond(c, errcode.SaveFailed.WithDetail(err))
}

// Update 编辑（secret 空=保留）。
func (h *WebhookHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	w, err := h.op.Get(id)
	if err != nil {
		// #345 与本文件 Delete 的 requireExists 同一判据：手抄的 Respond(WebhookNotFound)
		// 只判「不存在」，不判 DB 真故障（WebhookOper.Get 底层是 db.First）。
		respondRecordError(c, err, errcode.WebhookNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	var in webhookReq
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	if in.Name != "" {
		w.Name = in.Name
	}
	w.TargetWorkflowID = in.TargetWorkflowID
	w.Enabled = in.Enabled
	if err := h.op.Update(w, in.Secret); err != nil {
		// #371 密文写失败（主密钥侧）与库写失败都归 SaveFailed，详见该函数注释。
		respondWebhookWriteErr(c, err)
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
		// #345 与本文件 Delete 的 requireExists 同一判据：手抄的 Respond(WebhookNotFound)
		// 只判「不存在」，不判 DB 真故障（WebhookOper.Get 底层是 db.First）。
		respondRecordError(c, err, errcode.WebhookNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	w.Token = genToken()
	if err := h.op.Update(w, ""); err != nil {
		// #371 同 Update：Rotate 也会真的写库（token 换了），失败要说清是写失败。
		respondWebhookWriteErr(c, err)
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
	if !requireExists(c, h.op.Get, id, errcode.WebhookNotFound) {
		return
	}
	if err := h.op.Delete(id); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}
