package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// NoticeHandler 全局通知中心（站内收件箱）。
type NoticeHandler struct {
	svc *notice.Service
}

// NewNoticeHandler 构造。
func NewNoticeHandler(svc *notice.Service) *NoticeHandler {
	return &NoticeHandler{svc: svc}
}

// List 列通知（?unread=true 仅未读；?limit=）。
func (h *NoticeHandler) List(c *gin.Context) {
	unread := c.Query("unread") == "true"
	limit, _ := strconv.Atoi(c.Query("limit"))
	ns, err := h.svc.List(unread, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"notices": ns})
}

// Read 标单条已读。
func (h *NoticeHandler) Read(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.MarkRead(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"read": id})
}

// ReadAll 全部已读。
func (h *NoticeHandler) ReadAll(c *gin.Context) {
	if err := h.svc.MarkAllRead(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"read_all": true})
}

// Delete 删单条。
func (h *NoticeHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Clear 一键清空全部通知。
func (h *NoticeHandler) Clear(c *gin.Context) {
	n, err := h.svc.ClearAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cleared": n})
}
