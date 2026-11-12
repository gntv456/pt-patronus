// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/entertainment"
	"github.com/hxpt/ptpatronus/internal/errtext"
)

type betReq struct {
	Selection string            `json:"selection" binding:"max=64"`
	Amount    float64           `json:"amount"`
	Extra     map[string]string `json:"extra" binding:"dive,max=512"`
}

// Bet 下注/选号（猜大小/骰宝/双色球/翻面/Betting/红包）。
func (h *EntertainmentHandler) Bet(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req betReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	res, err := h.ent.Bet(c.Request.Context(), id, c.Param("game"), entertainment.BetRequest{
		Selection: req.Selection, Amount: req.Amount, Extra: req.Extra,
	})
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// Events 列竞猜主题（赛事押注，如 OurBits 菠菜）。
func (h *EntertainmentHandler) Events(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.Events(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		entReadErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": res})
}

// EventDetail 某主题盘口（选项+赔率）。
func (h *EntertainmentHandler) EventDetail(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	eventID, err := strconv.Atoi(c.Param("eventID"))
	if err != nil || eventID <= 0 {
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	res, err := h.ent.EventDetail(c.Request.Context(), id, c.Param("game"), eventID)
	if err != nil {
		entReadErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type blackjackReq struct {
	Action string `json:"action" binding:"required,oneof=new hit stand double"`
}

// Blackjack 二十一点动作（new/hit/stand/double）。
func (h *EntertainmentHandler) Blackjack(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req blackjackReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	res, err := h.ent.Blackjack(c.Request.Context(), id, c.Param("game"), req.Action)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// FlipState 翻面夺宝全景（PandaPT：当前轮次/余额/100 格状态/奖项/记录）。
func (h *EntertainmentHandler) FlipState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.FlipState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		entReadErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// FlipWS 翻面夺宝实时事件流（站点 wss 桥接）。
// ?ticket=<一次性> 鉴权 → 升级 → 后端用站点 cookie 连 wss://host/wss，把 tileFlip 事件转发客户端。
// 客户端断开 → 关上游；每客户端连接 = 一条上游 WS（正常用量，非攻击）。
func (h *EntertainmentHandler) FlipWS(c *gin.Context) {
	if _, ok := h.tickets.Consume(c.Query("ticket")); !ok {
		errcode.Respond(c, errcode.TicketInvalidOrExpired)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	upgrader := websocket.Upgrader{ReadBufferSize: 256, WriteBufferSize: 256, CheckOrigin: h.checkOrigin}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade 已写错误响应
	}
	defer conn.Close()
	// ctx 生命周期绑定客户端连接：客户端断开或请求取消 → 取消上游桥接
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	events, err := h.ent.FlipBridge(ctx, id, c.Param("game"))
	if err != nil {
		// WS 错误帧同样走 errtext 单一文案源：err.Error() 直发会把英文内部错误
		// 弹给游戏页（HTTP 面已收敛，这条 WS 旁路是 #112 说的「改了这边漏那边」）。
		_ = conn.WriteJSON(gin.H{"type": "error", "message": errtext.Text(err)})
		return
	}
	// 读泵：仅用于探测客户端断开（读到错即取消 ctx，关上游）
	safego.Go("entertainment:582", func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	})
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return // 上游关闭 events
			}
			if err := conn.WriteJSON(ev); err != nil {
				return // 客户端写失败
			}
		case <-ctx.Done():
			return // 客户端断开/请求取消（避免半开连接 events 停滞时永久阻塞 goroutine）
		}
	}
}

// BigSmallState 猜大小全景快照（走势/排行/记录/当前局/任务/道具/聊天/红包）。
func (h *EntertainmentHandler) BigSmallState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.BigSmallState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		entReadErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// BigSmallChat 拉新聊天（?since=N；好学 chat_get last_id，熊猫 getLastMsg 忽略 since）。
func (h *EntertainmentHandler) BigSmallChat(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	since, _ := strconv.Atoi(c.DefaultQuery("since", "0"))
	res, err := h.ent.BigSmallChat(c.Request.Context(), id, c.Param("game"), since)
	if err != nil {
		entReadErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"chat": res})
}

type bsSendChatReq struct {
	Message string `json:"message" binding:"required,max=2000"`
}

// BigSmallSendChat 发聊天。
func (h *EntertainmentHandler) BigSmallSendChat(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req bsSendChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	res, err := h.ent.BigSmallSendChat(c.Request.Context(), id, c.Param("game"), req.Message)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type bsTaskReq struct {
	Action string `json:"action" binding:"required,oneof=claim reward refund"`
	TaskID int    `json:"task_id" binding:"min=1,max=100000"`
}

// BigSmallTaskAction 任务领取/领奖/亏返卡。
func (h *EntertainmentHandler) BigSmallTaskAction(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req bsTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	res, err := h.ent.BigSmallTaskAction(c.Request.Context(), id, c.Param("game"), req.Action, req.TaskID)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type bsItemReq struct {
	Tag string `json:"tag" binding:"required,max=64"`
}

// BigSmallUseItem 使用道具（熊猫 useItem；好学亏返卡 refund）。
func (h *EntertainmentHandler) BigSmallUseItem(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req bsItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	res, err := h.ent.BigSmallUseItem(c.Request.Context(), id, c.Param("game"), req.Tag)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type bsRedpacketReq struct {
	Action string            `json:"action" binding:"required,oneof=send claim info"`
	Extra  map[string]string `json:"extra" binding:"dive,max=512"`
}

// BigSmallRedpacket 红包发送/领取/详情。
func (h *EntertainmentHandler) BigSmallRedpacket(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req bsRedpacketReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	res, err := h.ent.BigSmallRedpacket(c.Request.Context(), id, c.Param("game"), req.Action, req.Extra)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
