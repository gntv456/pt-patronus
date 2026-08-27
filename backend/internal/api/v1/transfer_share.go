package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// ---- 识别词云端共享（共享仓库 CRUD + 拉取预览 + 一键合并导入） ----

// GetSharedRepos GET /transfer/identify-words/repos。
func (h *TransferHandler) GetSharedRepos(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"repos": transfer.LoadSharedRepos(h.ch.KV())})
}

// SetSharedRepos PUT /transfer/identify-words/repos（全量保存仓库列表）。
func (h *TransferHandler) SetSharedRepos(c *gin.Context) {
	var in struct {
		Repos []transfer.SharedRepo `json:"repos" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := transfer.SaveSharedRepos(h.ch.KV(), in.Repos); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"repos": in.Repos})
}

// FetchSharedRepo POST /transfer/identify-words/repos/preview —— 拉取仓库词表预览
//（不落库：先看会合并进哪些新词，确认后再导入；与 MSaber「云端协作共享」同流程）。
func (h *TransferHandler) FetchSharedRepo(c *gin.Context) {
	var in struct {
		URL string `json:"url" binding:"required,url"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	words, err := transfer.FetchSharedRepo(c.Request.Context(), in.URL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	local := transfer.LoadIdentifyWords(h.ch.KV())
	_, added := transfer.MergeWords(local, words)
	c.JSON(http.StatusOK, gin.H{"words": words, "total": len(words), "new": added})
}

// ImportSharedRepo POST /transfer/identify-words/repos/import —— 拉取并合并进本地词表。
// 本地同 kind+from 优先（云端不覆盖本地自定义）；返回合并后总数与新增数。
func (h *TransferHandler) ImportSharedRepo(c *gin.Context) {
	var in struct {
		URL string `json:"url" binding:"required,url"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	words, err := transfer.FetchSharedRepo(c.Request.Context(), in.URL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	local := transfer.LoadIdentifyWords(h.ch.KV())
	merged, added := transfer.MergeWords(local, words)
	if err := transfer.SaveIdentifyWords(h.ch.KV(), merged); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": len(merged), "added": added})
}
