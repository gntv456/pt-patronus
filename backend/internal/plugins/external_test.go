// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// external.go 纯逻辑/轻 I/O 函数表测。external.go 993 行原 0 覆盖；此处覆盖
// ACL(hasPluginPermission)、模板替换、路径解析、host 鉴权、JSON 编解码、运行时状态快照等
// 无需真实子进程/网络/DB 的面。ensureStarted/Stop/health/post 等需真实进程的留作集成测。

// ── Tier 1：纯函数 ──

func TestRestartDelay(t *testing.T) {
	t.Parallel()
	cases := []struct {
		count int
		want  time.Duration
	}{
		{0, time.Second},
		{-1, time.Second}, // <=0 落到首分支
		{1, 2 * time.Second},
		{2, 5 * time.Second},
		{3, 10 * time.Second},
		{4, 30 * time.Second}, // default 封顶
		{99, 30 * time.Second},
	}
	for _, c := range cases {
		c := c
		t.Run("", func(t *testing.T) {
			t.Parallel()
			if got := restartDelay(c.count); got != c.want {
				t.Errorf("restartDelay(%d) = %v, want %v", c.count, got, c.want)
			}
		})
	}
}

func TestReplaceVars(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, in       string
		port           int
		token, dir, id string
		want           string
	}{
		{"all vars", "py ${PLUGIN_DIR}/main.py ${PORT} ${TOKEN} ${PLUGIN_ID}", 8080, "tok", "/d", "demo",
			"py /d/main.py 8080 tok demo"},
		{"no vars", "no vars here", 0, "", "", "", "no vars here"},
		{"empty string", "", 8080, "t", "/d", "i", ""},
		{"port only", "port=${PORT}", 9090, "", "", "", "port=9090"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := replaceVars(c.in, c.port, c.token, c.dir, c.id); got != c.want {
				t.Errorf("replaceVars() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestResolveWorkDir(t *testing.T) {
	t.Parallel()
	dir := "/plugins/demo"
	abs := filepath.Join(os.TempDir(), "abscwd") // 当前 OS 保证绝对
	cases := []struct {
		name, workDir, want string
	}{
		{"empty defaults to dir", "", dir},
		{"relative joined", "subdir", filepath.Join(dir, "subdir")},
		{"absolute passthrough", abs, abs},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveWorkDir(dir, c.workDir); got != c.want {
				t.Errorf("resolveWorkDir(%q) = %q, want %q", c.workDir, got, c.want)
			}
		})
	}
}

func TestHostPathValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, path, prefix string
		want               string
	}{
		{"normal", "/kv/my-key", "/kv/", "my-key"},
		{"url encoded", "/kv/hello%20world", "/kv/", "hello world"},
		{"nested path", "/sites/123/cookie", "/sites/", "123/cookie"},
		{"empty after strip", "/kv/", "/kv/", ""},
		{"prefix absent passes through", "/other", "/kv/", "/other"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := hostPathValue(c.path, c.prefix)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != c.want {
				t.Errorf("hostPathValue(%q,%q) = %q, want %q", c.path, c.prefix, got, c.want)
			}
		})
	}
	// 非法 %转义 → 报错
	if _, err := hostPathValue("/kv/%", "/kv/"); err == nil {
		t.Error("expected error for invalid escape")
	}
}

func TestCloneTimePtr(t *testing.T) {
	t.Parallel()
	t.Run("nil stays nil", func(t *testing.T) {
		t.Parallel()
		if got := cloneTimePtr(nil); got != nil {
			t.Errorf("cloneTimePtr(nil) = %v, want nil", got)
		}
	})
	t.Run("non-nil copies value to new pointer", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		got := cloneTimePtr(&now)
		if got == &now {
			t.Fatal("must be a different pointer (no aliasing)")
		}
		if !got.Equal(now) {
			t.Errorf("value = %v, want %v", *got, now)
		}
	})
}

