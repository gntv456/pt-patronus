package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// host_tools.go 覆盖：registry CRUD/可见性过滤、Dispatch 四象限（notFound/403/err/success）、
// per-tool 超时、adopt 的 json.RawMessage 不双重转义，以及 externalHostServer 的 /tools 端点。

func TestHostToolRegistry_RegisterAndLookup(t *testing.T) {
	t.Parallel()
	reg := NewHostToolRegistry()
	reg.Register(HostTool{Name: "a", Permission: "x:read"}, func(ctx context.Context, _ map[string]any) (any, error) {
		return "A", nil
	})
	// 空 Name / nil handler 被忽略
	reg.Register(HostTool{Name: ""}, nil)
	reg.Register(HostTool{Name: "b"}, nil)

	if _, ok := reg.Lookup("a"); !ok {
		t.Error("Lookup(a) missing")
	}
	if _, ok := reg.Lookup("missing"); ok {
		t.Error("Lookup(missing) should be false")
	}
	// nil registry 不 panic
	var nilReg *HostToolRegistry
	if _, ok := nilReg.Lookup("a"); ok {
		t.Error("nil registry Lookup should be false")
	}
	if got := nilReg.VisibleTools(nil); len(got) != 0 {
		t.Errorf("nil registry VisibleTools = %v, want empty", got)
	}
}

func TestHostToolRegistry_VisibleTools_PermissionFilter(t *testing.T) {
	t.Parallel()
	reg := NewHostToolRegistry()
	reg.Register(HostTool{Name: "torrents_list", Permission: "downloader:read", Category: "downloader"}, nopHandler)
	reg.Register(HostTool{Name: "torrents_add", Permission: "downloader:write", Category: "downloader"}, nopHandler)
	reg.Register(HostTool{Name: "subscribe_list", Permission: "subscribe:read", Category: "subscribe"}, nopHandler)
	reg.Register(HostTool{Name: "system.info", Permission: "", Category: "system"}, nopHandler) // 公开

	cases := []struct {
		name  string
		perms []string
		want  []string // tool names 可见
	}{
		{"empty perms: only public", nil, []string{"system.info"}},
		{"read only", []string{"downloader:read"}, []string{"system.info", "torrents_list"}},
		{"wildcard domain", []string{"downloader:*"}, []string{"system.info", "torrents_add", "torrents_list"}},
		{"global star", []string{"*"}, []string{"system.info", "subscribe_list", "torrents_add", "torrents_list"}},
		{"host star", []string{"host:*"}, []string{"system.info", "subscribe_list", "torrents_add", "torrents_list"}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := reg.VisibleTools(c.perms)
			names := toolNames(got)
			if !sameSet(names, c.want) {
				t.Errorf("VisibleTools(%v) = %v, want %v", c.perms, names, c.want)
			}
		})
	}
	// 结果按 Category,Name 排序
	got := reg.VisibleTools([]string{"*"})
	if len(got) < 2 {
		t.Fatalf("expected >=2 tools, got %d", len(got))
	}
	if got[0].Category > got[1].Category || (got[0].Category == got[1].Category && got[0].Name > got[1].Name) {
		t.Errorf("VisibleTools not sorted by (Category,Name): %+v", got[:2])
	}
}

func TestHostToolRegistry_Dispatch(t *testing.T) {
	t.Parallel()
	reg := NewHostToolRegistry()
	reg.Register(HostTool{Name: "ok", Permission: "x:read"}, func(ctx context.Context, _ map[string]any) (any, error) {
		return map[string]any{"echo": 42}, nil
	})
	reg.Register(HostTool{Name: "boom", Permission: "x:read"}, func(ctx context.Context, _ map[string]any) (any, error) {
		return nil, errors.New("kaboom")
	})
	reg.Register(HostTool{Name: "w", Permission: "x:write"}, nopHandler)

	// notFound
	if _, _, notFound, err := reg.Dispatch(context.Background(), nil, "nope", nil); !notFound || err == nil {
		t.Errorf("notFound case: notFound=%v err=%v", notFound, err)
	}
	// 权限不足 → ErrPermissionDenied，perm 回填
	_, perm, notFound, err := reg.Dispatch(context.Background(), []string{"x:read"}, "w", nil)
	if notFound || !errors.Is(err, ErrPermissionDenied) || perm != "x:write" {
		t.Errorf("403 case: notFound=%v err=%v perm=%q", notFound, err, perm)
	}
	// 成功
	out, _, _, err := reg.Dispatch(context.Background(), []string{"x:read"}, "ok", nil)
	if err != nil {
		t.Fatalf("ok case err: %v", err)
	}
	if m, _ := out.(map[string]any); m["echo"] != 42 {
		t.Errorf("ok result = %v, want echo=42", out)
	}
	// handler error
	if _, _, _, err := reg.Dispatch(context.Background(), []string{"x:read"}, "boom", nil); err == nil || !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("boom case err = %v, want kaboom", err)
	}
}

