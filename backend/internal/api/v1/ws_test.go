// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/auth"
)

// 鉴权是 WS 的安全关键：未带 / 无效 ticket 必须在 Upgrade 前拒绝。
// 成功路径会触发 broadcaster.Serve（需完整 manager/DB 依赖），留作运行时验证。

func TestWSHandlerRejectsMissingTicket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewWSHandler(nil, auth.NewTicketStore(), nil, false) // bc=nil：拒绝分支不触达
	r := gin.New()
	r.GET("/ws", h.Handle)
	srv := httptest.NewServer(r)
	defer srv.Close()

	_, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err == nil {
		t.Fatal("expected dial to fail without ticket")
	}
	if resp == nil || resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %v", resp)
	}
	if resp != nil {
		resp.Body.Close()
	}
}

func TestWSHandlerRejectsBadTicket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewWSHandler(nil, auth.NewTicketStore(), nil, false)
	r := gin.New()
	r.GET("/ws", h.Handle)
	srv := httptest.NewServer(r)
	defer srv.Close()

	_, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws?ticket=garbage", nil)
	if err == nil {
		t.Fatal("expected dial to fail with bad ticket")
	}
	if resp == nil || resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %v", resp)
	}
	if resp != nil {
		resp.Body.Close()
	}
}

// TestWS_OriginFailClosedRelease release 模式下未配置白名单：带 Origin 的浏览器
// 连接必须被拒（CSWSH fail-closed）；无 Origin 的非浏览器客户端照常放行。
func TestWS_OriginFailClosedRelease(t *testing.T) {
	h := NewWSHandler(nil, auth.NewTicketStore(), nil, true)
	req := httptest.NewRequest("GET", "/ws?ticket=x", nil)
	req.Header.Set("Origin", "https://evil.example")
	if h.checkOrigin(req) {
		t.Fatal("release + empty whitelist: browser Origin must be rejected")
	}
	req2 := httptest.NewRequest("GET", "/ws?ticket=x", nil)
	if !h.checkOrigin(req2) {
		t.Fatal("non-browser (no Origin) must always pass")
	}
}

// TestWS_SameOriginAlwaysAllowed same-origin 自动放行：IP 直连、反代域名+非标端口
// 等自部署主流形态零配置可连（#issue）。即使 release+空白名单（fail-closed）也放行——
// CSWSH 场景下攻击者页面的 Origin host 永远对不上受害站点的 Host。
func TestWS_SameOriginAlwaysAllowed(t *testing.T) {
	h := NewWSHandler(nil, auth.NewTicketStore(), nil, true) // release + 空白名单：最严组合
	cases := []struct{ name, origin, host string }{
		{"反代域名+非标端口", "https://ptp.example.xyz:12345", "ptp.example.xyz:12345"},
		{"IP 直连+端口", "http://192.168.1.10:8088", "192.168.1.10:8088"},
		{"https 默认端口归一（Origin 省端口）", "https://pt.example.com", "pt.example.com:443"},
		{"http 默认端口归一（Host 带显式 80）", "http://pt.example.com:80", "pt.example.com"},
		{"host 大小写不敏感", "https://PT.Example.COM:12345", "pt.example.com:12345"},
		{"IPv6 直连", "http://[::1]:8088", "[::1]:8088"},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/ws?ticket=x", nil)
		req.Header.Set("Origin", c.origin)
		req.Host = c.host
		if !h.checkOrigin(req) {
			t.Errorf("%s: same-origin 被拒 origin=%s host=%s", c.name, c.origin, c.host)
		}
	}
}

// TestWS_CrossOriginStillRejected 跨域来源（host 对不上）在 release+空白名单下仍全拒，
// 且配置白名单后仅放行命中项——same-origin 分支不能成为跨域旁路。
func TestWS_CrossOriginStillRejected(t *testing.T) {
	empty := NewWSHandler(nil, auth.NewTicketStore(), nil, true)
	req := httptest.NewRequest("GET", "/ws?ticket=x", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Host = "pt.example.com:12345"
	if empty.checkOrigin(req) {
		t.Fatal("cross-origin must stay rejected with empty whitelist in release")
	}

	wl := NewWSHandler(nil, auth.NewTicketStore(), []string{"https://allowed.example"}, true)
	ok := httptest.NewRequest("GET", "/ws?ticket=x", nil)
	ok.Header.Set("Origin", "https://allowed.example")
	ok.Host = "pt.example.com:12345"
	if !wl.checkOrigin(ok) {
		t.Fatal("whitelisted cross-origin must pass")
	}
	bad := httptest.NewRequest("GET", "/ws?ticket=x", nil)
	bad.Header.Set("Origin", "https://other.example")
	bad.Host = "pt.example.com:12345"
	if wl.checkOrigin(bad) {
		t.Fatal("non-whitelisted cross-origin must be rejected")
	}
	// debug 模式空白名单：跨域放行（旧行为保持）
	dbg := NewWSHandler(nil, auth.NewTicketStore(), nil, false)
	d := httptest.NewRequest("GET", "/ws?ticket=x", nil)
	d.Header.Set("Origin", "https://evil.example")
	d.Host = "pt.example.com:12345"
	if !dbg.checkOrigin(d) {
		t.Fatal("debug + empty whitelist: cross-origin should stay permissive (legacy behavior)")
	}
}
