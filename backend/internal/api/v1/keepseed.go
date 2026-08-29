package v1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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

// GetConfig 返回辅种配置（IYUU token 掩码 + Torznab 端点）。
func (h *KeepSeedHandler) GetConfig(c *gin.Context) {
	token, err := h.ks.GetToken()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
	tzURL, tzKey, _ := h.ks.GetTorznab()
	tzKeyMasked := ""
	if tzKey != "" {
		if len(tzKey) > 6 {
			tzKeyMasked = tzKey[:3] + "****" + tzKey[len(tzKey)-2:]
		} else {
			tzKeyMasked = "****"
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"iyuu_token_masked":  masked,
		"configured":         token != "",
		"torznab_url":        tzURL,
		"torznab_key_masked": tzKeyMasked,
		"torznab_configured": strings.TrimSpace(tzURL) != "",
	})
}

type configReq struct {
	IYUUToken  string `json:"iyuu_token" binding:"max=256"`
	TorznabURL string `json:"torznab_url" binding:"max=512"`
	TorznabKey string `json:"torznab_apikey" binding:"max=256"`
	// HasTorznab 为 true 时才写 Torznab（避免仅改 IYUU 时清空端点）
	SetTorznab bool `json:"set_torznab"`
}

// SetConfig 保存辅种配置（IYUU token / 可选 Torznab）。
func (h *KeepSeedHandler) SetConfig(c *gin.Context) {
	var req configReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if tok := strings.TrimSpace(req.IYUUToken); tok != "" {
		if err := h.ks.SetToken(tok); err != nil {
			errcode.Respond(c, errcode.Internal.WithErr(err))
			return
		}
	}
	if req.SetTorznab {
		if err := h.ks.SetTorznab(req.TorznabURL, req.TorznabKey); err != nil {
			errcode.Respond(c, errcode.Internal.WithErr(err))
			return
		}
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	created, err := h.ks.Reseed(c.Request.Context(), req.DownloaderID)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": created, "count": len(created)})
}

type missingReq struct {
	DownloaderID  uint   `json:"downloader_id" binding:"required,min=1"`
	TargetSiteIDs []uint `json:"target_site_ids"`
}

// Missing 未辅种检查：对某下载器的做种，检查目标站点是否已辅种（IYUU hash 跨站匹配），返回未辅种清单。
func (h *KeepSeedHandler) Missing(c *gin.Context) {
	var req missingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	summary, err := h.ks.FindMissing(c.Request.Context(), req.DownloaderID, req.TargetSiteIDs)
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, summary)
}

// Tasks 列出辅种候选任务（?status=pending|added|skipped|failed；?site_id= 按候选站过滤）。
func (h *KeepSeedHandler) Tasks(c *gin.Context) {
	var siteID uint
	if s := c.Query("site_id"); s != "" {
		if n, err := strconv.ParseUint(s, 10, 64); err == nil && n > 0 {
			siteID = uint(n)
		}
	}
	tasks, err := h.ks.ListTasks(c.Query("status"), siteID)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	var req resolveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.ks.Resolve(c.Request.Context(), uint(id), req.Action); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetSetting 返回该下载器的辅种设置（auto_reseed / auto_reseed_add / save_path / pause_after_add）。
func (h *KeepSeedHandler) GetSetting(c *gin.Context) {
	id, err := strconv.ParseUint(c.Query("downloader_id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	s, err := h.ks.Setting(uint(id))
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
