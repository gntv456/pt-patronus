package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/hxcard"
)

// HxCardHandler 「PT Patronus 守护神」名片推送设置。
type HxCardHandler struct {
	card *hxcard.Pusher
}

// NewHxCardHandler 构造。
func NewHxCardHandler(card *hxcard.Pusher) *HxCardHandler {
	return &HxCardHandler{card: card}
}

type hxcardSettingsReq struct {
	Enabled  bool   `json:"enabled"`
	Endpoint string `json:"endpoint"`
	Secret   string `json:"secret"` // 空=保持不变；"-"=清除
}

// GetSettings 返回推送配置（secret 掩码）。
func (h *HxCardHandler) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": h.card.GetSettings()})
}

// SetSettings 更新推送配置。
func (h *HxCardHandler) SetSettings(c *gin.Context) {
	var req hxcardSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.card.SetSettings(req.Enabled, req.Endpoint, req.Secret); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": h.card.GetSettings()})
}

// Push 立即推送一次当前聚合数据。
func (h *HxCardHandler) Push(c *gin.Context) {
	if err := h.card.Push(c.Request.Context()); err != nil {
		// 推送失败多为可自解释的运营态（未配密钥/限流/好学拒绝），把真实原因带给前端
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已推送到好学"})
}

// Preview 返回将要推送的聚合数据（不发送），供设置页确认。
func (h *HxCardHandler) Preview(c *gin.Context) {
	pl, err := h.card.Collect()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": pl})
}