// hasPluginPermission 是插件 ACL 核心。注意实际行为：*、host:*、plugin:* 在 switch 里
// 直接命中→对任意 required 都授权（超级授权）；其余 X:* 走后缀前缀匹配仅授 X 类别。
func TestHasPluginPermission(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		perms    []string
		required string
		want     bool
	}{
		{"exact match", []string{"config:read"}, "config:read", true},
		{"star grants all", []string{"*"}, "anything:whatever", true},
		{"host star is full grant", []string{"host:*"}, "config:read", true},
		{"plugin star is full grant", []string{"plugin:*"}, "kv:get", true},
		{"category wildcard hit", []string{"config:*"}, "config:write", true},
		{"category wildcard prefix miss", []string{"config:*"}, "kv:get", false},
		{"kv wildcard not config", []string{"kv:*"}, "config:read", false},
		{"exact mismatch", []string{"config:read"}, "config:write", false},
		{"empty perms", []string{}, "config:read", false},
		{"nil perms", nil, "config:read", false},
		{"perm whitespace trimmed", []string{"  config:read  "}, "config:read", true},
		{"required whitespace trimmed", []string{"config:read"}, "  config:read  ", true},
		{"multiple perms one matches", []string{"kv:get", "config:read", "log:write"}, "config:read", true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := hasPluginPermission(c.perms, c.required); got != c.want {
				t.Errorf("hasPluginPermission(%v,%q) = %v, want %v", c.perms, c.required, got, c.want)
			}
		})
	}
}

// ── Tier 2：轻构造的方法/轻 I/O ──

func TestExternalDefaultConfig(t *testing.T) {
	t.Parallel()
	p := &externalHTTPPlugin{manifest: Manifest{ConfigSchema: []ConfigField{
		{Key: "timeout", Type: "number", Default: 30},
		{Key: "debug", Type: "switch"},
		{Key: "name", Type: "text", Default: "demo"},
		{Key: "url", Type: "text"}, // 无 default → 空串
	}}}
	cfg := p.DefaultConfig()
	if cfg["timeout"] != 30 {
		t.Errorf("timeout default = %v, want 30", cfg["timeout"])
	}
	if cfg["debug"] != false {
		t.Errorf("switch default = %v, want false", cfg["debug"])
	}
	if cfg["name"] != "demo" {
		t.Errorf("text default = %v, want demo", cfg["name"])
	}
	if cfg["url"] != "" {
		t.Errorf("no-default text = %v, want empty", cfg["url"])
	}

	t.Run("empty schema", func(t *testing.T) {
		t.Parallel()
		cfg := (&externalHTTPPlugin{}).DefaultConfig()
		if len(cfg) != 0 {
			t.Errorf("empty schema cfg len = %d, want 0", len(cfg))
		}
	})
}

func TestExternalRuntimeStatus(t *testing.T) {
	t.Parallel()
	t.Run("stopped unmanaged", func(t *testing.T) {
		t.Parallel()
		rs := (&externalHTTPPlugin{}).RuntimeStatus()
		if rs.State != "stopped" || rs.Managed {
			t.Errorf("stopped/unmanaged: state=%q managed=%v", rs.State, rs.Managed)
		}
	})
	t.Run("running with host + managed", func(t *testing.T) {
		t.Parallel()
		p := &externalHTTPPlugin{
			baseURL:  "http://127.0.0.1:8080",
			pid:      1234,
			host:     &externalHostServer{baseURL: "http://127.0.0.1:9000"},
			manifest: Manifest{ID: "demo", Entry: &RuntimeEntry{Command: "python main.py"}},
		}
		rs := p.RuntimeStatus()
		if rs.State != "running" {
			t.Errorf("state = %q, want running", rs.State)
		}
		if !rs.Managed {
			t.Error("managed = false, want true (Entry.Command set)")
		}
		if rs.PID != 1234 {
			t.Errorf("pid = %d, want 1234", rs.PID)
		}
		if rs.HostBaseURL != "http://127.0.0.1:9000" {
			t.Errorf("host base url = %q", rs.HostBaseURL)
		}
	})
	t.Run("entry without command is unmanaged", func(t *testing.T) {
		t.Parallel()
		p := &externalHTTPPlugin{manifest: Manifest{Entry: &RuntimeEntry{Command: "   "}}}
		if p.RuntimeStatus().Managed {
			t.Error("blank command should be unmanaged")
		}
	})
}

