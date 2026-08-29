package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

func (h *DownloaderHandler) Files(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	hash := c.Query("hash")
	if hash == "" {
		errcode.Respond(c, errcode.TorrentHashRequired)
		return
	}
	files, err := h.dl.ListFiles(c.Request.Context(), id, hash)
	if err != nil {
		opResult(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

type filePrioReq struct {
	Hash     string `json:"hash" binding:"max=64"`
	Indices  []int  `json:"indices"`
	Priority int    `json:"priority"`
}

// SetFilePriority 设置文件优先级。
func (h *DownloaderHandler) SetFilePriority(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req filePrioReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opResult(c, h.dl.SetFilePriority(c.Request.Context(), id, req.Hash, req.Indices, req.Priority))
}

// —— Tracker ——

// Trackers 列出某任务的 tracker。
func (h *DownloaderHandler) Trackers(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	hash := c.Query("hash")
	if hash == "" {
		errcode.Respond(c, errcode.TorrentHashRequired)
		return
	}
	trk, err := h.dl.ListTrackers(c.Request.Context(), id, hash)
	if err != nil {
		opResult(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"trackers": trk})
}

type trackersReq struct {
	Hash string   `json:"hash" binding:"required,max=64"`
	URLs []string `json:"urls" binding:"required,min=1,dive,required,max=512"`
}

// AddTrackers 追加 tracker。
func (h *DownloaderHandler) AddTrackers(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req trackersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opResult(c, h.dl.AddTrackers(c.Request.Context(), id, req.Hash, req.URLs))
}

// RemoveTrackers 移除 tracker。
func (h *DownloaderHandler) RemoveTrackers(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req trackersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	opResult(c, h.dl.RemoveTrackers(c.Request.Context(), id, req.Hash, req.URLs))
}
