package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// HaremHandler 后宫（邀请库存 + 被邀请人花名册）。
type HaremHandler struct {
	ch     *chain.SiteChain
	siteOp *oper.SiteOper
}

// NewHaremHandler 构造。
func NewHaremHandler(ch *chain.SiteChain, siteOp *oper.SiteOper) *HaremHandler {
	return &HaremHandler{ch: ch, siteOp: siteOp}
}

// All 返回全站后宫聚合（?refresh=1 强制刷新）。
func (h *HaremHandler) All(c *gin.Context) {
	force := c.Query("refresh") == "1"
	res := h.ch.HaremAll(c.Request.Context(), force)
	if res == nil {
		c.JSON(http.StatusOK, &chain.HaremResult{Prescriptions: []site.PrescriptionItem{}})
		return
	}
	c.JSON(http.StatusOK, res)
}

// AllStream SSE 流式推送全站后宫（?refresh=1）。每站完成即推一帧 event:batch
// （results 为单元素数组，该站药单）；结束推 event:done（带 overview 全量汇总）。
func (h *HaremHandler) AllStream(c *gin.Context) {
	force := c.Query("refresh") == "1"
	emit := beginSSE(c.Writer)
	overview := h.ch.HaremAllStream(c.Request.Context(), force, func(item site.PrescriptionItem) {
		emit("batch", gin.H{"results": []site.PrescriptionItem{item}})
	})
	emit("done", gin.H{"overview": overview})
}

// SyncOne 立即抓取单站后宫（药单+花名册）。
func (h *HaremHandler) SyncOne(c *gin.Context) {
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
	ctx := c.Request.Context()
	status, e1 := h.ch.HaremStatus(ctx, s)
	invitees, e2 := h.ch.HaremInvitees(ctx, s)
	errMsg := ""
	if e1 != nil {
		errMsg = e1.Error()
	} else if e2 != nil {
		errMsg = e2.Error()
	}
	c.JSON(http.StatusOK, chain.BuildPrescription(s.ID, s.Name, status, invitees, errMsg))
}

// Export 返回药单文本（站点[X]: 剩余[N]个. 可购买[N]个. 权限[✓/✗]，CRLF）。
func (h *HaremHandler) Export(c *gin.Context) {
	force := c.Query("refresh") == "1"
	res := h.ch.HaremAll(c.Request.Context(), force)
	items := []site.PrescriptionItem{}
	if res != nil {
		items = res.Prescriptions
	}
	totalRemain, totalCanBuy, failed, text := chain.ExportPrescription(items)
	c.JSON(http.StatusOK, gin.H{
		"total_remain": totalRemain, "total_can_buy": totalCanBuy,
		"failed_count": failed, "text": text,
	})
}
