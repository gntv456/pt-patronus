package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// gobangSiteErr 站点侧错误分类：cookie 失效/未登录（站点 message 含「登录」）→
// SiteAuthRejected（提示更新站点 cookie），其余透传 BadGateway（含站点原文案日志）。
func gobangSiteErr(c *gin.Context, err error) {
	if err != nil && strings.Contains(err.Error(), "请先登录") {
		errcode.Respond(c, errcode.SiteAuthRejected.WithErr(err))
		return
	}
	errcode.Respond(c, errcode.BadGateway.WithErr(err))
}

// GobangState 五子棋全景快照（蜂巢：对局列表/蜂蜜汇总/周榜）。
func (h *EntertainmentHandler) GobangState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.GobangState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		gobangSiteErr(c, err)
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
		if req.Params == nil && (req.Action == "move" || req.Action == "choose-color" || req.Action == "undo" || req.Action == "forfeit" || req.Action == "retry-ai") {
			// 动作需要 params（matchId 等）却没带：站点会拒，先在前端可读地拦下。
			errcode.Respond(c, errcode.InvalidInput.WithErr(err))
			return
		}
		gobangSiteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// FriendTradeState 好友买卖全景快照（PANDA：身价/主人/奴隶/收益/任务/动态）。只读。
func (h *EntertainmentHandler) FriendTradeState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.FriendTradeState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		gobangSiteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// FishingState 钓鱼全景快照（思齐：钓次/装备/图鉴）。只读。
func (h *EntertainmentHandler) FishingState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.FishingState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		gobangSiteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// City13WheelState 13City 大转盘全景（余额/今日次数/奖品环/最近开奖）。只读。
func (h *EntertainmentHandler) City13WheelState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.City13WheelState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		gobangSiteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// City13DiceState 13City 幸运骰子全景（余额/玩法/最近开奖）。只读。
func (h *EntertainmentHandler) City13DiceState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.City13DiceState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		gobangSiteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
