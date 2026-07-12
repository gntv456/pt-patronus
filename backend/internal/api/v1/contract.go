package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// ContractHandler 保种契约规则 CRUD + 现场检查。
type ContractHandler struct {
	svc *chain.ContractChain
}

func NewContractHandler(svc *chain.ContractChain) *ContractHandler {
	return &ContractHandler{svc: svc}
}

func (h *ContractHandler) List(c *gin.Context) {
	rs, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if rs == nil {
		rs = []models.ContractRule{}
	}
	c.JSON(http.StatusOK, gin.H{"rules": rs})
}

// Upsert 新增或更新契约（按 site_id 合并，每站一条）。
func (h *ContractHandler) Upsert(c *gin.Context) {
	var r models.ContractRule
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if r.SiteID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "站点不能为空"})
		return
	}
	if err := h.svc.Upsert(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, r)
}

func (h *ContractHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	opResult(c, h.svc.Delete(uint(id)))
}

// Check 现场检查单条契约，返回达标/违例详情。
func (h *ContractHandler) Check(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	res, err := h.svc.Check(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}
