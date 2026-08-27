package v1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
)

// MediaProxyHandler 媒体代理（Emby/JF 反代 + 302 直链 + 失败回退）。
// 公开路由（播放器不能带 JWT）：/api/v1/mediaproxy/:id/*rest——Emby 客户端把服务器
// 地址填成该前缀即可；普通请求透明转发，原始流播放尝试云盘直链 302。
type MediaProxyHandler struct {
	svc *chain.MediaProxyService
}

func NewMediaProxyHandler(svc *chain.MediaProxyService) *MediaProxyHandler {
	return &MediaProxyHandler{svc: svc}
}

// Handle gin 入口。rest 含完整 Emby 路径与 query。
func (h *MediaProxyHandler) Handle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server id"})
		return
	}
	rest := c.Param("rest")
	if q := c.Request.URL.RawQuery; q != "" {
		rest += "?" + q
	}
	if strings.Contains(rest, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad path"})
		return
	}
	h.svc.Handle(c.Writer, c.Request, uint(id), rest)
}
