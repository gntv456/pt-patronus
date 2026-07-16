package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// MedalHandler 勋章墙（跨站勋章聚合 + 购买/佩戴/摘下）。
type MedalHandler struct {
	ch     *chain.MedalChain
	siteOp *oper.SiteOper
}

// NewMedalHandler 构造。
func NewMedalHandler(ch *chain.MedalChain, siteOp *oper.SiteOper) *MedalHandler {
	return &MedalHandler{ch: ch, siteOp: siteOp}
}

// All 返回全站勋章聚合（?refresh=1 强制刷新）。nil 切片归一为空数组，防前端 JSON null。
func (h *MedalHandler) All(c *gin.Context) {
	force := c.Query("refresh") == "1"
	ctx, cancel := aggregateRequestContext(c)
	defer cancel()
	cards := h.ch.MedalAll(ctx, force)
	if cards == nil {
		cards = []chain.MedalCard{}
	}
	c.JSON(http.StatusOK, gin.H{"medals": cards})
}

// AllStream SSE 流式推送全站勋章（?refresh=1）。每站完成即推一帧 event:batch
// （results 为该站多张卡，失败站为单张错误卡）；结束推 event:done。供勋章墙「加载一个显示一个」。
func (h *MedalHandler) AllStream(c *gin.Context) {
	force := c.Query("refresh") == "1"
	emit := beginSSE(c.Writer)
	h.ch.MedalAllStream(c.Request.Context(), force, func(cards []chain.MedalCard) {
		emit("batch", gin.H{"results": cards})
	})
	emit("done", gin.H{})
}

// medalActionBody 写操作请求体。
type medalActionBody struct {
	SiteID  uint   `json:"site_id" binding:"min=1"`
	MedalID string `json:"medal_id" binding:"max=64"`
}

// Buy 购买勋章。
func (h *MedalHandler) Buy(c *gin.Context) { h.doAction(c, "buy") }

// Wear 佩戴勋章。
func (h *MedalHandler) Wear(c *gin.Context) { h.doAction(c, "wear") }

// TakeOff 摘下勋章。
func (h *MedalHandler) TakeOff(c *gin.Context) { h.doAction(c, "takeoff") }

func (h *MedalHandler) doAction(c *gin.Context, action string) {
	var body medalActionBody
	if err := c.ShouldBindJSON(&body); err != nil || body.MedalID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	s, err := h.siteOp.Get(body.SiteID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	ctx := c.Request.Context()
	var success bool
	var message string
	switch action {
	case "buy":
		r, _ := h.ch.Buy(ctx, s, body.MedalID)
		success, message = r.Success, r.Message
	case "wear":
		r, _ := h.ch.Wear(ctx, s, body.MedalID)
		success, message = r.Success, r.Message
	case "takeoff":
		r, _ := h.ch.TakeOff(ctx, s, body.MedalID)
		success, message = r.Success, r.Message
	}
	c.JSON(http.StatusOK, gin.H{"success": success, "message": message})
}
