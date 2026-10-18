package v1

import (
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"log/slog"
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
	reqLimit, _ := strconv.Atoi(c.Query("limit"))
	// 回显**生效的** limit：被归一过的请求若不告诉调用方，收件箱会以为自己拿到了全部。
	limit := oper.NormalizeNoticeLimit(reqLimit)
	ns, err := h.svc.List(unread, limit)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// total/truncated 与清单同批给出（口径同 RSS / 云任务队列 / 设备快照）：
	// 计数读不出时保守报「可能被截断」，也不编一个假 total。
	total, cntErr := h.svc.Count(unread)
	truncated := cntErr != nil || total > int64(len(ns))
	if cntErr != nil {
		slog.Warn("notice count failed", "unread", unread, "err", cntErr)
	}
	resp := gin.H{"notices": ns, "limit": limit, "truncated": truncated}
	if cntErr == nil {
		resp["total"] = total
	}
	// 角标要的是「未读总数」，而收件箱默认按全量分页：只从这一页里数未读，
	// 未读超过上限时角标会少报（用户以为只剩这些）。多一条 Count(true) 给前端兜底。
	if unreadTotal, e := h.svc.Count(true); e == nil {
		resp["unread_total"] = unreadTotal
	} else {
		slog.Warn("notice unread count failed", "err", e)
	}
	c.JSON(http.StatusOK, resp)
}

// Read 标单条已读。
func (h *NoticeHandler) Read(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.MarkRead(id); err != nil {
		respondRecordError(c, err, errcode.NoticeNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, gin.H{"read": id})
}

// ReadAll 全部已读。
func (h *NoticeHandler) ReadAll(c *gin.Context) {
	if err := h.svc.MarkAllRead(); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
		respondRecordError(c, err, errcode.NoticeNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Clear 一键清空全部通知。
func (h *NoticeHandler) Clear(c *gin.Context) {
	n, err := h.svc.ClearAll()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"cleared": n})
}
