package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
)

// DownloaderHandler 处理下载器 CRUD 与任务操作。
type DownloaderHandler struct {
	op  *oper.DownloaderOper
	dl  *chain.DownloadChain
	mgr *manager.Manager
}

// NewDownloaderHandler 构造。
func NewDownloaderHandler(op *oper.DownloaderOper, dl *chain.DownloadChain, mgr *manager.Manager) *DownloaderHandler {
	return &DownloaderHandler{op: op, dl: dl, mgr: mgr}
}

// List 列出全部下载器。
func (h *DownloaderHandler) List(c *gin.Context) {
	ds, err := h.op.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"downloaders": ds})
}

// Create 创建下载器。
func (h *DownloaderHandler) Create(c *gin.Context) {
	var in oper.DownloaderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	d, err := h.op.Create(in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, d)
}

// Update 更新下载器（失效缓存）。
func (h *DownloaderHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.DownloaderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	d, err := h.op.Update(id, in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.mgr.Invalidate(id)
	c.JSON(http.StatusOK, d)
}

// Delete 删除下载器。
func (h *DownloaderHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.op.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.mgr.Invalidate(id)
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Test 连接测试。
func (h *DownloaderHandler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.dl.Test(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Torrents 列出任务。
func (h *DownloaderHandler) Torrents(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ts, err := h.dl.List(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"torrents": []any{}, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"torrents": ts})
}

type addReq struct {
	URL           string   `json:"url" binding:"required"`
	Category      string   `json:"category"`
	Tags          []string `json:"tags"`
	SavePath      string   `json:"save_path"`
	Paused        bool     `json:"paused"`
	UploadLimit   int64    `json:"upload_limit"`   // bytes/s, 0 不限
	DownloadLimit int64    `json:"download_limit"` // bytes/s, 0 不限
}

// Add 添加种子。
func (h *DownloaderHandler) Add(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req addReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.dl.Add(c.Request.Context(), id, downloader.AddOptions{
		URL: req.URL, Category: req.Category, Tags: req.Tags, SavePath: req.SavePath, Paused: req.Paused,
		UploadLimit: req.UploadLimit, DownloadLimit: req.DownloadLimit,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type hashesReq struct {
	Hashes      []string `json:"hashes"`
	DeleteFiles bool     `json:"delete_files"`
}

// Pause 暂停任务。
func (h *DownloaderHandler) Pause(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.dl.Pause(c.Request.Context(), id, req.Hashes); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Resume 恢复任务。
func (h *DownloaderHandler) Resume(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.dl.Resume(c.Request.Context(), id, req.Hashes); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Remove 删除任务。
func (h *DownloaderHandler) Remove(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.dl.Delete(c.Request.Context(), id, req.Hashes, req.DeleteFiles); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Stats 全局速率。
func (h *DownloaderHandler) Stats(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	st, err := h.dl.Stats(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, st)
}
