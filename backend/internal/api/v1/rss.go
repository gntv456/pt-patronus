// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
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
	// interval_min 的归一从前只在 Create 做一次 ⇒ 编辑（以及 AI 工具/批量 PUT 不带这个键）会把 0 写进库。
	// 0 不会把节奏改坏（CheckAll 与插件工具各自兜底成 30 分钟），但这一行会被两个客户端原样回显：
	// Web Rss.vue「间隔 0 分钟」、移动 rss_page.dart「间隔 ${intervalMin} 分钟」，而实际按 30 抓 ⇒
	// 账面与生效不一致。判定收在这一处，新建与编辑共用（读侧兜底保留，护住存量行）。
	if in.IntervalMin <= 0 {
		in.IntervalMin = 30
	}
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
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscriptions": rs})
}

// Create 新建。
func (h *RssHandler) Create(c *gin.Context) {
	var in rssReq
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	if in.Name == "" || in.URL == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	r := &models.RssSubscription{}
	applyRssReq(r, in)
	out, err := h.ch.Create(r)
	if err != nil {
		// 链层会透传 oper 的「写给人看」输入原因（地址协议不对等），必须走分类层：
		// 从前这里一律 errcode.Internal ⇒ 用户笔误被报成 500「服务器内部错误」，中文原因只进日志。
		respondInputError(c, err, errcode.Internal)
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
		// #347 与本文件 Delete 的 respondRecordError 同一判据：手抄的
		// Respond(RssSubscriptionGone) 只判「不存在」，不判 DB 真故障
		// （RssChain.Get 底层是 db.First）。
		respondRecordError(c, err, errcode.RssSubscriptionGone, errcode.InvalidInput, errcode.Internal)
		return
	}
	var in rssReq
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	applyRssReq(r, in)
	if err := h.ch.Update(r); err != nil {
		respondInputError(c, err, errcode.Internal) // 同上：输入原因走 400+人话，真故障才 500
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
		// 存在性判定在链层（HTTP / AI 工具 / 批量取消共用），这里只做归类：
		// 「本来就没有」必须区别于「真失败」，否则用户对着 500 反复点。
		respondRecordError(c, err, errcode.RssSubscriptionGone, errcode.InvalidInput, errcode.Internal)
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
		// #347 与本文件 Delete 的 respondRecordError 同一判据：手抄的
		// Respond(RssSubscriptionGone) 只判「不存在」，不判 DB 真故障
		// （RssChain.Get 底层是 db.First）。
		respondRecordError(c, err, errcode.RssSubscriptionGone, errcode.InvalidInput, errcode.Internal)
		return
	}
	res, err := h.ch.Check(c.Request.Context(), r)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "new_count": 0, "error": userErrText(err)})
		return
	}
	// scanned/total/truncated 与 new_count 同行返回：本轮只看窗口内的条目，不把上限
	// 说成「整个源都查过了」（窗口截断时新发现数天然是上限内的数）。
	c.JSON(http.StatusOK, gin.H{
		"ok": true, "new_count": res.New,
		"scanned": res.Scanned, "total": res.Total, "truncated": res.Truncated,
		// 自动下载失败的条数与原因：这些条目已回滚、下轮重试，不能只报「新增 N 条」——
		// 条目列表是空的，用户看不到任何失败迹象。
		"download_failed": res.DownloadFailed, "download_errors": res.DownloadErrors,
	})
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
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// total/truncated 与清单同批给出：条目列表有上限（默认 100、封顶 500），只回 items 时
	// 「还有 300 条没显示」在界面上与「就这些」同形。口径对齐 keepseed：计数失败宁可保守
	// 报「可能被截断」，也不给出「这是全部」的假安心，并且不给一个假的 total。
	total, cntErr := h.ch.CountItems(id)
	truncated := cntErr != nil || total > int64(len(its))
	if cntErr != nil {
		slog.Warn("rss item count failed", "id", id, "err", cntErr)
	}
	resp := gin.H{"items": its, "limit": limit, "truncated": truncated}
	if cntErr == nil {
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}
