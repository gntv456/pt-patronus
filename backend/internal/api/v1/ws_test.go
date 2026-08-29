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
