// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/chain"
	"gorm.io/gorm"
)

// EntertainmentHandler 处理娱乐游戏。
type EntertainmentHandler struct {
	ent            *chain.EntertainmentChain
	tickets        *auth.TicketStore // flip-ws 一次性 ticket 鉴权
	allowedOrigins map[string]bool   // flip-ws CheckOrigin 白名单（防 CSWSH）
}

// NewEntertainmentHandler 构造。tickets/origins 供翻面夺宝 websocket 桥接端点用。
func NewEntertainmentHandler(ent *chain.EntertainmentChain, tickets *auth.TicketStore, allowedOrigins []string) *EntertainmentHandler {
	set := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" && o != "*" {
			set[o] = true
		}
	}
	return &EntertainmentHandler{ent: ent, tickets: tickets, allowedOrigins: set}
}

// checkOrigin 防 CSWSH。空白名单=开发全开语义（failClosedEmpty=false 保持旧行为）；
// same-origin 放行 + 跨域白名单的统一口径见 origin.go。
func (h *EntertainmentHandler) checkOrigin(r *http.Request) bool {
	return originAllowed(h.allowedOrigins, false, r)
}

// spendErr 处理"花钱"操作（Play/Buy/Draw/Synthesize）的错误：
// 重复提交（in-flight 守卫拦截）→ 429，避免双击/重放重复扣站点游戏币。
func spendErr(c *gin.Context, err error) {
	spendErrAt(c, err, true)
}

// entReadErr 读侧（档位/奖品/记录/余额/总览）同款分类，只是不谈扣费。
func entReadErr(c *gin.Context, err error) {
	spendErrAt(c, err, false)
}

// spendErrAt 三类处境分开说。从前只有「重复提交 → 429」这一档被区分，
// 其余一律 400「请求参数有误，请检查后重试」：
//   - 链层写的「该游戏不支持 X」是人话原因，WithErr 只进日志 ⇒ 用户不知道该换游戏；
//   - 站点/网络失败是上游问题，而且花钱操作**是否已扣费界面完全没说**，
//     用户要么以为没扣反复点（重复消耗），要么以为扣了而不敢重试。
func spendErrAt(c *gin.Context, err error, spend bool) {
	if errors.Is(err, chain.ErrDuplicateSpend) {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": errcode.DuplicateSpend.Code, "message": err.Error(), "error": err.Error()})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 站点已被删除：链层 siteOp.Get 抛的是记录不存在，与「没有这个游戏」不是一回事。
		errcode.Respond(c, errcode.SiteNotFound.WithErr(err))
		return
	}
	if errors.Is(err, chain.ErrGameNotFound) {
		// 站点没注册这个游戏（含站点已被删）：是引用失效，不是上游故障也不是参数问题。
		errcode.Respond(c, errcode.EntertainmentGameNotFound.WithErr(err))
		return
	}
	if errors.Is(err, chain.ErrGameUnsupported) {
		// WithDetail：链层这句本来就是写给人看的，原样抵达界面。
		errcode.Respond(c, errcode.EntertainmentGameUnsupported.WithDetail(err))
		return
	}
	if errors.Is(err, chain.ErrSiteCredentialUnreadable) {
		// 必须先于 spend/读侧兜底两档：这一档能确定地说「没发请求、没扣费」，
		// 而 EntertainmentSpendFailed 的措辞是「是否已扣费以站点页面为准」，套上去会把人引去核余额。
		errcode.Respond(c, errcode.EntertainmentCredentialUnreadable.WithDetail(err))
		return
	}
	if spend {
		errcode.Respond(c, errcode.EntertainmentSpendFailed.WithErr(err))
		return
	}
	errcode.Respond(c, errcode.EntertainmentDataUnreadable.WithErr(err))
}

// Sites 列出全部站点及其已适配的娱乐项目数（娱乐页站点筛选用，GET /entertainment）。
func (h *EntertainmentHandler) Sites(c *gin.Context) {
	sites, err := h.ent.Sites()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"sites": sites})
}

// Games 列出某站点可用的游戏。
func (h *EntertainmentHandler) Games(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	games, err := h.ent.Games(id)
	if err != nil {
		errcode.Respond(c, errcode.SiteNotFound)
		return
	}
	out := make([]gin.H, 0, len(games))
	for _, g := range games {
		out = append(out, gin.H{"id": g.ID(), "name": g.Name()})
	}
	c.JSON(http.StatusOK, gin.H{"games": out})
}

type playReq struct {
	Game string `json:"game" binding:"required,max=64"`
}

// Play 玩一次游戏。
func (h *EntertainmentHandler) Play(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req playReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	res, err := h.ent.Play(c.Request.Context(), id, req.Game)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// Stats 游戏统计（余额/剩余刮卡等，仅部分游戏支持）。
func (h *EntertainmentHandler) Stats(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.Stats(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		entReadErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// BuyOptions 购卡档位。
func (h *EntertainmentHandler) BuyOptions(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	opts, err := h.ent.BuyOptions(id, c.Param("game"))
	if err != nil {
		entReadErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"options": opts})
}

type buyReq struct {
	OptionType int `json:"option_type" binding:"min=0,max=100"`
}

// Buy 购卡。
func (h *EntertainmentHandler) Buy(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req buyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	res, err := h.ent.Buy(c.Request.Context(), id, c.Param("game"), req.OptionType)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type playMultiReq struct {
	Count int `json:"count" binding:"min=1,max=100"`
}

// PlayMulti 连抽（仅部分游戏支持，如 LongPT 大转盘 size=N）。
func (h *EntertainmentHandler) PlayMulti(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req playMultiReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	res, err := h.ent.PlayMulti(c.Request.Context(), id, c.Param("game"), req.Count)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// Prizes 奖品展示。
func (h *EntertainmentHandler) Prizes(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.Prizes(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		entReadErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"prizes": res})
}

// History 抽奖记录。
func (h *EntertainmentHandler) History(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.History(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		entReadErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": res})
}

// Overview 娱乐总览（全部站点×游戏矩阵，一次拉齐；总览页用）。
func (h *EntertainmentHandler) Overview(c *gin.Context) {
	res, err := h.ent.Overview()
	if err != nil {
		entReadErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
