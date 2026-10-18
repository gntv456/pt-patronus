package v1

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/entertainment"
)

// gobangSiteErr 站点侧错误分类：cookie 失效/未登录（站点 message 含「登录」）→
// SiteAuthRejected（提示更新站点 cookie），其余交娱乐域公共判据。
//
// 从前兜底是一句裸 BadGateway「上游服务不可用，请稍后重试」：这九个只读全景出口
// 于是把「这个游戏没有五子棋面板」（该换游戏）、「站点凭据解不开」（该去重新保存，
// 而且确实没发请求）和真的网络故障说成同一句话，用户只能反复重试。
func gobangSiteErr(c *gin.Context, err error) {
	if err != nil && strings.Contains(err.Error(), "请先登录") {
		errcode.Respond(c, errcode.SiteAuthRejected.WithErr(err))
		return
	}
	entReadErr(c, err)
}

// gobangSpendSiteErr 同款判据，但这条路会消耗游戏币/蜂蜜：扣费语义由 spendErr 交代。
func gobangSpendSiteErr(c *gin.Context, err error) {
	if err != nil && strings.Contains(err.Error(), "请先登录") {
		errcode.Respond(c, errcode.SiteAuthRejected.WithErr(err))
		return
	}
	spendErr(c, err)
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
	// Action 合法动作：create / choose-color / select / move / undo / forfeit / retry-ai。
	Action string `json:"action" binding:"required,oneof=create choose-color select move undo forfeit retry-ai"`
	// Params 动作参数原样透传站点：create{difficulty}、choose-color{matchId,color}、
	// select{matchId,x,y,turnToken}（计榜局换 moveToken）、move{matchId,x,y|moveToken}、
	// undo/forfeit/retry-ai{matchId}。
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
	if req.Params == nil && req.Action != "create" {
		// 动作需要 params（matchId 等）却没带：站点会拒，先在前端可读地拦下。
		errcode.Respond(c, errcode.InvalidInput.WithErr(fmt.Errorf("动作 %s 缺少 params", req.Action)))
		return
	}
	res, err := h.ent.GobangAction(c.Request.Context(), id, c.Param("game"), req.Action, req.Params)
	if err != nil {
		// 双击/重放（ErrDuplicateSpend → 429）已在 gobangSpendSiteErr 的公共判据里，
		// 这里从前又抄了一遍，两处措辞一改就漂移。
		gobangSpendSiteErr(c, err)
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

// CbgWheelState 藏宝阁大转盘全景。只读。
func (h *EntertainmentHandler) CbgWheelState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.CbgWheelState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		gobangSiteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// CbgSlotState 藏宝阁老虎机全景。只读。
func (h *EntertainmentHandler) CbgSlotState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.CbgSlotState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		gobangSiteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// TangptLotteryState 躺平抽奖全景。只读。
func (h *EntertainmentHandler) TangptLotteryState(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.TangptLotteryState(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		gobangSiteErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type gobangHintReq struct {
	Board [][]int `json:"board" binding:"required,max=32,dive,max=32,dive,min=0,max=2"`
	Mine  int     `json:"mine" binding:"required,min=1,max=2"`
}

// GobangHint 五子棋引擎着法提示（本地引擎计算，不请求站点）：
// 驱动脚本把站点棋盘 POST 进来，返回引擎选点。供自动化对局使用。
func (h *EntertainmentHandler) GobangHint(c *gin.Context) {
	var req gobangHintReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	theirs := 3 - req.Mine
	x, y, ok := entertainment.GobangBestMoveForHint(req.Board, req.Mine, theirs)
	if !ok {
		errcode.Respond(c, errcode.Internal.WithErr(fmt.Errorf("no move")))
		return
	}
	c.JSON(http.StatusOK, gin.H{"x": x, "y": y})
}
