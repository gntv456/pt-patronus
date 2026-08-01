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
	Source       string  `json:"source" binding:"max=16"`
	SourceID     string  `json:"source_id" binding:"max=64"`
	Title        string  `json:"title" binding:"max=255"`
	Year         int     `json:"year"`
	Type         string  `json:"type" binding:"max=16"`
	Category     string  `json:"category" binding:"max=16"`
	PosterURL    string  `json:"poster_url" binding:"max=512"`
	Resolution   *string `json:"resolution" binding:"omitempty,max=32"`
	DownloaderID *uint   `json:"downloader_id"`
	SavePath     *string `json:"save_path" binding:"omitempty,max=255"`
	Status       string  `json:"status" binding:"max=16"`
	// EnableRatingGuard 用指针以区分「未提供」与「显式 false」：Create nil 继承全局默认；Update nil 不改。
	EnableRatingGuard *bool `json:"enable_rating_guard"`
	// FilterGroups 过滤规则组名（管道分隔）；nil=不改，空串=清空（回退全局 SubscribeFilterRuleGroups）。
	// omitempty：nil 指针跳过 max 校验（否则 validator 对 nil *string 报 max 失败，致正常请求被 400）。
	FilterGroups *string `json:"filter_groups" binding:"omitempty,max=255"`
	// 洗版（best_version）：nil 在 Create 时继承全局默认，在 Update 时不改。
	BestVersion     *bool `json:"best_version"`
	BestVersionFull *bool `json:"best_version_full"`
	// OfficialOnly 仅官种：nil 在 Create 时继承全局默认，在 Update 时不改；清空回全局默认走 reset 端点。
	OfficialOnly *bool `json:"official_only"`
	// PromoTypes 接受的促销类型多选：非 nil 即赋值（空 slice=不限=不过滤，未传字段=不改）。
	PromoTypes []string `json:"promo_types"`
	// SiteIDs 参与站点 id：非 nil 即赋值（空 slice=不限全部站，未传字段=不改）。
	SiteIDs []uint `json:"site_ids"`
}

// List 列出全部订阅。
func (h *SubscribeHandler) List(c *gin.Context) {
	ss, err := h.subCh.ListWithPosters(c.Request.Context())
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
		Status: in.Status,
		// *bool 直接赋值：nil=未设置（Check 时回退全局默认 SubscribeGlobalDefaults），true/false=订阅自定义。
		EnableRatingGuard: in.EnableRatingGuard,
		BestVersion:       in.BestVersion,
		OfficialOnly:      in.OfficialOnly,
	}
	if in.Resolution != nil {
		s.Resolution = *in.Resolution
	}
	if in.DownloaderID != nil {
		s.DownloaderID = *in.DownloaderID
	}
	if in.SavePath != nil {
		s.SavePath = *in.SavePath
	}
	if in.FilterGroups != nil {
		s.FilterGroups = *in.FilterGroups
	}
	if in.BestVersionFull != nil {
		s.BestVersionFull = *in.BestVersionFull
	}
	if in.PromoTypes != nil {
		s.PromoTypes = models.StringSlice(in.PromoTypes)
	}
	if in.SiteIDs != nil {
		s.SiteIDs = models.IntSlice(in.SiteIDs)
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
	// *bool：nil=不改，非 nil=设订阅自定义值；清空回全局默认走 reset 端点。
	if in.EnableRatingGuard != nil {
		s.EnableRatingGuard = in.EnableRatingGuard
	}
	if in.FilterGroups != nil {
		s.FilterGroups = *in.FilterGroups
	}
	if in.BestVersion != nil {
		s.BestVersion = in.BestVersion
	}
	if in.OfficialOnly != nil {
		s.OfficialOnly = in.OfficialOnly
	}
	if in.BestVersionFull != nil {
		s.BestVersionFull = *in.BestVersionFull
	}
	if in.PromoTypes != nil {
		s.PromoTypes = models.StringSlice(in.PromoTypes)
	}
	if in.SiteIDs != nil {
		s.SiteIDs = models.IntSlice(in.SiteIDs)
	}
	// 指针区分“未提供”与“显式清空/归零”。
	if in.Resolution != nil {
		s.Resolution = *in.Resolution
	}
	if in.Status != "" {
		s.Status = in.Status
	}
	if in.DownloaderID != nil {
		s.DownloaderID = *in.DownloaderID
	}
	if in.SavePath != nil {
		s.SavePath = *in.SavePath
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

// GetGlobalSettings 读全局订阅默认设置（未单独设置的订阅回退到此）。
func (h *SubscribeHandler) GetGlobalSettings(c *gin.Context) {
	c.JSON(http.StatusOK, h.subCh.LoadGlobalDefaults())
}

// UpdateGlobalSettings 写全局订阅默认设置。
func (h *SubscribeHandler) UpdateGlobalSettings(c *gin.Context) {
	var gd chain.GlobalDefaults
	if err := c.ShouldBindJSON(&gd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.subCh.SaveGlobalDefaults(gd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gd)
}

// ResetFields 清空订阅指定字段使其回退到全局默认。
// 支持 enable_rating_guard / best_version / resolution / promo_types / site_ids。
// bool 字段的「nil=未设置」无法经 Update 表达（Update 的 *bool nil=不改），故提供此端点。
func (h *SubscribeHandler) ResetFields(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	s, err := h.subCh.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return
	}
	var req struct {
		Fields []string `json:"fields"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, f := range req.Fields {
		switch f {
		case "enable_rating_guard":
			s.EnableRatingGuard = nil
		case "best_version":
			s.BestVersion = nil
		case "official_only":
			s.OfficialOnly = nil
		case "resolution":
			s.Resolution = ""
		case "promo_types":
			s.PromoTypes = nil
		case "site_ids":
			s.SiteIDs = nil
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown field: " + f})
			return
		}
	}
	if err := h.subCh.Update(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}
