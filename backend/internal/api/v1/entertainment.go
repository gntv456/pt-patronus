package v1

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/entertainment"
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
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error(), "code": "duplicate_spend"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

// Games 列出某站点可用的游戏。
func (h *EntertainmentHandler) Games(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	games, err := h.ent.Games(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	out := make([]gin.H, 0, len(games))
	for _, g := range games {
		out = append(out, gin.H{"id": g.ID(), "name": g.Name()})
	}
	c.JSON(http.StatusOK, gin.H{"games": out})
}

type playReq struct {
	Game string `json:"game" binding:"required"`
}

// Play 玩一次游戏。
func (h *EntertainmentHandler) Play(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req playReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"options": opts})
}

type buyReq struct {
	OptionType int `json:"option_type"`
}

// Buy 购卡。
func (h *EntertainmentHandler) Buy(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req buyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	Count int `json:"count"`
}

// PlayMulti 连抽（仅部分游戏支持，如 LongPT 大转盘 size=N）。
func (h *EntertainmentHandler) PlayMulti(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req playMultiReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": res})
}

// Syntheses 卡牌合成目标列表（含碎片库存）。
func (h *EntertainmentHandler) Syntheses(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.Syntheses(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"targets": res})
}

type drawReq struct {
	SynthesisID int `json:"synthesis_id"`
	Count       int `json:"count"`
}

// Draw 抽碎片（1/10/100/其它）。
func (h *EntertainmentHandler) Draw(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req drawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.ent.Draw(c.Request.Context(), id, c.Param("game"), req.SynthesisID, req.Count)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type synthReq struct {
	SynthesisID int `json:"synthesis_id"`
}

// Synthesize 合成。
func (h *EntertainmentHandler) Synthesize(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req synthReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.ent.Synthesize(c.Request.Context(), id, c.Param("game"), req.SynthesisID)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// MedalTree 勋章金字塔（六十四卦）。
func (h *EntertainmentHandler) MedalTree(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.MedalTree(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": res})
}

// MedalShards 阳爻/阴爻碎片余额。
func (h *EntertainmentHandler) MedalShards(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.MedalShards(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"shards": res})
}

type composeReq struct {
	MedalID string `json:"medal_id" binding:"required"`
}

// ComposeMedal 碎片合成（太极/两仪/四象）。
func (h *EntertainmentHandler) ComposeMedal(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req composeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.ent.ComposeMedal(c.Request.Context(), id, c.Param("game"), req.MedalID)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// ComposeBigMedal 大勋章合成（八卦/六十四卦，耗魔力+子勋章）。
func (h *EntertainmentHandler) ComposeBigMedal(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req composeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.ent.ComposeBigMedal(c.Request.Context(), id, c.Param("game"), req.MedalID)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type convertReq struct {
	YiYang int `json:"yi_yang"`
}

// ConvertShard 阴阳转换（yi_yang>0 阴→阳，<0 阳→阴）。
func (h *EntertainmentHandler) ConvertShard(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req convertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.ent.ConvertShard(c.Request.Context(), id, c.Param("game"), req.YiYang)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// Farm 农场全景（NovaHD 魔力农场）。
func (h *EntertainmentHandler) Farm(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.Farm(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

type farmItemReq struct {
	Section string `json:"section"` // crop | animal
	ItemID  int    `json:"item_id"`
}

// Plant 种植/养殖。
func (h *EntertainmentHandler) Plant(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req farmItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.ent.Plant(c.Request.Context(), id, c.Param("game"), req.Section, req.ItemID)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// Harvest 收获单个。
func (h *EntertainmentHandler) Harvest(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req farmItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.ent.Harvest(c.Request.Context(), id, c.Param("game"), req.Section, req.ItemID)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// HarvestAll 一键收获。
func (h *EntertainmentHandler) HarvestAll(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.ent.HarvestAll(c.Request.Context(), id, c.Param("game"))
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type sellReq struct {
	Key string `json:"key"`
}

// Sell 出售仓库物品。
func (h *EntertainmentHandler) Sell(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req sellReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.ent.Sell(c.Request.Context(), id, c.Param("game"), req.Key)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

type betReq struct {
	Selection string            `json:"selection"`
	Amount    float64           `json:"amount"`
	Extra     map[string]string `json:"extra"`
}

// Bet 下注/选号（猜大小/骰宝/双色球/翻面/Betting/红包）。
func (h *EntertainmentHandler) Bet(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req betReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event id"})
		return
	}
	res, err := h.ent.EventDetail(c.Request.Context(), id, c.Param("game"), eventID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

type blackjackReq struct {
	Action string `json:"action"`
}

// Blackjack 二十一点动作（new/hit/stand/double）。
func (h *EntertainmentHandler) Blackjack(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req blackjackReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// FlipWS 翻面夺宝实时事件流（站点 wss 桥接）。
// ?ticket=<一次性> 鉴权 → 升级 → 后端用站点 cookie 连 wss://host/wss，把 tileFlip 事件转发客户端。
// 客户端断开 → 关上游；每客户端连接 = 一条上游 WS（正常用量，非攻击）。
func (h *EntertainmentHandler) FlipWS(c *gin.Context) {
	if _, ok := h.tickets.Consume(c.Query("ticket")); !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired ticket"})
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
		_ = conn.WriteJSON(gin.H{"type": "error", "message": err.Error()})
		return
	}
	// 读泵：仅用于探测客户端断开（读到错即取消 ctx，关上游）
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()
	for ev := range events {
		if err := conn.WriteJSON(ev); err != nil {
			break
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"chat": res})
}

type bsSendChatReq struct {
	Message string `json:"message"`
}

// BigSmallSendChat 发聊天。
func (h *EntertainmentHandler) BigSmallSendChat(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req bsSendChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	Action string `json:"action"` // claim | reward | refund
	TaskID int    `json:"task_id"`
}

// BigSmallTaskAction 任务领取/领奖/亏返卡。
func (h *EntertainmentHandler) BigSmallTaskAction(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req bsTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	Tag string `json:"tag"`
}

// BigSmallUseItem 使用道具（熊猫 useItem；好学亏返卡 refund）。
func (h *EntertainmentHandler) BigSmallUseItem(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req bsItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	Action string            `json:"action"` // send | claim | info
	Extra  map[string]string `json:"extra"`
}

// BigSmallRedpacket 红包发送/领取/详情。
func (h *EntertainmentHandler) BigSmallRedpacket(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req bsRedpacketReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.ent.BigSmallRedpacket(c.Request.Context(), id, c.Param("game"), req.Action, req.Extra)
	if err != nil {
		spendErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
