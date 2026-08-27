package v1

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
)

// DashboardHandler 返回仪表盘聚合数据。
type DashboardHandler struct {
	dash *chain.DashboardChain
}

// NewDashboardHandler 构造。
func NewDashboardHandler(d *chain.DashboardChain) *DashboardHandler {
	return &DashboardHandler{dash: d}
}

// Overview 聚合所有站点最新统计 + 下载器数。
func (h *DashboardHandler) Overview(c *gin.Context) {
	overview, err := h.dash.Overview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, overview)
}

// Sync 立即抓取所有启用站点统计并返回刷新后的总览（回填新加入站点的 0 数据）。
func (h *DashboardHandler) Sync(c *gin.Context) {
	overview, err := h.dash.SyncAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, overview)
}

// SyncOne 立即抓取单个站点统计（站点明细「刷新数据」按钮）。
func (h *DashboardHandler) SyncOne(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	overview, err := h.dash.SyncOne(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, overview)
}

// clampDays 把 ?days= 收敛到 [min,max]：空/非法→def，越界→最近的边界。
// 防止 days=999999 之类触发超大范围聚合查询拖垮 DB。
func clampDays(q string, def, min, max int) int {
	d, err := strconv.Atoi(q)
	if err != nil || d == 0 {
		return def
	}
	if d < min {
		return min
	}
	if d > max {
		return max
	}
	return d
}

// History 返回某站点最近 N 天的统计快照（站点明细「用户历史数据」按钮）。
func (h *DashboardHandler) History(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	days := clampDays(c.Query("days"), 30, 1, 365)
	pts, err := h.dash.History(c.Request.Context(), uint(id), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, pts)
}

// Trend 返回按天聚合的趋势序列（?days=N，默认 7，上限 90）。
func (h *DashboardHandler) Trend(c *gin.Context) {
	days := clampDays(c.Query("days"), 7, 1, 90)
	trend, err := h.dash.Trend(c.Request.Context(), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, trend)
}

// WorkbenchHandler 工作台聚合（媒体库态势需 MediaServerChain 枚举库计数）。
type WorkbenchHandler struct {
	dash *chain.DashboardChain
	ms   *chain.MediaServerChain // nil=跳过媒体库态势
}

// NewDashboardHandler2 构造（Workbench 专用）。
func NewDashboardHandler2(d *chain.DashboardChain, ms *chain.MediaServerChain) *WorkbenchHandler {
	return &WorkbenchHandler{dash: d, ms: ms}
}

// Workbench GET /dashboard/workbench：一次返回今日流转节点/待办/动态/摘要/媒体库态势/体检分。
func (h *WorkbenchHandler) Workbench(c *gin.Context) {
	var msLib func(ctx context.Context) chain.WorkbenchMediaLibrary
	if h.ms != nil {
		msCh := h.ms
		msLib = func(ctx context.Context) chain.WorkbenchMediaLibrary {
			return msCh.MediaLibraryOverview(ctx)
		}
	}
	wb, err := h.dash.Workbench(c.Request.Context(), msLib)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, wb)
}
