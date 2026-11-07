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
	token := loginAsAdmin(t, srv)

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

	t.Run("authed database write always forbidden", func(t *testing.T) {
		code, body := do(t, srv, "POST", "/api/v1/ops/database/query",
			map[string]any{"sql": "CREATE TABLE t (id int)", "confirm_risk": true}, token)
		if code != 403 {
			t.Errorf("write with confirm: code=%d, want 403", code)
		}
		errCode, _ := body["code"].(string)
		if errCode != "write_sql_forbidden" {
			t.Errorf("error code=%q, want write_sql_forbidden", errCode)
		}
	})

	t.Run("authed database forbidden sql", func(t *testing.T) {
		code, _ := do(t, srv, "POST", "/api/v1/ops/database/query",
			map[string]any{"sql": "SELECT * FROM (DELETE FROM x) y", "confirm_risk": true}, token)
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
		// 直接查既有业务表的 schema（写 SQL 已被禁用，无法经 query 建表）
		code, body := do(t, srv, "GET", "/api/v1/ops/database/tables/users/schema", nil, token)
		if code != 200 {
			t.Fatalf("schema: code=%d body=%v", code, body)
		}
		cols, _ := body["columns"].([]any)
		if len(cols) == 0 {
			t.Error("schema columns: got 0, want >0")
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