func TestExternalURL(t *testing.T) {
	t.Parallel()
	p := &externalHTTPPlugin{baseURL: "http://127.0.0.1:8080"}
	cases := []struct{ in, want string }{
		{"/health", "http://127.0.0.1:8080/health"},
		{"health", "http://127.0.0.1:8080/health"}, // 自动补前导斜杠
		{"", "http://127.0.0.1:8080/"},
	}
	for _, c := range cases {
		if got := p.url(c.in); got != c.want {
			t.Errorf("url(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// baseURL 带尾斜杠 → 去重
	p.baseURL = "http://127.0.0.1:8080/"
	if got := p.url("/health"); got != "http://127.0.0.1:8080/health" {
		t.Errorf("trailing-slash baseURL url = %q", got)
	}
}

func TestExternalAuthorize(t *testing.T) {
	t.Parallel()
	t.Run("with token sets bearer", func(t *testing.T) {
		t.Parallel()
		p := &externalHTTPPlugin{token: "secret"}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		p.authorize(req)
		if got := req.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want Bearer secret", got)
		}
	})
	t.Run("empty token leaves header unset", func(t *testing.T) {
		t.Parallel()
		p := &externalHTTPPlugin{}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		p.authorize(req)
		if got := req.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want empty", got)
		}
	})
}

func TestExternalHostAuthorized(t *testing.T) {
	t.Parallel()
	h := &externalHostServer{token: "host-token"}
	cases := []struct {
		name   string
		bearer string
		xhost  string
		want   bool
	}{
		{"bearer correct", "Bearer host-token", "", true},
		{"x-host-token correct", "", "host-token", true},
		{"bearer wrong", "Bearer wrong", "", false},
		{"x-host-token wrong", "", "wrong", false},
		{"no creds", "", "", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if c.bearer != "" {
				req.Header.Set("Authorization", c.bearer)
			}
			if c.xhost != "" {
				req.Header.Set("X-PTP-Host-Token", c.xhost)
			}
			if got := h.authorized(req); got != c.want {
				t.Errorf("authorized() = %v, want %v", got, c.want)
			}
		})
	}
	// 空 token → 放行全部（未配置鉴权）
	open := &externalHostServer{token: "  "}
	if !open.authorized(httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Error("blank server token should allow all")
	}
}

func TestWriteHostJSON(t *testing.T) {
	t.Parallel()
	t.Run("normal payload", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		writeHostJSON(rec, http.StatusOK, map[string]any{"ok": true})
		if rec.Code != http.StatusOK {
			t.Errorf("code = %d, want 200", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("content-type = %q", ct)
		}
		if got := rec.Body.String(); got != `{"ok":true}` {
			t.Errorf("body = %q, want {\"ok\":true}", got)
		}
	})
	t.Run("marshal failure falls back to 500", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		writeHostJSON(rec, http.StatusOK, make(chan int)) // chan 不可序列化
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("code = %d, want 500", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "json encode failed") {
			t.Errorf("body = %q, want fallback error", rec.Body.String())
		}
	})
}

func TestDecodeHostJSON(t *testing.T) {
	t.Parallel()
	t.Run("valid decode", func(t *testing.T) {
		t.Parallel()
		var in struct {
			Level string `json:"level"`
		}
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"level":"info"}`)))
		rec := httptest.NewRecorder()
		if !decodeHostJSON(rec, req, &in) {
			t.Fatal("expected ok=true")
		}
		if in.Level != "info" {
			t.Errorf("level = %q", in.Level)
		}
	})
	t.Run("invalid json returns 400", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{invalid`)))
		rec := httptest.NewRecorder()
		if decodeHostJSON(rec, req, &struct{}{}) {
			t.Fatal("expected ok=false")
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
	})
	t.Run("nil body returns true no-op", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = nil // httptest 默认用 http.NoBody，显式置 nil 才命中守卫
		rec := httptest.NewRecorder()
		if !decodeHostJSON(rec, req, &struct{}{}) {
			t.Fatal("nil body should return true")
		}
	})
}

func TestExternalHostContext(t *testing.T) {
	t.Parallel()
	t.Run("nil host returns nil", func(t *testing.T) {
		t.Parallel()
		if got := (&externalHTTPPlugin{}).hostContext(); got != nil {
			t.Errorf("nil host hostContext = %v, want nil", got)
		}
	})
	t.Run("with host maps fields", func(t *testing.T) {
		t.Parallel()
		p := &externalHTTPPlugin{
			host:     &externalHostServer{baseURL: "http://host:9000", token: "htok"},
			manifest: Manifest{ID: "demo", Permissions: []string{"log:write", "kv:get"}},
		}
		ctx := p.hostContext()
		if ctx == nil {
			t.Fatal("expected non-nil context")
		}
		if ctx.PluginID != "demo" || ctx.BaseURL != "http://host:9000" || ctx.Token != "htok" {
			t.Errorf("context mapping wrong: %+v", ctx)
		}
		if ctx.ProtocolVersion != 1 {
			t.Errorf("protocol = %d, want 1", ctx.ProtocolVersion)
		}
		if len(ctx.Permissions) != 2 {
			t.Errorf("permissions len = %d, want 2", len(ctx.Permissions))
		}
		if len(ctx.HostCapabilities) == 0 {
			t.Error("host capabilities should be populated")
		}
	})
}

