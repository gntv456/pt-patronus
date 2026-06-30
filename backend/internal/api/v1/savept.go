package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/savept"
)

// SaveptHandler 「PT监护室」站点健康聚合。
type SaveptHandler struct {
	svc *savept.Service
}

// NewSaveptHandler 构造。
func NewSaveptHandler(svc *savept.Service) *SaveptHandler {
	return &SaveptHandler{svc: svc}
}

// Get 返回监护室数据（卡片+公告+汇总+本地匹配）；?refresh=1 强制重抓 savept.icu。
func (h *SaveptHandler) Get(c *gin.Context) {
	force := c.Query("refresh") == "1"
	res, err := h.svc.Fetch(c.Request.Context(), force)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "抓取 savept.icu 失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}
