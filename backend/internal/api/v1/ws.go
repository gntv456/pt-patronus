package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/ws"
)

// WSHandler 处理实时推送的 WebSocket 升级（?ticket=<一次性> 鉴权）。
// 客户端先用 header-JWT 调 /auth/ws-ticket 换 ticket，再用 ?ticket= 连——JWT 不落 URL。
type WSHandler struct {
	bc             *ws.Broadcaster
	tickets        *auth.TicketStore
	allowedOrigins map[string]bool // 空=不限（仅建议开发）；非空=仅放行白名单 Origin，防 CSWSH
}

// NewWSHandler 构造。allowedOrigins 为允许的浏览器来源（与 CORS 同源策略一致）；
// 为空时放行所有来源；非浏览器客户端（无 Origin 头，如 curl/移动端）始终放行。
func NewWSHandler(bc *ws.Broadcaster, tickets *auth.TicketStore, allowedOrigins []string) *WSHandler {
	set := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" && o != "*" {
			set[o] = true
		}
	}
	return &WSHandler{bc: bc, tickets: tickets, allowedOrigins: set}
}

// checkOrigin 防 CSWSH：配置了白名单时浏览器来源必须命中；非浏览器（无 Origin）放行。
// 注意：WS 升级由 gorilla/websocket 直接处理，不经过 Gin CORS 中间件，CheckOrigin 才是
// 浏览器侧 WebSocket 的真实跨域关卡。
func (h *WSHandler) checkOrigin(r *http.Request) bool {
	if len(h.allowedOrigins) == 0 {
		return true // 开发：未配置白名单
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // 非浏览器客户端无 Origin，不适用 CSWSH
	}
	return h.allowedOrigins[origin]
}

// Handle 升级为 WebSocket：校验 query ticket（单次消费）→ Upgrade → 注册并单推首帧。
func (h *WSHandler) Handle(c *gin.Context) {
	if _, ok := h.tickets.Consume(c.Query("ticket")); !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired ticket"})
		return
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     h.checkOrigin,
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade 已写过错误响应
	}
	// Serve 内部注册连接、单推首帧并启动读写泵；连接断开由泵自清理。
	h.bc.Serve(c.Request.Context(), conn)
}
