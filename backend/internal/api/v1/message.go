package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// MessageHandler 站点私信（列表 + 正文 + 回复/删除/标记已读）。
type MessageHandler struct {
	ch     *chain.SiteChain
	siteOp *oper.SiteOper
}

// NewMessageHandler 构造。
func NewMessageHandler(ch *chain.SiteChain, siteOp *oper.SiteOper) *MessageHandler {
	return &MessageHandler{ch: ch, siteOp: siteOp}
}

func parseMailbox(s string) site.MailboxType {
	switch s {
	case "sent":
		return site.MailboxSent
	case "system":
		return site.MailboxSystem
	case "cheater":
		return site.MailboxCheater
	case "reports":
		return site.MailboxReports
	case "staff":
		return site.MailboxStaff
	}
	return site.MailboxInbox
}

// All 返回全站消息聚合（?mailbox=inbox|sent|system|...，默认 inbox）。
func (h *MessageHandler) All(c *gin.Context) {
	mb := parseMailbox(c.DefaultQuery("mailbox", "inbox"))
	force := c.Query("refresh") == "1"
	results := h.ch.MessagesAll(c.Request.Context(), mb, force)
	c.JSON(http.StatusOK, gin.H{"sites": results})
}

// Stream 流式返回消息（NDJSON，每站一行）：每站解析完立即 flush，前端"加载一个显示一个"，
// 不等全部站抓完。慢站（context deadline 15s）不会阻塞已就绪站的展示。
func (h *MessageHandler) Stream(c *gin.Context) {
	mb := parseMailbox(c.DefaultQuery("mailbox", "inbox"))
	force := c.Query("refresh") == "1"
	w := c.Writer
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // 关反代缓冲，每站即时到前端
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	var mu sync.Mutex // emit 并发回调，写 ResponseWriter 需串行化
	emit := func(r chain.MessageResult) {
		b, err := json.Marshal(r)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
	listErr := h.ch.MessagesStream(c.Request.Context(), mb, force, emit)
	if listErr != "" {
		// 站点清单读失败=0 行且无任何错误留痕，末尾 done 行带 list_error 供前端弹错。
		emitDone(w, &mu, flusher, listErr)
	}
}

// SyncOne 立即抓取单站消息。
func (h *MessageHandler) SyncOne(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.SiteInvalidID)
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		respondSiteLookup(c, err, h.siteOp)
		return
	}
	mb := parseMailbox(c.DefaultQuery("mailbox", "inbox"))
	msgs, e := h.ch.Messages(c.Request.Context(), s, mb)
	r := chain.MessageResult{SiteID: s.ID, SiteName: s.Name, Messages: msgs}
	// 未接线判定与聚合面同源：漏接这一处，用户点单站刷新后「平台没适配私信」会退化成「这站没私信」。
	r.Unsupported = !h.ch.MessagesWired(s)
	if e != nil {
		r.Error = userErrText(e)
	}
	c.JSON(http.StatusOK, r)
}

// Content 取单条消息正文（?url=...）。
func (h *MessageHandler) Content(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.SiteInvalidID)
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		respondSiteLookup(c, err, h.siteOp)
		return
	}
	content, e := h.ch.MessageContent(c.Request.Context(), s, c.Query("url"))
	if e != nil {
		respondFetchError(c, e, errcode.BadGateway)
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": content})
}

// Reply 回复消息（body: {url, content}）。
func (h *MessageHandler) Reply(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.SiteInvalidID)
		return
	}
	var body struct {
		URL     string `json:"url" binding:"max=512"`
		Content string `json:"content" binding:"max=65536"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.InvalidInput)
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		respondSiteLookup(c, err, h.siteOp)
		return
	}
	r, e := h.ch.ReplyMessage(c.Request.Context(), s, body.URL, body.Content)
	if e != nil {
		respondFetchError(c, e, errcode.BadGateway)
		return
	}
	c.JSON(http.StatusOK, r)
}

// Delete 删除消息（body: {url}）。
func (h *MessageHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.SiteInvalidID)
		return
	}
	var body struct {
		URL string `json:"url" binding:"max=512"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.InvalidInput)
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		respondSiteLookup(c, err, h.siteOp)
		return
	}
	if e := h.ch.DeleteMessage(c.Request.Context(), s, body.URL); e != nil {
		respondFetchError(c, e, errcode.BadGateway)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MarkRead 批量标记已读（body: {ids: [...]}）。
func (h *MessageHandler) MarkRead(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.SiteInvalidID)
		return
	}
	var body struct {
		IDs []string `json:"ids" binding:"dive,max=128"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.InvalidInput)
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		respondSiteLookup(c, err, h.siteOp)
		return
	}
	r, e := h.ch.MarkRead(c.Request.Context(), s, body.IDs)
	if e != nil {
		respondFetchError(c, e, errcode.BadGateway)
		return
	}
	c.JSON(http.StatusOK, r)
}
