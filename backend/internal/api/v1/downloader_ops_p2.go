package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/downloader"
)

type shareLimitsReq struct {
	Hashes           []string `json:"hashes" binding:"dive,max=64"`
	RatioLimit       float64  `json:"ratio_limit"`        // -2=无限 -1=全局默认 >=0=值
	SeedingTimeLimit int      `json:"seeding_time_limit"` // 分钟，同上语义
}

// SetShareLimits 设置分享比率/做种时长上限。
func (h *DownloaderHandler) SetShareLimits(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req shareLimitsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	opResult(c, h.dl.SetShareLimits(c.Request.Context(), id, req.Hashes, downloader.ShareLimits{
		RatioLimit:       req.RatioLimit,
		SeedingTimeLimit: req.SeedingTimeLimit,
	}))
}

// —— 队列优先级 ——

type queueReq struct {
	Hashes []string `json:"hashes" binding:"dive,max=64"`
	Action string   `json:"action" binding:"required,oneof=top up down bottom"`
}

// SetQueuePriority 调整队列优先级。
func (h *DownloaderHandler) SetQueuePriority(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req queueReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	opResult(c, h.dl.SetQueuePriority(c.Request.Context(), id, req.Hashes, req.Action))
}

// —— 强制开始 ——

type forceStartReq struct {
	Hashes []string `json:"hashes" binding:"dive,max=64"`
	Enable bool     `json:"enable"`
}

// SetForceStart 强制开始/取消强制。
func (h *DownloaderHandler) SetForceStart(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req forceStartReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	opResult(c, h.dl.SetForceStart(c.Request.Context(), id, req.Hashes, req.Enable))
}

// —— 顺序下载 / 首尾优先 ——

// ToggleSequential 切换顺序下载。
func (h *DownloaderHandler) ToggleSequential(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	opResult(c, h.dl.ToggleSequentialDownload(c.Request.Context(), id, req.Hashes))
}

// ToggleFirstLast 切换首尾优先。
func (h *DownloaderHandler) ToggleFirstLast(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	opResult(c, h.dl.ToggleFirstLastPiece(c.Request.Context(), id, req.Hashes))
}

// —— Peers ——

// Peers 列出某任务当前连接的对等体。
func (h *DownloaderHandler) Peers(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	hash := c.Query("hash")
	if hash == "" {
		errcode.Respond(c, errcode.TorrentHashRequired)
		return
	}
	peers, err := h.dl.ListPeers(c.Request.Context(), id, hash)
	if err != nil {
		opResult(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"peers": peers})
}

// —— 分类增删 ——

type categoryEditReq struct {
	Name     string `json:"name" binding:"required,max=128"`
	SavePath string `json:"save_path" binding:"max=512"`
}

// CreateCategory 新建分类。
func (h *DownloaderHandler) CreateCategory(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req categoryEditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	opResult(c, h.dl.CreateCategory(c.Request.Context(), id, req.Name, req.SavePath))
}

// EditCategory 编辑分类保存路径。
func (h *DownloaderHandler) EditCategory(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req categoryEditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	opResult(c, h.dl.EditCategory(c.Request.Context(), id, req.Name, req.SavePath))
}

type deleteCategoriesReq struct {
	Names []string `json:"names" binding:"dive,max=128"`
}

// DeleteCategories 删除分类。
func (h *DownloaderHandler) DeleteCategories(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req deleteCategoriesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	opResult(c, h.dl.DeleteCategories(c.Request.Context(), id, req.Names))
}
