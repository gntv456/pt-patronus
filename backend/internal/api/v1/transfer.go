package v1

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/dirwatch"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// TransferHandler 处理媒体整理的历史/配置/手动整理/重做/扫描。
type TransferHandler struct {
	ch *chain.TransferChain
}

// NewTransferHandler 构造。
func NewTransferHandler(ch *chain.TransferChain) *TransferHandler {
	return &TransferHandler{ch: ch}
}

// transferConfigReq 配置更新请求体（空字段=不改语义由 withDefaults 兜底）。
type transferConfigReq = transfer.Config

// organizeReq 手动整理请求体。
type organizeReq struct {
	DownloaderID uint   `json:"downloader_id"`
	Hash         string `json:"hash" binding:"max=128"`
}

// History 列整理历史（可选 status 过滤、limit）。
func (h *TransferHandler) History(c *gin.Context) {
	status := c.Query("status")
	limit := 200
	if n, err := parseLimit(c.Query("limit")); err == nil && n > 0 {
		if n > 500 {
			n = 500
		}
		limit = int(n)
	}
	hs, err := h.ch.History(status, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"history": hs}) // oper 保证非 nil
}

// Summary 整理待办汇总：failed 历史数 + 扫下载器得到的真实待入库数（可选 items 明细）。
// Query: items=1 附带 pending_items；limit 限制明细条数（默认 50，最大 100）。
func (h *TransferHandler) Summary(c *gin.Context) {
	withItems := c.Query("items") == "1" || c.Query("items") == "true"
	itemLimit := 50
	if n, err := parseLimit(c.Query("limit")); err == nil && n > 0 {
		itemLimit = int(n)
	}
	sum, err := h.ch.Summary(c.Request.Context(), withItems, itemLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sum)
}

// GetConfig 读整理配置。
func (h *TransferHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.ch.Config())
}

// SetConfig 写整理配置（即时生效）。
func (h *TransferHandler) SetConfig(c *gin.Context) {
	var in transferConfigReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.ch.SetConfig(in); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.ch.Config())
}

// GetDirwatchConfig 读目录监视配置。
func (h *TransferHandler) GetDirwatchConfig(c *gin.Context) {
	c.JSON(http.StatusOK, dirwatch.Redacted(h.ch.DirwatchConfig()))
}

// SetDirwatchConfig 写目录监视配置（需重启进程生效）。
func (h *TransferHandler) SetDirwatchConfig(c *gin.Context) {
	var in dirwatch.Config
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.ch.SetDirwatchConfig(in); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dirwatch.Redacted(in))
}

// GetCategoriesConfig 读自动分类规则。
func (h *TransferHandler) GetCategoriesConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.ch.CategoriesConfig())
}

// SetCategoriesConfig 写自动分类规则。
func (h *TransferHandler) SetCategoriesConfig(c *gin.Context) {
	var in transfer.CategoryConfig
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.ch.SetCategoriesConfig(in); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.ch.CategoriesConfig())
}

// Organize 手动整理指定下载器的某个种子（绕过分类过滤）。
func (h *TransferHandler) Organize(c *gin.Context) {
	var in organizeReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.DownloaderID == 0 || in.Hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "downloader_id and hash required"})
		return
	}
	res, err := h.ch.ManualOrganize(c.Request.Context(), in.DownloaderID, in.Hash)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": res})
}

// Redo 重做一条历史记录（按 hash 跨下载器找回种子重整）。
func (h *TransferHandler) Redo(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ch.Redo(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": res})
}

// DeleteHistory 删单条历史。
func (h *TransferHandler) DeleteHistory(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.ch.DeleteHistory(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// BatchDeleteHistory 批量删除整理历史（admin）。返回删除成功数与失败 id 列表（不存在的）。
func (h *TransferHandler) BatchDeleteHistory(c *gin.Context) {
	var body struct {
		IDs []uint `json:"ids" binding:"required,min=1,max=200,dive"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	failed, err := h.ch.DeleteHistories(body.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if failed == nil {
		failed = []uint{}
	}
	c.JSON(http.StatusOK, gin.H{"deleted": len(body.IDs) - len(failed), "failed": failed})
}

// ClearFailedHistory 一键清除全部失败整理记录（admin）。返回删除条数。
func (h *TransferHandler) ClearFailedHistory(c *gin.Context) {
	n, err := h.ch.ClearFailed()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// Sweep 手动触发一次整理扫描（所有下载器的已完成种子）。
func (h *TransferHandler) Sweep(c *gin.Context) {
	go h.ch.Sweep(transferSweepContext(c.Request.Context()))
	c.JSON(http.StatusOK, gin.H{"ok": true, "msg": "sweep started"})
}

func transferSweepContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
