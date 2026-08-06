package api

import (
	"net/http/httptest"
	"testing"
)

// TestOpsCenterRoutesAuth 验证运维中心四大功能的路由注册与鉴权链：
// 未认证 → 401；认证后 → 路由可达（非 404/405）。
func TestOpsCenterRoutesAuth(t *testing.T) {
	deps := newTestDeps(t)
	srv := httptest.NewServer(Build(deps))
	defer srv.Close()

	// 登录获取 admin token
	_, body := do(t, srv, "POST", "/api/v1/auth/hxpt/cookie-login",
		map[string]string{"cookie": "c_secure_uid=777"}, "")
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("no token from login")
	}

	t.Run("unauth blocked", func(t *testing.T) {
		cases := []struct{ method, path string }{
			{"GET", "/api/v1/ops/database/tables"},
			{"POST", "/api/v1/ops/database/query"},
			{"GET", "/api/v1/ops/docker/1/list"},
			{"GET", "/api/v1/system/metrics"},
		}
		for _, tc := range cases {
			code, _ := do(t, srv, tc.method, tc.path, nil, "")
			if code != 401 {
				t.Errorf("%s %s: unauth code=%d, want 401", tc.method, tc.path, code)
			}
		}
	})

	t.Run("authed database tables", func(t *testing.T) {
		code, body := do(t, srv, "GET", "/api/v1/ops/database/tables", nil, token)
		if code != 200 {
			t.Fatalf("tables: code=%d body=%v", code, body)
		}
		tables, _ := body["tables"].([]any)
		if tables == nil {
			t.Error("expected tables array")
		}
	})

	t.Run("authed database read query", func(t *testing.T) {
		code, body := do(t, srv, "POST", "/api/v1/ops/database/query",
			map[string]any{"sql": "SELECT 1 as v", "limit": 5}, token)
		if code != 200 {
			t.Fatalf("query: code=%d body=%v", code, body)
		}
		isWrite, _ := body["is_write"].(bool)
		if isWrite {
			t.Error("SELECT should not be write")
		}
	})

	t.Run("authed database write needs confirm", func(t *testing.T) {
		code, body := do(t, srv, "POST", "/api/v1/ops/database/query",
			map[string]any{"sql": "CREATE TABLE t (id int)"}, token)
		if code != 400 {
			t.Errorf("write without confirm: code=%d, want 400", code)
		}
		errCode, _ := body["code"].(string)
		if errCode != "risk_confirmation_required" {
			t.Errorf("error code=%q, want risk_confirmation_required", errCode)
		}
	})

	t.Run("authed database forbidden sql", func(t *testing.T) {
		code, _ := do(t, srv, "POST", "/api/v1/ops/database/query",
			map[string]any{"sql": "DROP DATABASE x", "confirm_risk": true}, token)
		if code != 400 {
			t.Errorf("forbidden sql: code=%d, want 400", code)
		}
	})

	t.Run("authed database multi-statement blocked", func(t *testing.T) {
		code, _ := do(t, srv, "POST", "/api/v1/ops/database/query",
			map[string]any{"sql": "SELECT 1; DROP TABLE t", "confirm_risk": true}, token)
		if code != 400 {
			t.Errorf("multi-statement: code=%d, want 400", code)
		}
	})

	t.Run("authed database schema valid table", func(t *testing.T) {
		// 先创建一个表，再查 schema
		do(t, srv, "POST", "/api/v1/ops/database/query",
			map[string]any{"sql": "CREATE TABLE test_ops (id INTEGER PRIMARY KEY, name TEXT NOT NULL)", "confirm_risk": true}, token)
		code, body := do(t, srv, "GET", "/api/v1/ops/database/tables/test_ops/schema", nil, token)
		if code != 200 {
			t.Fatalf("schema: code=%d body=%v", code, body)
		}
		cols, _ := body["columns"].([]any)
		if len(cols) != 2 {
			t.Errorf("schema columns: got %d, want 2", len(cols))
		}
	})

	t.Run("authed database schema invalid ident", func(t *testing.T) {
		code, _ := do(t, srv, "GET", "/api/v1/ops/database/tables/bad%20table/schema", nil, token)
		if code != 400 {
			t.Errorf("invalid ident: code=%d, want 400", code)
		}
	})

	t.Run("authed system metrics", func(t *testing.T) {
		code, body := do(t, srv, "GET", "/api/v1/system/metrics", nil, token)
		if code != 200 {
			t.Fatalf("metrics: code=%d body=%v", code, body)
		}
		online, _ := body["online"].(bool)
		if !online {
			t.Error("expected online=true")
		}
	})

	t.Run("authed docker route exists", func(t *testing.T) {
		// Docker 端点需要 MonSvc（SSH 主机），集成测试无 SSH 主机，
		// 但路由必须存在——非 404 即证明路由注册正确。
		code, _ := do(t, srv, "GET", "/api/v1/ops/docker/99999/list", nil, token)
		if code == 404 {
			t.Error("docker list route not found (404) — route registration broken")
		}
	})

	t.Run("authed docker compose up missing path", func(t *testing.T) {
		code, _ := do(t, srv, "POST", "/api/v1/ops/docker/99999/compose/up",
			map[string]any{}, token)
		if code != 400 {
			t.Errorf("compose up missing path: code=%d, want 400", code)
		}
	})
}
