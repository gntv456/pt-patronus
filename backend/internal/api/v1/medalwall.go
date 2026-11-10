// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// MedalHandler 勋章墙（跨站勋章聚合 + 购买/佩戴/摘下）。
type MedalHandler struct {
	ch     *chain.MedalChain
	siteOp *oper.SiteOper
}

// NewMedalHandler 构造。
func NewMedalHandler(ch *chain.MedalChain, siteOp *oper.SiteOper) *MedalHandler {
	return &MedalHandler{ch: ch, siteOp: siteOp}
}

// All 返回全站勋章聚合（?refresh=1 强制刷新）。nil 切片归一为空数组，防前端 JSON null。
func (h *MedalHandler) All(c *gin.Context) {
	force := c.Query("refresh") == "1"
	// The shared 15s aggregate timeout truncates queued medal sites. Each site is
	// already bounded by MedalChain, so let the caller control the overall wait.
	cards, err := h.ch.MedalAll(c.Request.Context(), force)
	if err != nil {
		// 站点清单读不出要报错：以前回 200 空数组，与「没配站点」在界面上同形。
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if cards == nil {
		cards = []chain.MedalCard{}
	}
	c.JSON(http.StatusOK, gin.H{"medals": cards})
}

// AllStream SSE 流式推送全站勋章（?refresh=1）。每站完成即推一帧 event:batch
// （results 为该站多张卡，失败站为单张错误卡）；结束推 event:done（清单读失败附 list_error）。
func (h *MedalHandler) AllStream(c *gin.Context) {
	force := c.Query("refresh") == "1"
	emit := beginSSE(c.Writer)
	listErr := h.ch.MedalAllStream(c.Request.Context(), force, func(cards []chain.MedalCard) {
		emit("batch", gin.H{"results": cards})
	})
	done := gin.H{}
	if listErr != "" {
		done["list_error"] = listErr
	}
	emit("done", done)
}

// medalActionBody 写操作请求体。
type medalActionBody struct {
	SiteID  uint   `json:"site_id" binding:"min=1"`
	MedalID string `json:"medal_id" binding:"max=64"`
}

// Buy 购买勋章。
func (h *MedalHandler) Buy(c *gin.Context) { h.doAction(c, "buy") }

// Wear 佩戴勋章。
func (h *MedalHandler) Wear(c *gin.Context) { h.doAction(c, "wear") }

// TakeOff 摘下勋章。
func (h *MedalHandler) TakeOff(c *gin.Context) { h.doAction(c, "takeoff") }

func (h *MedalHandler) doAction(c *gin.Context, action string) {
	var body medalActionBody
	if err := c.ShouldBindJSON(&body); err != nil || body.MedalID == "" {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	s, err := h.siteOp.Get(body.SiteID)
	if err != nil {
		respondSiteLookup(c, err, h.siteOp)
		return
	}
	ctx := c.Request.Context()
	var success bool
	var message string
	switch action {
	case "buy":
		r, _ := h.ch.Buy(ctx, s, body.MedalID)
		success, message = r.Success, r.Message
	case "wear":
		r, _ := h.ch.Wear(ctx, s, body.MedalID)
		success, message = r.Success, r.Message
	case "takeoff":
		r, _ := h.ch.TakeOff(ctx, s, body.MedalID)
		success, message = r.Success, r.Message
	}
	c.JSON(http.StatusOK, gin.H{"success": success, "message": message})
}
