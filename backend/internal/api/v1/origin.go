// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// originAllowed 是全部 WS 升级端点共用的 Origin 判定（ws / monitor 终端 / docker exec /
// 娱乐小游戏），防 CSWSH。口径（#issue）：
//  1. 非浏览器（无 Origin 头）放行——curl/移动端不适用 CSWSH；
//  2. same-origin 放行：Origin 的 host:port 与请求 Host 一致即同源。IP 直连、反代域名
//     +非标端口的自部署主流形态零配置可连。CSWSH 不受影响——攻击者页面的 Origin host
//     永远对不上受害站点的 Host，此分支不可能命中恶意来源；
//  3. 跨域来源必须命中 allowedOrigins 白名单；
//  4. 白名单为空且 failClosedEmpty（release）：跨域全拒（fail-closed）。
//     failClosedEmpty=false 的调用方（entertainment：空白名单=开发全开语义）保持旧行为。
//
// 注意：WS 升级由 gorilla/websocket 直接处理，不经过 Gin CORS 中间件，这里才是
// 浏览器侧 WebSocket 的真实跨域关卡。
func originAllowed(allowedOrigins map[string]bool, failClosedEmpty bool, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if sameOrigin(origin, r.Host) {
		return true
	}
	if len(allowedOrigins) == 0 {
		// ticket 虽是一次性，但 CSWSH 下受害者浏览器可先替攻击者换 ticket，
		// 故 fail-closed（命令执行级能力不能默认敞开）。
		return !failClosedEmpty
	}
	return allowedOrigins[origin]
}

// sameOrigin 判断浏览器 Origin（scheme://host[:port]）与请求 Host 是否同一站点。
// 端口归一：Origin 省略端口 = 该 scheme 默认端口（http=80/https=443），Host 同理
// （反代场景 Host 由 Lucky/nginx 原样转发，非默认端口时带端口）。
func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	u.Host = net.JoinHostPort(u.Hostname(), normalizePort(u.Port(), u.Scheme))
	return strings.EqualFold(u.Host, net.JoinHostPort(parseHostname(host), normalizePort(parsePort(host), u.Scheme)))
}

// normalizePort 空端口（scheme 默认）映射为显式端口号。
func normalizePort(port, scheme string) string {
	if port != "" {
		return port
	}
	if scheme == "https" || scheme == "wss" {
		return "443"
	}
	return "80"
}

// parseHostname / parsePort 从「host:port」里取主机与端口（IPv6 字面量 [::1]:8080 兼容）。
func parseHostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return strings.Trim(host, "[]") // 无端口：整体即主机（可能是裸 IPv6 字面量）
}

func parsePort(host string) string {
	if _, p, err := net.SplitHostPort(host); err == nil {
		return p
	}
	return ""
}
