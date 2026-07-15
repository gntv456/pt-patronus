package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// AssessmentHandler 站点考核。
type AssessmentHandler struct {
	ch     *chain.AssessmentChain
	siteOp *oper.SiteOper
}

// NewAssessmentHandler 构造。
func NewAssessmentHandler(ch *chain.AssessmentChain, siteOp *oper.SiteOper) *AssessmentHandler {
	return &AssessmentHandler{ch: ch, siteOp: siteOp}
}

// All 返回全站考核聚合（按 progress 降序；?refresh=1 强制刷新）。
func (h *AssessmentHandler) All(c *gin.Context) {
	force := c.Query("refresh") == "1"
	cards := h.ch.AssessAll(c.Request.Context(), force)
	if cards == nil {
		cards = []chain.AssessmentCard{}
	}
	c.JSON(http.StatusOK, gin.H{"sites": cards})
}

// AllStream SSE 流式推送全站考核（?refresh=1）。每站完成即推一帧 event:batch
// （results 为单元素数组，该站卡片）；结束推 event:done。供考核页「加载一个显示一个」。
func (h *AssessmentHandler) AllStream(c *gin.Context) {
	force := c.Query("refresh") == "1"
	emit := beginSSE(c.Writer)
	h.ch.AssessAllStream(c.Request.Context(), force, func(card chain.AssessmentCard) {
		emit("batch", gin.H{"results": []chain.AssessmentCard{card}})
	})
	emit("done", gin.H{})
}

// SyncOne 立即抓取单站考核。
func (h *AssessmentHandler) SyncOne(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	s, err := h.siteOp.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}
	c.JSON(http.StatusOK, h.ch.AssessOne(c.Request.Context(), s))
}