// TestHostToolRegistry_Dispatch_PerToolTimeout：Timeout>0 的 tool 超时后 ctx 被取消。
func TestHostToolRegistry_Dispatch_PerToolTimeout(t *testing.T) {
	reg := NewHostToolRegistry()
	reg.Register(HostTool{Name: "slow", Permission: "", Timeout: 50 * time.Millisecond}, func(ctx context.Context, _ map[string]any) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return "done", nil
		}
	})
	_, _, _, err := reg.Dispatch(context.Background(), nil, "slow", nil)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

// TestDispatch_RawMessage_NoDoubleEscape：handler 返回 json.RawMessage 时，json.Marshal 透传
// 预序列化 JSON，不双重转义。这是 adoptAgentTool 的正确性前提。
func TestDispatch_RawMessage_NoDoubleEscape(t *testing.T) {
	t.Parallel()
	reg := NewHostToolRegistry()
	reg.Register(HostTool{Name: "adopted", Permission: ""}, func(ctx context.Context, _ map[string]any) (any, error) {
		return json.RawMessage(`{"hits":[1,2,3]}`), nil
	})
	out, _, _, err := reg.Dispatch(context.Background(), nil, "adopted", nil)
	if err != nil {
		t.Fatalf("Dispatch err: %v", err)
	}
	raw, merr := json.Marshal(out)
	if merr != nil {
		t.Fatalf("Marshal err: %v", merr)
	}
	// 期望透传为对象，而非被字符串化成 "{\"hits\":[1,2,3]}"
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Errorf("result %q is not valid JSON object (double-escaped?): %v", raw, err)
	}
	hits, _ := obj["hits"].([]any)
	if len(hits) != 3 {
		t.Errorf("hits len = %d, want 3; raw=%s", len(hits), raw)
	}
}

// ===== externalHostServer /tools 端点（httptest） =====

func newToolsTestServer(perms []string, reg *HostToolRegistry) *externalHostServer {
	mgr := &Manager{}
	mgr.SetHostTools(reg)
	p := &externalHTTPPlugin{
		manifest: Manifest{ID: "test-plugin", Permissions: perms},
		manager:  mgr,
	}
	// ServeHTTP 在路由前会检查 currentRuntime() 非 nil（否则 503），故注入一个最小 runtime。
	p.setRuntime(&pluginRuntime{manager: mgr, pluginID: "test-plugin", config: map[string]any{}})
	return &externalHostServer{plugin: p, token: "tok"}
}

func TestExternalHostServer_ToolsList(t *testing.T) {
	t.Parallel()
	reg := NewHostToolRegistry()
	reg.Register(HostTool{Name: "torrents_list", Permission: "downloader:read", Category: "downloader"}, nopHandler)
	reg.Register(HostTool{Name: "torrents_add", Permission: "downloader:write", Category: "downloader"}, nopHandler)

	// 有 read 权限：只见 torrents_list
	h := newToolsTestServer([]string{"downloader:read"}, reg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tools", nil)
	req.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "torrents_list") {
		t.Errorf("body missing torrents_list: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "torrents_add") {
		t.Errorf("body should NOT include torrents_add (no write perm): %s", rec.Body.String())
	}

	// 未注入 registry → 空数组（不 panic）
	h2 := newToolsTestServer(nil, nil)
	rec2 := httptest.NewRecorder()
	req2b := httptest.NewRequest(http.MethodGet, "/tools", nil)
	req2b.Header.Set("Authorization", "Bearer tok")
	h2.ServeHTTP(rec2, req2b)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), `"tools":[]`) {
		t.Errorf("nil registry: code=%d body=%s", rec2.Code, rec2.Body.String())
	}
}

func TestExternalHostServer_ToolCall(t *testing.T) {
	t.Parallel()
	reg := NewHostToolRegistry()
	reg.Register(HostTool{Name: "echo", Permission: "x:read"}, func(ctx context.Context, in map[string]any) (any, error) {
		return map[string]any{"got": in["v"]}, nil
	})
	reg.Register(HostTool{Name: "danger", Permission: "x:write", Destructive: true}, nopHandler)

	// 成功调用 read tool
	h := newToolsTestServer([]string{"x:read"}, reg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tools/echo", strings.NewReader(`{"input":{"v":7}}`))
	req.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("echo code=%d want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"got":7`) {
		t.Errorf("echo body=%s want got=7", rec.Body.String())
	}

	// 权限不足 → 403 + permission 回填
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/tools/danger", strings.NewReader(`{"input":{}}`))
	req2.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Errorf("danger code=%d want 403; body=%s", rec2.Code, rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), "x:write") {
		t.Errorf("403 body should name permission x:write: %s", rec2.Body.String())
	}

	// 未找到 → 404
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/tools/nope", strings.NewReader(`{}`))
	req3.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Errorf("nope code=%d want 404; body=%s", rec3.Code, rec3.Body.String())
	}

	// 未鉴权 → 401
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodPost, "/tools/echo", strings.NewReader(`{}`))
	h.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusUnauthorized {
		t.Errorf("unauth code=%d want 401", rec4.Code)
	}
}

// ===== 辅助 =====

func nopHandler(context.Context, map[string]any) (any, error) { return map[string]any{"ok": true}, nil }

func toolNames(tools []HostTool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(b))
	for _, x := range b {
		set[x] = struct{}{}
	}
	for _, x := range a {
		if _, ok := set[x]; !ok {
			return false
		}
	}
	return true
}
