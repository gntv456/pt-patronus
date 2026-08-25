package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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
	SynthesisID int `json:"synthesis_id" binding:"min=1,max=100000"`
	Count       int `json:"count" binding:"min=1,max=100"`
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
	SynthesisID int `json:"synthesis_id" binding:"min=1,max=100000"`
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
	MedalID string `json:"medal_id" binding:"required,max=64"`
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
	YiYang int `json:"yi_yang" binding:"min=-100000,max=100000"`
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
	Section string `json:"section" binding:"required,oneof=crop animal"`
	ItemID  int    `json:"item_id" binding:"min=1,max=100000"`
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
	Key string `json:"key" binding:"required,max=64"`
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
