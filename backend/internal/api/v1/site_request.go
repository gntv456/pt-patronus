package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
)

// RequestHandler 站点求种请求（对标 MSaber「站点请求」）。
type RequestHandler struct {
	ch *chain.SiteChain
}

// NewSiteRequestHandler 构造。
func NewSiteRequestHandler(ch *chain.SiteChain) *RequestHandler { return &RequestHandler{ch: ch} }

// All GET /site-requests?filled=1&site_id= —— 跨站聚合求种列表（site_id 指定单站）。
func (h *RequestHandler) All(c *gin.Context) {
	filled := c.Query("filled") == "1"
	if raw := c.Query("site_id"); raw != "" {
		sid, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || sid == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site_id"})
			return
		}
		s, err := h.ch.SiteOper().Get(uint(sid))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
			return
		}
		r, err := h.ch.Requests(c.Request.Context(), s, filled)
		if err != nil && r == nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"sites": []*chain.RequestResult{r}})
		return
	}
	results := h.ch.RequestsAll(c.Request.Context(), filled)
	c.JSON(http.StatusOK, gin.H{"sites": results})
}