// recordingRuntime 捕获 Log 调用，用于验证 pluginOutputWriter 的行切分逻辑。
type recordingRuntime struct{ logs []string }

func (r *recordingRuntime) Config() map[string]any { return nil }
func (r *recordingRuntime) Log(level, eventName, message string, _ map[string]any) {
	r.logs = append(r.logs, message)
}
func (r *recordingRuntime) Notice(context.Context, NoticeInput) error       { return nil }
func (r *recordingRuntime) Publish(context.Context, string, map[string]any) {}
func (r *recordingRuntime) KVGet(string, any) error                         { return nil }
func (r *recordingRuntime) KVSet(string, any) error                         { return nil }
func (r *recordingRuntime) KVDelete(string) error                           { return nil }
func (r *recordingRuntime) SiteList() []SiteRef                             { return nil }
func (r *recordingRuntime) SiteCookie(uint) (string, error)                 { return "", nil }
func (r *recordingRuntime) SetRuntimeConfig(string, any) error              { return nil }

func newWriter(rt Runtime) *pluginOutputWriter {
	p := &externalHTTPPlugin{}
	p.setRuntime(rt)
	return &pluginOutputWriter{plugin: p, level: "info", eventName: "test"}
}

func TestPluginOutputWriterLineSplitting(t *testing.T) {
	t.Parallel()
	t.Run("complete line flushes immediately", func(t *testing.T) {
		t.Parallel()
		rt := &recordingRuntime{}
		w := newWriter(rt)
		if _, err := w.Write([]byte("hello\n")); err != nil {
			t.Fatal(err)
		}
		if len(rt.logs) != 1 || rt.logs[0] != "hello" {
			t.Errorf("logs = %v, want [hello]", rt.logs)
		}
	})
	t.Run("partial line buffers until newline", func(t *testing.T) {
		t.Parallel()
		rt := &recordingRuntime{}
		w := newWriter(rt)
		_, _ = w.Write([]byte("par")) // 无 \n → 缓冲
		if len(rt.logs) != 0 {
			t.Errorf("after partial: logs = %v, want []", rt.logs)
		}
		_, _ = w.Write([]byte("tial\n")) // 补全 → 刷新
		if len(rt.logs) != 1 || rt.logs[0] != "partial" {
			t.Errorf("after completion: logs = %v, want [partial]", rt.logs)
		}
	})
	t.Run("empty lines suppressed", func(t *testing.T) {
		t.Parallel()
		rt := &recordingRuntime{}
		w := newWriter(rt)
		_, _ = w.Write([]byte("\n\n\n"))
		if len(rt.logs) != 0 {
			t.Errorf("blank lines should be suppressed, logs = %v", rt.logs)
		}
	})
	t.Run("overlong line truncated to 4000", func(t *testing.T) {
		t.Parallel()
		rt := &recordingRuntime{}
		w := newWriter(rt)
		long := strings.Repeat("x", 5000) + "\n"
		_, _ = w.Write([]byte(long))
		if len(rt.logs) != 1 {
			t.Fatalf("logs len = %d, want 1", len(rt.logs))
		}
		if len(rt.logs[0]) != 4000 {
			t.Errorf("truncated len = %d, want 4000", len(rt.logs[0]))
		}
	})
}

// TestPostDecodeLimited 验证 external plugin 响应解码有 1MB 上限：
// 超大响应体不应让 host 进程 OOM，应返回读取/解码错误。
func TestPostDecodeLimited(t *testing.T) {
	bigPayload := strings.Repeat("x", 2<<20) // 2MB > 1MB 上限
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 不论路径都回大 JSON 字符串
		_, _ = w.Write([]byte(`{"output":"` + bigPayload + `"}`))
	}))
	defer server.Close()

	p := &externalHTTPPlugin{
		manifest: Manifest{ID: "big", Entry: &RuntimeEntry{BaseURL: server.URL, Health: "/health"}},
		client:   &http.Client{Timeout: 30 * time.Second},
		baseURL:  server.URL,
	}
	// 直接调 post：baseURL 已设，避免走 ensureStarted 的健康检查路径。
	var out map[string]any
	err := p.post(context.Background(), "/anything", map[string]any{}, &out)
	// httptest 默认 200，post 不会因 status 报错；超大 body 解码应失败。
	if err == nil {
		t.Fatalf("expected decode error for >1MB body, got nil; out=%v", out)
	}
}
