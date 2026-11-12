// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/ws"
)

// WSHandler 处理实时推送的 WebSocket 升级（?ticket=<一次性> 鉴权）。
// 客户端先用 header-JWT 调 /auth/ws-ticket 换 ticket，再用 ?ticket= 连——JWT 不落 URL。
type WSHandler struct {
	bc             *ws.Broadcaster
	tickets        *auth.TicketStore
	allowedOrigins map[string]bool // 空=跨域全拒（release）；非空=跨域仅放行白名单；same-origin 恒放行
	release        bool            // release 模式：白名单为空时拒绝带 Origin 的浏览器连接（fail-closed）
}

// NewWSHandler 构造。allowedOrigins 为允许的浏览器来源（与 CORS 同源策略一致）；
// same-origin（Origin 与请求 Host 同站点）始终放行；跨域来源为空时：debug 放行所有来源、
// release 拒绝（CSWSH fail-closed）；非浏览器客户端（无 Origin 头，如 curl/移动端）始终放行。
func NewWSHandler(bc *ws.Broadcaster, tickets *auth.TicketStore, allowedOrigins []string, release bool) *WSHandler {
	set := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" && o != "*" {
			set[o] = true
		}
	}
	return &WSHandler{bc: bc, tickets: tickets, allowedOrigins: set, release: release}
}

// checkOrigin 防 CSWSH：same-origin 自动放行；跨域须命中白名单（release 空白名单 fail-closed）。
// 统一口径与实现见 origin.go 的 originAllowed。
func (h *WSHandler) checkOrigin(r *http.Request) bool {
	return originAllowed(h.allowedOrigins, h.release, r)
}

// Handle 升级为 WebSocket：校验 query ticket（单次消费）→ Upgrade → 注册并单推首帧。
func (h *WSHandler) Handle(c *gin.Context) {
	if _, ok := h.tickets.Consume(c.Query("ticket")); !ok {
		errcode.Respond(c, errcode.TicketInvalidOrExpired)
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
	// proto=inc：客户端声明支持增量协议（收 torrents_full/torrents_diff 而非旧 torrents 全量）。
	h.bc.Serve(c.Request.Context(), conn, c.Query("proto") == "inc")
}
