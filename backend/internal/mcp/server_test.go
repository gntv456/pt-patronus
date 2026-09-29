package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rpc 发一条 JSON-RPC 请求并解析响应。
func rpc(t *testing.T, srv *Server, body string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret") // 鉴权 fail-closed：功能用例统一带 token
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
	return out
}

func TestServerDisabledReturns404(t *testing.T) {
	srv := NewServer()
	srv.SetSettingsGetter(func() Settings { return Settings{Enabled: false} })
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

// TestServerEmptyTokenFailClosed P2 回归：Enabled 且 Token 为空必须 401（fail-closed）——
// 此前空 token 完全放行，管理员忘填 token 即匿名可用全部 MCP 工具。
func TestServerEmptyTokenFailClosed(t *testing.T) {
	srv := NewServer()
	srv.SetSettingsGetter(func() Settings { return Settings{Enabled: true, Token: ""} })
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("空 token 应 401 拒绝（fail-closed），got %d", rec.Code)
	}
}

func TestServerTokenGuard(t *testing.T) {
	srv := NewServer()
	srv.SetSettingsGetter(func() Settings { return Settings{Enabled: true, Token: "secret"} })
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestServerInitializeAndToolsList(t *testing.T) {
	srv := NewServer()
	srv.SetSettingsGetter(func() Settings { return Settings{Enabled: true, Token: "secret"} })
	srv.RegisterTool(Tool{
		Name: "echo", Description: "echo back",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			return CompactJSON(args), nil
		},
	})
	// initialize
	r := rpc(t, srv, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if r["result"] == nil {
		t.Fatalf("initialize missing result: %v", r)
	}
	// tools/list
	r = rpc(t, srv, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	res := r["result"].(map[string]any)
	tools := res["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(tools))
	}
}

func TestServerToolCallSuccess(t *testing.T) {
	srv := NewServer()
	srv.SetSettingsGetter(func() Settings { return Settings{Enabled: true, Token: "secret"} })
	srv.RegisterTool(Tool{
		Name: "add", Handler: func(ctx context.Context, args map[string]any) (string, error) {
			return "42", nil
		},
	})
	r := rpc(t, srv, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"add","arguments":{"x":1}}}`)
	res := r["result"].(map[string]any)
	content := res["content"].([]any)
	if content[0].(map[string]any)["text"] != "42" {
		t.Fatalf("unexpected content: %v", content)
	}
}

func TestServerToolCallErrorIsNotRPCError(t *testing.T) {
	srv := NewServer()
	srv.SetSettingsGetter(func() Settings { return Settings{Enabled: true, Token: "secret"} })
	srv.RegisterTool(Tool{
		Name: "boom", Handler: func(ctx context.Context, args map[string]any) (string, error) {
			return "", errBoom
		},
	})
	r := rpc(t, srv, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"boom"}}`)
	// 工具错误以 isError=true 的成功信封返回，不得破坏 JSON-RPC。
	if r["error"] != nil {
		t.Fatalf("tool error must not set rpc error: %v", r)
	}
	res := r["result"].(map[string]any)
	if res["isError"] != true {
		t.Fatalf("want isError=true, got %v", res)
	}
}

func TestServerUnknownMethod(t *testing.T) {
	srv := NewServer()
	srv.SetSettingsGetter(func() Settings { return Settings{Enabled: true, Token: "secret"} })
	r := rpc(t, srv, `{"jsonrpc":"2.0","id":5,"method":"nope"}`)
	if r["error"] == nil {
		t.Fatalf("want rpc error for unknown method: %v", r)
	}
}

func TestServerNotificationNoResponse(t *testing.T) {
	srv := NewServer()
	srv.SetSettingsGetter(func() Settings { return Settings{Enabled: true, Token: "secret"} })
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("notification want 202, got %d", rec.Code)
	}
}

var errBoom = errSentinel{}

type errSentinel struct{}

func (errSentinel) Error() string { return "boom" }
