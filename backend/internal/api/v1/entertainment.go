package v1

import (
	"errors"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/chain"
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

// checkOrigin 防 CSWSH：配置白名单时浏览器来源须命中；非浏览器（无 Origin）放行。
func (h *EntertainmentHandler) checkOrigin(r *http.Request) bool {
	if len(h.allowedOrigins) == 0 {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return h.allowedOrigins[origin]
}

// spendErr 处理"花钱"操作（Play/Buy/Draw/Synthesize）的错误：
// 重复提交（in-flight 守卫拦截）→ 429，避免双击/重放重复扣站点游戏币；其余 → 400。
func spendErr(c *gin.Context, err error) {
	if errors.Is(err, chain.ErrDuplicateSpend) {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": errcode.DuplicateSpend.Code, "message": err.Error(), "error": err.Error()})
		return
	}
	errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": res})
}
