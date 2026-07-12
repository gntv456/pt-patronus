package v1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// KeepSeedHandler 处理辅种候选查询。
type KeepSeedHandler struct {
	ks *chain.KeepSeedChain
}

// NewKeepSeedHandler 构造。
func NewKeepSeedHandler(ks *chain.KeepSeedChain) *KeepSeedHandler {
	return &KeepSeedHandler{ks: ks}
}

type findReq struct {
	Name string `json:"name" binding:"required,max=512"`
	Size int64  `json:"size" binding:"min=0,max=1099511627776"` // 字节；为 0 仅按名称评分（上限 1TB）
}

// Find 按名称/大小跨站查找辅种候选。
func (h *KeepSeedHandler) Find(c *gin.Context) {
	var req findReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cands := h.ks.FindCandidates(c.Request.Context(), req.Name, req.Size)
	// 转 JSON 友好结构
	out := make([]gin.H, 0, len(cands))
	for _, cd := range cands {
		out = append(out, gin.H{
			"site_id":   cd.SiteID,
			"site_name": cd.SiteName,
			"score":     cd.Score,
			"torrent":   toTorrentJSON(cd.Torrent),
		})
	}
	c.JSON(http.StatusOK, gin.H{"candidates": out})
}

// toTorrentJSON 把 site.Torrent 转为可读 map。
func toTorrentJSON(t site.Torrent) gin.H {
	return gin.H{
		"title":        t.Title,
		"size":         t.Size,
		"size_text":    "",
		"seeders":      t.Seeders,
		"detail_url":   t.DetailURL,
		"download_url": t.DownloadURL,
	}
}

// ---- IYUU 精确辅种 ----

// GetConfig 返回辅种配置（token 掩码：仅显长度，不回明文）。
func (h *KeepSeedHandler) GetConfig(c *gin.Context) {
	token, err := h.ks.GetToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	masked := ""
	if token != "" {
		if len(token) > 8 {
			masked = token[:4] + "****" + token[len(token)-4:]
		} else {
			masked = "****"
		}
	}
	c.JSON(http.StatusOK, gin.H{"iyuu_token_masked": masked, "configured": token != ""})
}

type configReq struct {
	IYUUToken string `json:"iyuu_token" binding:"max=256"`
}

// SetConfig 保存辅种配置（IYUU token 明文写，DB 存储）。
func (h *KeepSeedHandler) SetConfig(c *gin.Context) {
	var req configReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.ks.SetToken(strings.TrimSpace(req.IYUUToken)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type reseedReq struct {
	DownloaderID uint `json:"downloader_id" binding:"required,min=1"`
}

// Reseed 触发某下载器的 IYUU 精确辅种，返回新增 pending 候选。
func (h *KeepSeedHandler) Reseed(c *gin.Context) {
	var req reseedReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	created, err := h.ks.Reseed(c.Request.Context(), req.DownloaderID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": created, "count": len(created)})
}

// Tasks 列出辅种候选任务（?status=pending|added|skipped|failed）。
func (h *KeepSeedHandler) Tasks(c *gin.Context) {
	tasks, err := h.ks.ListTasks(c.Query("status"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

type resolveReq struct {
	Action string `json:"action" binding:"required,max=16"` // add | skip
}

// Resolve 处理一个候选任务：添加辅种 / 跳过。
func (h *KeepSeedHandler) Resolve(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req resolveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.ks.Resolve(c.Request.Context(), uint(id), req.Action); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetSetting 返回该下载器的辅种设置（auto_reseed / auto_reseed_add / save_path / pause_after_add）。
func (h *KeepSeedHandler) GetSetting(c *gin.Context) {
	id, err := strconv.ParseUint(c.Query("downloader_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid downloader_id"})
		return
	}
	s, err := h.ks.Setting(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

type settingReq struct {
	DownloaderID  uint   `json:"downloader_id" binding:"required,min=1"`
	SavePath      string `json:"save_path" binding:"max=512"`
	PauseAfterAdd bool   `json:"pause_after_add"`
	AutoReseed    bool   `json:"auto_reseed"`
	AutoReseedAdd bool   `json:"auto_reseed_add"`
}

// SetSetting upsert 该下载器的辅种设置。
func (h *KeepSeedHandler) SetSetting(c *gin.Context) {
	var req settingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s := models.CrossSeedSetting{
		DownloaderID:  req.DownloaderID,
		SavePath:      req.SavePath,
		PauseAfterAdd: req.PauseAfterAdd,
		AutoReseed:    req.AutoReseed,
		AutoReseedAdd: req.AutoReseedAdd,
	}
	if err := h.ks.SaveSetting(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
