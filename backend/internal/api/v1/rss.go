package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// RssHandler 处理 RSS 订阅 CRUD、手动检查与条目查看。
type RssHandler struct {
	ch *chain.RssChain
}

func NewRssHandler(ch *chain.RssChain) *RssHandler { return &RssHandler{ch: ch} }

// rssReq 创建/编辑订阅的请求体。
type rssReq struct {
	Name         string `json:"name" binding:"max=128"`
	URL          string `json:"url" binding:"max=512"`
	SiteID       uint   `json:"site_id"`
	Enabled      bool   `json:"enabled"`
	IntervalMin  int    `json:"interval_min"`
	TitleInclude string `json:"title_include" binding:"max=2000"`
	TitleExclude string `json:"title_exclude" binding:"max=2000"`
	MinSize      int64  `json:"min_size"`
	MaxSize      int64  `json:"max_size"`
	Resolution   string `json:"resolution" binding:"max=32"`
	AutoDownload bool   `json:"auto_download"`
	DownloaderID uint   `json:"downloader_id"`
	SavePath     string `json:"save_path" binding:"max=255"`
	Category     string `json:"category" binding:"max=64"`
	Tags         string `json:"tags" binding:"max=500"`
	FilterGroups string `json:"filter_groups" binding:"max=255"` // 绑定规则组（| 分隔，空=不过滤）
}

func applyRssReq(r *models.RssSubscription, in rssReq) {
	r.Name = in.Name
	r.URL = in.URL
	r.SiteID = in.SiteID
	r.Enabled = in.Enabled
	r.IntervalMin = in.IntervalMin
	r.TitleInclude = in.TitleInclude
	r.TitleExclude = in.TitleExclude
	r.MinSize = in.MinSize
	r.MaxSize = in.MaxSize
	r.Resolution = in.Resolution
	r.AutoDownload = in.AutoDownload
	r.DownloaderID = in.DownloaderID
	r.SavePath = in.SavePath
	r.Category = in.Category
	r.Tags = in.Tags
	r.FilterGroups = in.FilterGroups
}

// List 全部订阅。
func (h *RssHandler) List(c *gin.Context) {
	rs, err := h.ch.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscriptions": rs})
}

// Create 新建。
func (h *RssHandler) Create(c *gin.Context) {
	var in rssReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Name == "" || in.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称和 URL 必填"})
		return
	}
	if in.IntervalMin <= 0 {
		in.IntervalMin = 30
	}
	r := &models.RssSubscription{}
	applyRssReq(r, in)
	out, err := h.ch.Create(r)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// Update 编辑。
func (h *RssHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	r, err := h.ch.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "订阅不存在"})
		return
	}
	var in rssReq
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	applyRssReq(r, in)
	if err := h.ch.Update(r); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, r)
}

// Delete 删除。
func (h *RssHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.ch.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Check 手动检查单个订阅，返回本轮新发现条数。
func (h *RssHandler) Check(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	r, err := h.ch.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "订阅不存在"})
		return
	}
	n, err := h.ch.Check(c.Request.Context(), r)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "new_count": 0, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "new_count": n})
}

// ListItems 某订阅的条目。
func (h *RssHandler) ListItems(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 { // 上限封顶防 DoS
		limit = 500
	}
	its, err := h.ch.ListItems(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": its})
}
