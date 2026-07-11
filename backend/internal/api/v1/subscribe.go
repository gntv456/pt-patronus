package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// SubscribeHandler 处理媒体订阅的 CRUD 与手动检查。
type SubscribeHandler struct {
	subCh *chain.SubscribeChain
}

// NewSubscribeHandler 构造。
func NewSubscribeHandler(subCh *chain.SubscribeChain) *SubscribeHandler {
	return &SubscribeHandler{subCh: subCh}
}

// subReq 创建/编辑订阅的请求体。
type subReq struct {
	Source       string `json:"source" binding:"max=16"`
	SourceID     string `json:"source_id" binding:"max=64"`
	Title        string `json:"title" binding:"max=255"`
	Year         int    `json:"year"`
	Type         string `json:"type" binding:"max=16"`
	Category     string `json:"category" binding:"max=16"`
	PosterURL    string `json:"poster_url" binding:"max=512"`
	Resolution   string `json:"resolution" binding:"max=32"`
	DownloaderID uint   `json:"downloader_id"`
	SavePath     string `json:"save_path" binding:"max=255"`
	Status       string `json:"status" binding:"max=16"`
}

// List 列出全部订阅。
func (h *SubscribeHandler) List(c *gin.Context) {
	ss, err := h.subCh.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscriptions": ss})
}

// Create 新建订阅（建后立即异步检查一次）。
func (h *SubscribeHandler) Create(c *gin.Context) {
	var in subReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Title == "" || in.Source == "" || in.SourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title/source/source_id required"})
		return
	}
	s := &models.Subscription{
		Source: in.Source, SourceID: in.SourceID, Title: in.Title, Year: in.Year,
		Type: in.Type, Category: in.Category, PosterURL: in.PosterURL,
		Resolution: in.Resolution, DownloaderID: in.DownloaderID, SavePath: in.SavePath,
		Status: in.Status,
	}
	out, err := h.subCh.Create(c.Request.Context(), s)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// Update 编辑订阅（resolution/downloader/save_path/status）。
func (h *SubscribeHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	s, err := h.subCh.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return
	}
	var in subReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Resolution != "" || in.Status != "" || in.DownloaderID != 0 || in.SavePath != "" {
		// 仅更新提供的字段（空=不改）
		if in.Resolution != "" {
			s.Resolution = in.Resolution
		}
		if in.Status != "" {
			s.Status = in.Status
		}
		if in.DownloaderID != 0 {
			s.DownloaderID = in.DownloaderID
		}
		if in.SavePath != "" {
			s.SavePath = in.SavePath
		}
	}
	if err := h.subCh.Update(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

// Delete 删除订阅。
func (h *SubscribeHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.subCh.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Check 手动触发单个订阅检查，返回本轮推送条数。
func (h *SubscribeHandler) Check(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	s, err := h.subCh.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return
	}
	n, err := h.subCh.Check(c.Request.Context(), s)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "pushed": 0, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "pushed": n})
}
