package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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
	results := h.ch.MessagesAll(c.Request.Context(), mb)
	c.JSON(http.StatusOK, gin.H{"sites": results})
}

// SyncOne 立即抓取单站消息。
func (h *MessageHandler) SyncOne(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	mb := parseMailbox(c.DefaultQuery("mailbox", "inbox"))
	msgs, e := h.ch.Messages(c.Request.Context(), s, mb)
	r := chain.MessageResult{SiteID: s.ID, SiteName: s.Name, Messages: msgs}
	if e != nil {
		r.Error = e.Error()
	}
	c.JSON(http.StatusOK, r)
}

// Content 取单条消息正文（?url=...）。
func (h *MessageHandler) Content(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	content, e := h.ch.MessageContent(c.Request.Context(), s, c.Query("url"))
	if e != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": e.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": content})
}

// Reply 回复消息（body: {url, content}）。
func (h *MessageHandler) Reply(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	var body struct {
		URL     string `json:"url" binding:"max=512"`
		Content string `json:"content" binding:"max=65536"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数有误"})
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	r, e := h.ch.ReplyMessage(c.Request.Context(), s, body.URL, body.Content)
	if e != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": e.Error()})
		return
	}
	c.JSON(http.StatusOK, r)
}

// Delete 删除消息（body: {url}）。
func (h *MessageHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	var body struct {
		URL string `json:"url" binding:"max=512"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数有误"})
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	if e := h.ch.DeleteMessage(c.Request.Context(), s, body.URL); e != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": e.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MarkRead 批量标记已读（body: {ids: [...]}）。
func (h *MessageHandler) MarkRead(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	var body struct {
		IDs []string `json:"ids" binding:"dive,max=128"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数有误"})
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	r, e := h.ch.MarkRead(c.Request.Context(), s, body.IDs)
	if e != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": e.Error()})
		return
	}
	c.JSON(http.StatusOK, r)
}
