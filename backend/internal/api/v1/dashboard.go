package v1

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
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
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, overview)
}

// Sync 全站同步（后台化，二轮 P1-3 修复）：50 站串行同步轻松超过前端 axios 15s
// 超时——此前沿用请求 ctx，客户端断开后剩余站点全部计入失败指标（metrics 污染）、
// 零数据落库，而 WS 进度帧照发（进度条显示走满但实际全失败）。
// 改为独立 15min ctx 立即返回受理；进度经 WS task-progress 实时推送（已具备），
// 数据结果由前端通过轮询 overview / 下一次加载看到。
func (h *DashboardHandler) Sync(c *gin.Context) {
	accepted := h.dash.StartSyncAll()
	if !accepted {
		// 已有同步在跑：直接返回当前总览（幂等，不报错）
		if overview, err := h.dash.Overview(c.Request.Context()); err == nil {
			c.JSON(http.StatusOK, overview)
			return
		}
		c.JSON(http.StatusOK, gin.H{"syncing": true})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"syncing": true})
}

// SyncOne 立即抓取单个站点统计（站点明细「刷新数据」按钮）。
func (h *DashboardHandler) SyncOne(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.SiteInvalidID)
		return
	}
	overview, err := h.dash.SyncOne(c.Request.Context(), uint(id))
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
		errcode.Respond(c, errcode.SiteInvalidID)
		return
	}
	days := clampDays(c.Query("days"), 30, 1, 365)
	pts, err := h.dash.History(c.Request.Context(), uint(id), days)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, pts)
}

// Trend 返回按天聚合的趋势序列（?days=N，默认 7，上限 90）。
func (h *DashboardHandler) Trend(c *gin.Context) {
	days := clampDays(c.Query("days"), 7, 1, 90)
	trend, err := h.dash.Trend(c.Request.Context(), days)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
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
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, wb)
}
