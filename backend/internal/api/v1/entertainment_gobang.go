package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// GobangState 五子棋全景快照（蜂巢：对局列表/蜂蜜汇总/周榜）。
func (h *EntertainmentHandler) GobangState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.GobangState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, res)
}

type gobangActionReq struct {
	// Action 合法动作：create / choose-color / move / undo / forfeit / retry-ai。
	Action string `json:"action" binding:"required,oneof=create choose-color move undo forfeit retry-ai"`
	// Params 动作参数原样透传站点：create{difficulty}、choose-color{matchId,color}、
	// move{matchId,x,y}、undo/forfeit/retry-ai{matchId}。
	Params map[string]any `json:"params" binding:"dive,max=512"`
}

// GobangAction 五子棋动作（花钱操作走 spendGuard，重复提交 → 429）。
func (h *EntertainmentHandler) GobangAction(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req gobangActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	res, err := h.ent.GobangAction(c.Request.Context(), id, c.Param("game"), req.Action, req.Params)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
