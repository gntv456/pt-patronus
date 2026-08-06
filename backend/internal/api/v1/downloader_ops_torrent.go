package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/downloader"
)

type categoryReq struct {
	Hashes   []string `json:"hashes" binding:"dive,max=64"`
	Category string   `json:"category" binding:"max=128"`
}

// SetCategoryOp 设置分类。
func (h *DownloaderHandler) SetCategoryOp(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req categoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetCategory(c.Request.Context(), id, req.Hashes, req.Category))
}

type tagsReq struct {
	Hashes []string `json:"hashes" binding:"dive,max=64"`
	Tags   []string `json:"tags" binding:"dive,max=128"`
	Mode   string   `json:"mode" binding:"required,oneof=set add remove"`
}

// TagsOp 标签操作（按 mode 分发）。
func (h *DownloaderHandler) TagsOp(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req tagsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	var err error
	switch req.Mode {
	case "add":
		err = h.dl.AddTags(ctx, id, req.Hashes, req.Tags)
	case "remove":
		err = h.dl.RemoveTags(ctx, id, req.Hashes, req.Tags)
	default: // set
		err = h.dl.SetTags(ctx, id, req.Hashes, req.Tags)
	}
	opResult(c, err)
}

// —— 生命周期 ——

// Recheck 强制重新校验。
func (h *DownloaderHandler) Recheck(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.Recheck(c.Request.Context(), id, req.Hashes))
}

// Reannounce 重新汇报。
func (h *DownloaderHandler) Reannounce(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.Reannounce(c.Request.Context(), id, req.Hashes))
}

type locationReq struct {
	Hashes   []string `json:"hashes" binding:"dive,max=64"`
	Location string   `json:"location" binding:"max=512"`
}

// SetLocation 移动保存路径。
func (h *DownloaderHandler) SetLocation(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req locationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetLocation(c.Request.Context(), id, req.Hashes, req.Location))
}

// —— 限速 ——

type limitsReq struct {
	Hashes        []string `json:"hashes" binding:"dive,max=64"`
	UploadLimit   int64    `json:"upload_limit"`
	DownloadLimit int64    `json:"download_limit"`
}

// SetLimits 设置单/多任务限速。
func (h *DownloaderHandler) SetLimits(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req limitsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetLimits(c.Request.Context(), id, req.Hashes, downloader.Limits{
		UploadLimit: req.UploadLimit, DownloadLimit: req.DownloadLimit,
	}))
}

// GetSpeedLimits 读取全局速度上限。
func (h *DownloaderHandler) GetSpeedLimits(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	lim, err := h.dl.GetSpeedLimits(c.Request.Context(), id)
	if err != nil {
		opResult(c, err)
		return
	}
	c.JSON(http.StatusOK, lim)
}

type speedLimitsReq struct {
	UploadLimit   int64 `json:"upload_limit"`
	DownloadLimit int64 `json:"download_limit"`
}

// SetSpeedLimits 设置全局速度上限。
func (h *DownloaderHandler) SetSpeedLimits(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req speedLimitsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opResult(c, h.dl.SetSpeedLimits(c.Request.Context(), id, downloader.Limits{
		UploadLimit: req.UploadLimit, DownloadLimit: req.DownloadLimit,
	}))
}
