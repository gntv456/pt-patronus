// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"fmt"

	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
)

// DownloaderHandler 处理下载器 CRUD 与任务操作。
type DownloaderHandler struct {
	op     *oper.DownloaderOper
	dl     *chain.DownloadChain
	mgr    *manager.Manager
	siteOp *oper.SiteOper // 手动加种 URL 反查站点用（cookie 下载 .torrent）
}

// NewDownloaderHandler 构造。
func NewDownloaderHandler(op *oper.DownloaderOper, dl *chain.DownloadChain, mgr *manager.Manager, siteOp *oper.SiteOper) *DownloaderHandler {
	return &DownloaderHandler{op: op, dl: dl, mgr: mgr, siteOp: siteOp}
}

// Caps 返回各下载器类型的能力位矩阵（后端权威，供前端按类型门控 UI 控件、避免显示后端
// 会 ErrNotImplemented 的按钮）。GET /downloaders/caps。
func (h *DownloaderHandler) Caps(c *gin.Context) {
	c.JSON(http.StatusOK, downloader.AllCapabilities())
}

// List 列出全部下载器。
func (h *DownloaderHandler) List(c *gin.Context) {
	ds, err := h.op.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"downloaders": ds})
}

// Get 取单个下载器详情。
func (h *DownloaderHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	d, err := h.op.Get(id)
	if err != nil {
		// #372 DownloaderOper.Get 底层是 db.First，它把「记录不存在」与「DB 真故障」
		// 原样返回；从前一律 404「下载器不存在」，于是数据库一挂，用户在详情页看到的
		// 也是「下载器不存在」，跑去刷新列表 —— 那页同样故障，而方向从头到尾是错的。
		// 与同文件 Update/Delete 用同一判据（这也是全仓最后一处手写查询出口）。
		respondRecordError(c, err, errcode.DownloaderNotFound, errcode.DownloaderInvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, gin.H{"downloader": d})
}

// Create 创建下载器。
// unknownDownloaderType 下载器类型不在能力登记表里：caps 会退成 minimalCaps，建出来是一条
// 什么操作都做不了的哑记录（typo 或旧客户端枚举不一致时最容易踩）。错误带上可选值清单。
func unknownDownloaderType(typ string) error {
	types := make([]string, 0, 8)
	for k := range downloader.AllCapabilities() {
		types = append(types, k)
	}
	sort.Strings(types)
	return fmt.Errorf("下载器类型 %q 不支持，可选：%s", typ, strings.Join(types, " | "))
}

func (h *DownloaderHandler) Create(c *gin.Context) {
	var in oper.DownloaderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	if !downloader.HasType(in.Type) {
		errcode.Respond(c, errcode.DownloaderInvalidInput.WithDetail(unknownDownloaderType(in.Type)))
		return
	}
	d, err := h.op.Create(in)
	if err != nil {
		respondInputErrorAs(c, err, errcode.DownloaderInvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, d)
}

// Update 更新下载器（失效缓存）。
func (h *DownloaderHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in oper.DownloaderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	if !downloader.HasType(in.Type) {
		errcode.Respond(c, errcode.DownloaderInvalidInput.WithDetail(unknownDownloaderType(in.Type)))
		return
	}
	d, err := h.op.Update(id, in)
	if err != nil {
		respondRecordError(c, err, errcode.DownloaderNotFound, errcode.DownloaderInvalidInput, errcode.Internal)
		return
	}
	h.mgr.Invalidate(id)
	c.JSON(http.StatusOK, d)
}

// Delete 删除下载器。
func (h *DownloaderHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if !requireExists(c, h.op.Get, id, errcode.DownloaderNotFound) {
		return
	}
	if err := h.op.Delete(id); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	h.mgr.Invalidate(id)
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Test 连接测试。
func (h *DownloaderHandler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.dl.Test(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Torrents 列出任务（支持 category/state/tag/q 过滤 + limit/offset 分页 + sort/order 排序）。
// 不传 query 时返回全部（仅多 total/limit/offset 元字段），向后兼容。
func (h *DownloaderHandler) Torrents(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ts, err := h.dl.List(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"torrents": []any{}, "total": 0, "error": userErrText(err)})
		return
	}
	ts = filterTorrents(ts, c)
	sortTorrents(ts, c)
	total := len(ts)
	limit := clampQueryInt(c, "limit", 200, 1, 1000)
	offset := clampQueryInt(c, "offset", 0, 0, total)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	c.JSON(http.StatusOK, gin.H{"torrents": ts[offset:end], "total": total, "limit": limit, "offset": offset})
}

type addReq struct {
	URL           string   `json:"url" binding:"required,max=2048"` // magnet 链较长，给宽上界
	Category      string   `json:"category" binding:"max=128"`
	Tags          []string `json:"tags" binding:"dive,max=128"`
	SavePath      string   `json:"save_path" binding:"max=512"`
	Paused        bool     `json:"paused"`
	UploadLimit   int64    `json:"upload_limit"`   // bytes/s, 0 不限
	DownloadLimit int64    `json:"download_limit"` // bytes/s, 0 不限
}

// Add 添加种子。
func (h *DownloaderHandler) Add(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req addReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	opts := downloader.AddOptions{
		URL: req.URL, Category: req.Category, Tags: req.Tags, SavePath: req.SavePath, Paused: req.Paused,
		UploadLimit: req.UploadLimit, DownloadLimit: req.DownloadLimit,
	}
	// 手动加种：URL 可能来自 PT 站（需 cookie 下载），反查站点设 SiteID 让 DownloadChain.Add 处理。
	if h.siteOp != nil {
		if st, _ := h.siteOp.FindByDownloadURL(req.URL); st != nil {
			opts.SiteID = st.ID
		}
	}
	err := h.dl.Add(c.Request.Context(), id, opts)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type hashesReq struct {
	Hashes      []string `json:"hashes" binding:"dive,max=64"` // 单元素上限防巨型 hash 串 DoS（infohash 40 hex）
	DeleteFiles bool     `json:"delete_files"`
}

// Pause 暂停任务。
func (h *DownloaderHandler) Pause(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if err := h.dl.Pause(c.Request.Context(), id, req.Hashes); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Resume 恢复任务。
func (h *DownloaderHandler) Resume(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if err := h.dl.Resume(c.Request.Context(), id, req.Hashes); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Remove 删除任务。
func (h *DownloaderHandler) Remove(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req hashesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if err := h.dl.Delete(c.Request.Context(), id, req.Hashes, req.DeleteFiles); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Stats 全局速率。
func (h *DownloaderHandler) Stats(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	st, err := h.dl.Stats(c.Request.Context(), id)
	if err != nil {
		// 502：下载器协议层故障（不可达/版本不支持），非客户端参数错误。
		errcode.Respond(c, errcode.DownloaderConnFailed.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, st)
}
