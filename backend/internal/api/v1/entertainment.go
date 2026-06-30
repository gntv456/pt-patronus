package v1

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/entertainment"
)

// EntertainmentHandler 处理娱乐游戏。
type EntertainmentHandler struct {
	ent *chain.EntertainmentChain
}

// NewEntertainmentHandler 构造。
func NewEntertainmentHandler(ent *chain.EntertainmentChain) *EntertainmentHandler {
	return &EntertainmentHandler{ent: ent}
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
