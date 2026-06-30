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
	h := NewWSHandler(nil, auth.NewTicketStore(), nil) // bc=nil：拒绝分支不触达
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
	h := NewWSHandler(nil, auth.NewTicketStore(), nil)
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
