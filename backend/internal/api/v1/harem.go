// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// HaremHandler 后宫（邀请库存 + 被邀请人花名册）。
type HaremHandler struct {
	ch     *chain.SiteChain
	siteOp *oper.SiteOper
}

// NewHaremHandler 构造。
func NewHaremHandler(ch *chain.SiteChain, siteOp *oper.SiteOper) *HaremHandler {
	return &HaremHandler{ch: ch, siteOp: siteOp}
}

// All 返回全站后宫聚合（?refresh=1 强制刷新）。
func (h *HaremHandler) All(c *gin.Context) {
	force := c.Query("refresh") == "1"
	ctx, cancel := aggregateRequestContext(c)
	defer cancel()
	res, err := h.ch.HaremAll(ctx, force)
	if err != nil {
		// 站点清单读不出要报错：以前回 200 空数组，与「没配站点」在界面上同形。
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, res)
}

// AllStream SSE 流式推送全站后宫（?refresh=1）。每站完成即推一帧 event:batch
// （results 为单元素数组，该站药单）；结束推 event:done（带 overview 全量汇总，
// 站点清单读失败时附 list_error——清单读不出与「全部空闲」不能同形）。
func (h *HaremHandler) AllStream(c *gin.Context) {
	force := c.Query("refresh") == "1"
	emit := beginSSE(c.Writer)
	overview, listErr := h.ch.HaremAllStream(c.Request.Context(), force, func(item site.PrescriptionItem) {
		emit("batch", gin.H{"results": []site.PrescriptionItem{item}})
	})
	done := gin.H{"overview": overview}
	if listErr != "" {
		done["list_error"] = listErr
	}
	emit("done", done)
}

// SyncOne 立即抓取单站后宫（药单+花名册）。
func (h *HaremHandler) SyncOne(c *gin.Context) {
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
	ctx := c.Request.Context()
	status, e1 := h.ch.HaremStatus(ctx, s)
	invitees, e2 := h.ch.HaremInvitees(ctx, s)
	errMsg := ""
	switch {
	case errors.Is(e1, chain.ErrNotSupported) || errors.Is(e2, chain.ErrNotSupported):
		errMsg = "该站不支持邀请页抓取"
	case e1 != nil:
		errMsg = userErrText(e1)
	case e2 != nil:
		errMsg = userErrText(e2)
	}
	c.JSON(http.StatusOK, chain.BuildPrescription(s.ID, s.Name, status, invitees, errMsg))
}

// Export 返回药单文本（站点[X]: 剩余[N]个. 可购买[N]个. 权限[✓/✗]，CRLF）。
func (h *HaremHandler) Export(c *gin.Context) {
	force := c.Query("refresh") == "1"
	ctx, cancel := aggregateRequestContext(c)
	defer cancel()
	res, err := h.ch.HaremAll(ctx, force)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	totalRemain, totalCanBuy, failed, text := chain.ExportPrescription(res.Prescriptions)
	c.JSON(http.StatusOK, gin.H{
		"total_remain": totalRemain, "total_can_buy": totalCanBuy,
		"failed_count": failed, "text": text,
	})
}
