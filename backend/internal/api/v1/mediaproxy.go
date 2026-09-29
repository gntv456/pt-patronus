package v1

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
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
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	rest := c.Param("rest")
	if q := c.Request.URL.RawQuery; q != "" {
		rest += "?" + q
	}
	if !safeProxyRest(rest) {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	h.svc.Handle(c.Writer, c.Request, uint(id), rest)
}

// safeProxyRest 上游路径段校验：拒绝 `..`（含双重编码 %2e%2e——gin 解码一次后
// 为字面 `%2e%2e`，黑名单 `..` 拦不住，上游再解码即穿越）、反斜杠与控制字符。
// 白名单式：仅放行常规 URL 路径/查询字符（P2 修复：原黑名单可被 %252e%252e 绕过）。
func safeProxyRest(rest string) bool {
	if rest == "" {
		return true
	}
	if strings.Contains(rest, "..") || strings.Contains(rest, "%2e%2e") ||
		strings.Contains(rest, "%2E%2E") || strings.Contains(rest, `\`) {
		return false
	}
	for _, r := range rest {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
