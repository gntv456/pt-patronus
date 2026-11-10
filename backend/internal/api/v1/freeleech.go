// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
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

// All 返回所有启用站点的站免聚合（带缓存；?refresh=1 强制刷新）。清单读失败回 errcode，不渲染成「全部站无站免」。
func (h *FreeleechHandler) All(c *gin.Context) {
	force := c.Query("refresh") == "1"
	results, err := h.ch.FreeleechAll(c.Request.Context(), force)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if results == nil {
		results = []chain.FreeleechResult{}
	}
	c.JSON(http.StatusOK, gin.H{"sites": results})
}

// AllStream 流式返回站免（NDJSON，每站一行）：每站抓完即 flush，前端"加载一个显示一个"，
// 不等全部站抓完。慢站（context deadline 15s）不会阻塞已就绪站的展示；首次进页面不再因
// 阻塞等全量而超过前端 timeout 显示失败（与公告 AllStream 同范式）。
func (h *FreeleechHandler) AllStream(c *gin.Context) {
	force := c.Query("refresh") == "1"
	w := c.Writer
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // 关反代缓冲，每站即时到前端
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	var mu sync.Mutex // emit 并发回调，写 ResponseWriter 需串行化
	emit := func(r chain.FreeleechResult) {
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
	// 流末尾补 done 行：站点清单读失败时带 list_error（前端识别后抛错留痕，不再「0 站全绿」）。
	listErr := h.ch.FreeleechStream(c.Request.Context(), force, emit)
	if listErr != "" {
		emitDone(w, &mu, flusher, listErr)
	}
}

// SyncOne 立即抓取单个站点站免并返回（站点卡片「刷新」按钮）。
func (h *FreeleechHandler) SyncOne(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.SiteInvalidID)
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		respondSiteLookup(c, err, h.siteOp)
		return
	}
	// 交回链层的单站组装口径：中文原因 + 「没接线」判定 + notices 归一（handler 自己拼
	// 会把 e.Error() 英文原文弹给用户，且未适配的站退化成「没有促销」）。
	c.JSON(http.StatusOK, h.ch.SyncFreeleech(c.Request.Context(), s))
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

// All 返回所有启用站点的公告聚合（带缓存；?refresh=1 强制刷新）。清单读失败回 errcode，不渲染成「全部站无公告」。
func (h *AnnouncementHandler) All(c *gin.Context) {
	force := c.Query("refresh") == "1"
	results, err := h.ch.AnnouncementsAll(c.Request.Context(), force)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if results == nil {
		results = []chain.AnnouncementResult{}
	}
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
	// 流末尾补 done 行：站点清单读失败时带 list_error（前端识别后抛错留痕，不再「0 站全绿」）。
	listErr := h.ch.AnnouncementsStream(c.Request.Context(), force, emit)
	if listErr != "" {
		emitDone(w, &mu, flusher, listErr)
	}
}

// emitDone 向 NDJSON 流末尾补一行 {"done":true,"list_error":...}（清单读失败留痕；"" 则不打扰）。
func emitDone(w gin.ResponseWriter, mu *sync.Mutex, flusher http.Flusher, listErr string) {
	b, err := json.Marshal(gin.H{"done": true, "list_error": listErr})
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

// SyncOne 立即抓取单个站点公告并返回。
func (h *AnnouncementHandler) SyncOne(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.SiteInvalidID)
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		respondSiteLookup(c, err, h.siteOp)
		return
	}
	// 同站免：交回链层口径（中文原因 + 未接线判定 + items 归一 + 回写缓存）。
	c.JSON(http.StatusOK, h.ch.SyncAnnouncement(c.Request.Context(), s))
}
