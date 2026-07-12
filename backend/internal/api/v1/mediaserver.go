package v1

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/mediaserver"
)

// MediaServerHandler 处理媒体服务器 CRUD + 连接测试 + 库操作 + 封面生成 + webhook 接收。
type MediaServerHandler struct {
	svc *chain.MediaServerChain
}

func NewMediaServerHandler(svc *chain.MediaServerChain) *MediaServerHandler {
	return &MediaServerHandler{svc: svc}
}

// List 列出全部媒体服务器。
func (h *MediaServerHandler) List(c *gin.Context) {
	ms, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if ms == nil {
		ms = []models.MediaServer{} // 兜底空数组（避免 nil slice → JSON null）
	}
	c.JSON(http.StatusOK, gin.H{"servers": ms})
}

// Create 创建媒体服务器。
func (h *MediaServerHandler) Create(c *gin.Context) {
	var in oper.MediaServerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m, err := h.svc.Create(in)
	if err != nil {
		respondMediaErr(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// Update 更新媒体服务器（token 空串=保留）。
func (h *MediaServerHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.MediaServerInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m, err := h.svc.Update(id, in)
	if err != nil {
		respondMediaErr(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// Delete 删除媒体服务器。
func (h *MediaServerHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	opResult(c, h.svc.Delete(id))
}

// Test 连接 + 鉴权探活。
func (h *MediaServerHandler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	info, err := h.svc.Test(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "info": info})
}

// Libraries 列媒体库。
func (h *MediaServerHandler) Libraries(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	libs, err := h.svc.Libraries(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"libraries": libs})
}

// Scan 触发整库扫描（body: {library_id}；空=扫全部库）。
func (h *MediaServerHandler) Scan(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var body struct {
		LibraryID string `json:"library_id"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.LibraryID != "" {
		opResult(c, h.svc.LibraryScan(c.Request.Context(), id, body.LibraryID))
		return
	}
	// 空 library_id：扫该服务器全部库。
	libs, err := h.svc.Libraries(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, lib := range libs {
		if e := h.svc.LibraryScan(c.Request.Context(), id, lib.ID); e != nil {
			opResult(c, e)
			return
		}
	}
	opResult(c, nil)
}

// Refresh 刷新库元数据（body: {library_id}）。
func (h *MediaServerHandler) Refresh(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var body struct {
		LibraryID string `json:"library_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.LibraryID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "library_id 必填"})
		return
	}
	opResult(c, h.svc.Refresh(c.Request.Context(), id, body.LibraryID))
}

// Cover 生成封面拼图（返回 image/jpeg）。query: library_id=&cols=&rows=&w=&h=。
func (h *MediaServerHandler) Cover(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	libID := c.Query("library_id")
	if libID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "library_id 必填"})
		return
	}
	opt := mediaserver.DefaultCoverOptions()
	if v, err := strconv.Atoi(c.Query("cols")); err == nil && v > 0 {
		opt.Cols = v
	}
	if v, err := strconv.Atoi(c.Query("rows")); err == nil && v > 0 {
		opt.Rows = v
	}
	if v, err := strconv.Atoi(c.Query("w")); err == nil && v > 0 {
		opt.ThumbW = v
	}
	if v, err := strconv.Atoi(c.Query("h")); err == nil && v > 0 {
		opt.ThumbH = v
	}
	data, err := h.svc.Cover(c.Request.Context(), id, libID, opt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "image/jpeg", data)
}

// Notify 向该服务器发管理员通知（body: {title, body}）。
func (h *MediaServerHandler) Notify(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Title == "" {
		body.Title = "PTPatronus 通知"
	}
	opResult(c, h.svc.NotifyOne(c.Request.Context(), id, body.Title, body.Body))
}

// respondMediaErr 把 oper 错误映射到状态码：输入校验（类型/URL 非法）→ 400，其余 → 500。
func respondMediaErr(c *gin.Context, err error) {
	if errors.Is(err, oper.ErrInvalidInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// Webhook 公开接收媒体服务器播放/事件回调（媒体服务器无法带 JWT，故公开；按 :id 路由）。
// 仅当该服务器开启 NotifyOnPlay 时解析标题并通知；始终返回 200（避免服务器侧重试风暴）。
func (h *MediaServerHandler) Webhook(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"ok": false})
		return
	}
	payload, _ := io.ReadAll(c.Request.Body)
	h.svc.ReceiveWebhook(c.Request.Context(), id, payload)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
