package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// FreeleechHandler 站点全站免费/促销检测。
type FreeleechHandler struct {
	ch     *chain.SiteChain
	siteOp *oper.SiteOper
}

// NewFreeleechHandler 构造。
func NewFreeleechHandler(ch *chain.SiteChain, siteOp *oper.SiteOper) *FreeleechHandler {
	return &FreeleechHandler{ch: ch, siteOp: siteOp}
}

// All 返回所有启用站点的站免聚合（带缓存；?refresh=1 强制刷新）。
func (h *FreeleechHandler) All(c *gin.Context) {
	force := c.Query("refresh") == "1"
	results := h.ch.FreeleechAll(c.Request.Context(), force)
	c.JSON(http.StatusOK, gin.H{"sites": results})
}

// SyncOne 立即抓取单个站点站免并返回（站点卡片「刷新」按钮）。
func (h *FreeleechHandler) SyncOne(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	notices, e := h.ch.Freeleech(c.Request.Context(), s)
	r := chain.FreeleechResult{SiteID: s.ID, SiteName: s.Name}
	if e != nil {
		r.Error = e.Error()
	}
	r.Notices = notices
	c.JSON(http.StatusOK, r.Normalize())
}

// AnnouncementHandler 站点公告。
type AnnouncementHandler struct {
	ch     *chain.SiteChain
	siteOp *oper.SiteOper
}

// NewAnnouncementHandler 构造。
func NewAnnouncementHandler(ch *chain.SiteChain, siteOp *oper.SiteOper) *AnnouncementHandler {
	return &AnnouncementHandler{ch: ch, siteOp: siteOp}
}

// All 返回所有启用站点的公告聚合（带缓存；?refresh=1 强制刷新）。
func (h *AnnouncementHandler) All(c *gin.Context) {
	force := c.Query("refresh") == "1"
	results := h.ch.AnnouncementsAll(c.Request.Context(), force)
	c.JSON(http.StatusOK, gin.H{"sites": results})
}

// AllStream 流式返回公告（NDJSON，每站一行）：每站解析完立即 flush，前端"加载一个显示一个"，
// 不等全部站抓完。慢站（context deadline 15s）不会阻塞已就绪站的展示。
func (h *AnnouncementHandler) AllStream(c *gin.Context) {
	force := c.Query("refresh") == "1"
	w := c.Writer
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // 关反代缓冲，每站即时到前端
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	var mu sync.Mutex // emit 并发回调，写 ResponseWriter 需串行化
	emit := func(r chain.AnnouncementResult) {
		b, err := json.Marshal(r)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
	h.ch.AnnouncementsStream(c.Request.Context(), force, emit)
}

// SyncOne 立即抓取单个站点公告并返回。
func (h *AnnouncementHandler) SyncOne(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	ann, e := h.ch.Announcements(c.Request.Context(), s)
	r := chain.AnnouncementResult{SiteID: s.ID, SiteName: s.Name, FetchedAt: time.Now()}
	if e != nil {
		r.Error = e.Error()
	}
	if ann != nil {
		r.RawText = ann.RawText
		r.Items = ann.Items
		r.FetchedAt = ann.FetchedAt
	}
	c.JSON(http.StatusOK, r.Normalize())
}
