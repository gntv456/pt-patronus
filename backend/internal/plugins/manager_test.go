package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

func newPluginTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.PluginState{}, &models.PluginLog{}, &models.PluginKV{}, &models.PluginRelease{}, &models.SystemConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// mustRegister 把内置插件显式注册进测试 manager。自 fe52adb 起 feature builtins 迁至
// 开放插件市场，NewManager 不再自动注册它们；测试需要时在此显式注册（Go 实现仍在仓内）。
func mustRegister(t *testing.T, m *Manager, p Plugin) {
	t.Helper()
	if err := m.Register(p); err != nil {
		t.Fatalf("register %s: %v", p.Manifest().ID, err)
	}
}

func TestExternalPluginHelperProcess(t *testing.T) {
	if os.Getenv("PTP_TEST_HELPER_PROCESS") != "1" {
		return
	}
	port := os.Getenv("PTP_PLUGIN_PORT")
	if port == "" {
		os.Exit(2)
	}
	if expected := os.Getenv("PTP_TEST_EXPECT_HOST_PERMISSIONS"); expected != "" && os.Getenv("PTP_HOST_PERMISSIONS") != expected {
		os.Exit(4)
	}
	exitAfterAction := os.Getenv("PTP_TEST_EXIT_AFTER_ACTION") == "1"
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/action", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(externalActionResponse{OK: true, Output: map[string]any{"ok": true}})
		if exitAfterAction {
			go func() {
				time.Sleep(100 * time.Millisecond)
				os.Exit(0)
			}()
		}
	})
	server := &http.Server{
		Addr:    net.JoinHostPort("127.0.0.1", port),
		Handler: mux,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestManagerHandlesEnabledPluginEvent(t *testing.T) {
	db := newPluginTestDB(t)

	bus := event.New()
	m := NewManager(db, bus, nil, nil, nil)
	mustRegister(t, m, NewEventAuditPlugin())
	m.Start(context.Background())

	if err := m.SetEnabled("event-audit", true); err != nil {
		t.Fatalf("enable plugin: %v", err)
	}
	bus.Publish(context.Background(), "plugin.test", map[string]any{"message": "hello"})

	logs, err := m.Logs("event-audit", 20)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	found := false
	for _, l := range logs {
		if l.Event == "plugin.test" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected plugin.test log, got %#v", logs)
	}
}

func TestManagerLoadsExternalHTTPPlugin(t *testing.T) {
	var eventHit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/event":
			eventHit = true
			w.WriteHeader(http.StatusOK)
		case "/action":
			var req externalActionRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(externalActionResponse{
				OK:     true,
				Output: map[string]any{"action": req.Action, "input": req.Input},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	dir := filepath.Join(root, "external-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := Manifest{
		ID:          "external-demo",
		Name:        "External Demo",
		Version:     "1.0.0",
		Runtime:     RuntimeExternalHTTP,
		Entry:       &RuntimeEntry{BaseURL: server.URL, Health: "/health"},
		Events:      []EventSpec{{Type: "plugin.test"}},
		Actions:     []ActionSpec{{Name: "ping", Label: "Ping"}},
		Permissions: []string{"event:read"},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	db := newPluginTestDB(t)
	bus := event.New()
	m := NewManager(db, bus, nil, nil, nil)
	if err := m.LoadExternal(root); err != nil {
		t.Fatalf("load external: %v", err)
	}
	m.Start(context.Background())
	if err := m.SetEnabled("external-demo", true); err != nil {
		t.Fatalf("enable external: %v", err)
	}
	out, err := m.ExecuteAction(context.Background(), "external-demo", "ping", map[string]any{"hello": "world"})
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if out["action"] != "ping" {
		t.Fatalf("unexpected output: %#v", out)
	}
	view, err := m.Get("external-demo")
	if err != nil {
		t.Fatalf("get plugin view: %v", err)
	}
	if view.RuntimeStatus == nil || view.RuntimeStatus.State != "running" || view.RuntimeStatus.BaseURL == "" || view.RuntimeStatus.HostBaseURL == "" {
		t.Fatalf("unexpected runtime status: %#v", view.RuntimeStatus)
	}
	bus.Publish(context.Background(), "plugin.test", map[string]any{"message": "ok"})
	if !eventHit {
		t.Fatal("expected external plugin event endpoint to be called")
	}
}

func TestExternalHTTPPluginCanCallHostAPI(t *testing.T) {
	var published bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/action":
			var req externalActionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode action request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.Host == nil || req.Host.BaseURL == "" || req.Host.Token == "" {
				t.Errorf("expected host context, got %#v", req.Host)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			hostReq := func(method, path string, body any, out any) {
				t.Helper()
				var rd io.Reader
				if body != nil {
					raw, _ := json.Marshal(body)
					rd = bytes.NewReader(raw)
				}
				hr, err := http.NewRequest(method, req.Host.BaseURL+path, rd)
				if err != nil {
					t.Fatalf("new host request: %v", err)
				}
				hr.Header.Set("Authorization", "Bearer "+req.Host.Token)
				if body != nil {
					hr.Header.Set("Content-Type", "application/json")
				}
				res, err := http.DefaultClient.Do(hr)
				if err != nil {
					t.Fatalf("host request %s %s: %v", method, path, err)
				}
				defer res.Body.Close()
				if res.StatusCode < 200 || res.StatusCode >= 300 {
					raw, _ := io.ReadAll(res.Body)
					t.Fatalf("host request %s %s status %d: %s", method, path, res.StatusCode, raw)
				}
				if out != nil {
					if err := json.NewDecoder(res.Body).Decode(out); err != nil {
						t.Fatalf("decode host response: %v", err)
					}
				}
			}

			hostReq(http.MethodPost, "/log", map[string]any{
				"level":   "info",
				"event":   "host-api",
				"message": "hello from external plugin",
			}, nil)
			hostReq(http.MethodPut, "/kv/demo", map[string]any{"value": map[string]any{"ok": true}}, nil)
			var kv struct {
				Found bool           `json:"found"`
				Value map[string]any `json:"value"`
			}
			hostReq(http.MethodGet, "/kv/demo", nil, &kv)
			if !kv.Found || kv.Value["ok"] != true {
				t.Fatalf("unexpected kv response: %#v", kv)
			}
			hostReq(http.MethodPost, "/events", map[string]any{
				"type": "plugin.host.test",
				"data": map[string]any{"source": "external"},
			}, nil)
			_ = json.NewEncoder(w).Encode(externalActionResponse{
				OK:     true,
				Output: map[string]any{"host": true, "kv_found": kv.Found},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	dir := filepath.Join(root, "external-host-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := Manifest{
		ID:      "external-host-demo",
		Name:    "External Host Demo",
		Version: "1.0.0",
		Runtime: RuntimeExternalHTTP,
		Entry:   &RuntimeEntry{BaseURL: server.URL, Health: "/health"},
		Actions: []ActionSpec{{Name: "use_host", Label: "Use Host"}},
		Permissions: []string{
			"log:write",
			"kv:read",
			"kv:write",
			"event:publish",
		},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	db := newPluginTestDB(t)
	bus := event.New()
	bus.Subscribe("plugin.host.test", 1, func(ctx context.Context, e event.Event) {
		published = e.Data["source"] == "external"
	})
	m := NewManager(db, bus, nil, nil, oper.NewPluginKVOper(db))
	if err := m.LoadExternal(root); err != nil {
		t.Fatalf("load external: %v", err)
	}
	m.Start(context.Background())
	if err := m.SetEnabled("external-host-demo", true); err != nil {
		t.Fatalf("enable external: %v", err)
	}
	out, err := m.ExecuteAction(context.Background(), "external-host-demo", "use_host", nil)
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if out["host"] != true || out["kv_found"] != true {
		t.Fatalf("unexpected output: %#v", out)
	}
	if !published {
		t.Fatal("expected host event to be published")
	}
	logs, err := m.Logs("external-host-demo", 20)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	foundLog := false
	for _, l := range logs {
		if l.Event == "host-api" {
			foundLog = true
			break
		}
	}
	if !foundLog {
		t.Fatalf("expected host-api log, got %#v", logs)
	}
}

func TestExternalHostAPIRequiresTokenAndPermission(t *testing.T) {
	var host *externalHostContext
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/action":
			var req externalActionRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			host = req.Host
			_ = json.NewEncoder(w).Encode(externalActionResponse{OK: true, Output: map[string]any{"ok": true}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	dir := filepath.Join(root, "external-permission-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := Manifest{
		ID:          "external-permission-demo",
		Name:        "External Permission Demo",
		Version:     "1.0.0",
		Runtime:     RuntimeExternalHTTP,
		Entry:       &RuntimeEntry{BaseURL: server.URL, Health: "/health"},
		Actions:     []ActionSpec{{Name: "capture_host", Label: "Capture Host"}},
		Permissions: []string{"log:write"},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, oper.NewPluginKVOper(db))
	if err := m.LoadExternal(root); err != nil {
		t.Fatalf("load external: %v", err)
	}
	m.Start(context.Background())
	if err := m.SetEnabled("external-permission-demo", true); err != nil {
		t.Fatalf("enable external: %v", err)
	}
	if _, err := m.ExecuteAction(context.Background(), "external-permission-demo", "capture_host", nil); err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if host == nil {
		t.Fatal("expected host context")
	}

	res, err := http.Get(host.BaseURL + "/config")
	if err != nil {
		t.Fatalf("unauthorized request: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status, got %d", res.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, host.BaseURL+"/kv/demo", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+host.Token)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("permission request: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected forbidden status, got %d", res.StatusCode)
	}
}

func TestExternalHTTPPluginShutdownEndpoint(t *testing.T) {
	var shutdownHit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/shutdown":
			shutdownHit = true
			w.WriteHeader(http.StatusOK)
		case "/action":
			_ = json.NewEncoder(w).Encode(externalActionResponse{OK: true, Output: map[string]any{"ok": true}})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	dir := filepath.Join(root, "external-shutdown-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := Manifest{
		ID:      "external-shutdown-demo",
		Name:    "External Shutdown Demo",
		Version: "1.0.0",
		Runtime: RuntimeExternalHTTP,
		Entry:   &RuntimeEntry{BaseURL: server.URL, Health: "/health", Shutdown: "/shutdown"},
		Actions: []ActionSpec{{Name: "noop", Label: "Noop"}},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(root)
	if err := m.LoadExternal(root); err != nil {
		t.Fatalf("load external: %v", err)
	}
	if err := m.SetEnabled("external-shutdown-demo", true); err != nil {
		t.Fatalf("enable external: %v", err)
	}
	if _, err := m.ExecuteAction(context.Background(), "external-shutdown-demo", "noop", nil); err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if err := m.Uninstall("external-shutdown-demo"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !shutdownHit {
		t.Fatal("expected shutdown endpoint to be called")
	}
}

func TestExternalPluginReadAsset(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "external-asset-demo")
	webDir := filepath.Join(dir, "web")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatalf("mkdir web: %v", err)
	}
	manifest := Manifest{
		ID:      "external-asset-demo",
		Name:    "External Asset Demo",
		Version: "1.0.0",
		Runtime: RuntimeExternalHTTP,
		Entry:   &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<h1>hello</h1>"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(root)
	if err := m.LoadExternal(root); err != nil {
		t.Fatalf("load external: %v", err)
	}
	asset, err := m.ReadAsset("external-asset-demo", "index.html")
	if err != nil {
		t.Fatalf("read asset: %v", err)
	}
	if string(asset.Data) != "<h1>hello</h1>" {
		t.Fatalf("unexpected asset: %s", asset.Data)
	}
	if _, err := m.ReadAsset("external-asset-demo", "../plugin.json"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestExternalHTTPPluginAutoRestart(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "external-restart-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := Manifest{
		ID:      "external-restart-demo",
		Name:    "External Restart Demo",
		Version: "1.0.0",
		Runtime: RuntimeExternalHTTP,
		Entry: &RuntimeEntry{
			Command: exe,
			Args:    []string{"-test.run=TestExternalPluginHelperProcess"},
			Env: map[string]string{
				"PTP_TEST_HELPER_PROCESS":          "1",
				"PTP_TEST_EXIT_AFTER_ACTION":       "1",
				"PTP_TEST_EXPECT_HOST_PERMISSIONS": "kv:read,downloader:*",
			},
			Health: "/health",
		},
		Permissions: []string{"kv:read", "downloader:*"},
		Actions:     []ActionSpec{{Name: "ping", Label: "Ping"}},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(root)
	if err := m.LoadExternal(root); err != nil {
		t.Fatalf("load external: %v", err)
	}
	if err := m.SetEnabled("external-restart-demo", true); err != nil {
		t.Fatalf("enable external: %v", err)
	}
	defer func() {
		_ = m.SetEnabled("external-restart-demo", false)
		time.Sleep(300 * time.Millisecond)
	}()
	if _, err := m.ExecuteAction(context.Background(), "external-restart-demo", "ping", nil); err != nil {
		t.Fatalf("execute action: %v", err)
	}
	time.Sleep(2200 * time.Millisecond)
	view, err := m.Get("external-restart-demo")
	if err != nil {
		t.Fatalf("get view: %v", err)
	}
	if view.RuntimeStatus == nil {
		t.Fatal("expected runtime status")
	}
	if view.RuntimeStatus.RestartCount < 1 {
		t.Fatalf("expected restart count >= 1, got %#v", view.RuntimeStatus)
	}
	if view.RuntimeStatus.State != "running" {
		t.Fatalf("expected restarted plugin to be running, got %#v", view.RuntimeStatus)
	}
}

func TestDisableStopsManagedExternalPlugin(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "external-stop-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := Manifest{
		ID:      "external-stop-demo",
		Name:    "External Stop Demo",
		Version: "1.0.0",
		Runtime: RuntimeExternalHTTP,
		Entry: &RuntimeEntry{
			Command: exe,
			Args:    []string{"-test.run=TestExternalPluginHelperProcess"},
			Env: map[string]string{
				"PTP_TEST_HELPER_PROCESS": "1",
			},
			Health: "/health",
		},
		Actions: []ActionSpec{{Name: "ping", Label: "Ping"}},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(root)
	if err := m.LoadExternal(root); err != nil {
		t.Fatalf("load external: %v", err)
	}
	if err := m.SetEnabled("external-stop-demo", true); err != nil {
		t.Fatalf("enable external: %v", err)
	}
	defer func() {
		_ = m.SetEnabled("external-stop-demo", false)
		time.Sleep(300 * time.Millisecond)
	}()
	if _, err := m.ExecuteAction(context.Background(), "external-stop-demo", "ping", nil); err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if err := m.SetEnabled("external-stop-demo", false); err != nil {
		t.Fatalf("disable external: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	view, err := m.Get("external-stop-demo")
	if err != nil {
		t.Fatalf("get view: %v", err)
	}
	if view.RuntimeStatus == nil || view.RuntimeStatus.State == "running" {
		t.Fatalf("expected stopped runtime status, got %#v", view.RuntimeStatus)
	}
}

func TestPluginBridgeKVAndPermissions(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "external-bridge-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := Manifest{
		ID:      "external-bridge-demo",
		Name:    "External Bridge Demo",
		Version: "1.0.0",
		Runtime: RuntimeExternalHTTP,
		Entry:   &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
		Permissions: []string{
			"kv:read",
			"kv:write",
			"log:write",
		},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, oper.NewPluginKVOper(db))
	m.SetPluginRoot(root)
	if err := m.LoadExternal(root); err != nil {
		t.Fatalf("load external: %v", err)
	}
	if err := m.SetEnabled("external-bridge-demo", true); err != nil {
		t.Fatalf("enable external: %v", err)
	}
	capsOut, err := m.Bridge(context.Background(), "external-bridge-demo", "capabilities", nil)
	if err != nil {
		t.Fatalf("bridge capabilities: %v", err)
	}
	hostCaps, ok := capsOut["host_capabilities"].([]string)
	if !ok || len(hostCaps) == 0 {
		t.Fatalf("expected host_capabilities in bridge capabilities, got %#v", capsOut)
	}
	if _, err := m.Bridge(context.Background(), "external-bridge-demo", "kv.set", map[string]any{"key": "demo", "value": "ok"}); err != nil {
		t.Fatalf("bridge kv.set: %v", err)
	}
	out, err := m.Bridge(context.Background(), "external-bridge-demo", "kv.get", map[string]any{"key": "demo"})
	if err != nil {
		t.Fatalf("bridge kv.get: %v", err)
	}
	if out["found"] != true || out["value"] != "ok" {
		t.Fatalf("unexpected bridge kv output: %#v", out)
	}
	if _, err := m.Bridge(context.Background(), "external-bridge-demo", "event.publish", map[string]any{"type": "bridge.test"}); err == nil {
		t.Fatal("expected event.publish permission error")
	}
}

func TestInstallFromMarket(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "plugins")
	pkgDir := filepath.Join(root, "packages")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir packages: %v", err)
	}
	manifest := Manifest{
		ID:          "market-demo",
		Name:        "Market Demo",
		Version:     "1.0.0",
		APIVersion:  CurrentPluginAPIVersion,
		AppVersion:  ">=0.1.0",
		Runtime:     RuntimeExternalHTTP,
		Entry:       &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
		Permissions: []string{"event:read"},
	}
	manifestV2 := manifest
	manifestV2.Version = "2.0.0"
	zipBytes := makePluginZip(t, manifest, map[string]string{"plugin.py": "print('v1')\n"})
	zipBytesV2 := makePluginZip(t, manifestV2, map[string]string{"plugin.py": "print('v2')\n"})
	sum := sha256.Sum256(zipBytes)
	sumV2 := sha256.Sum256(zipBytesV2)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sig := ed25519.Sign(priv, zipBytes)
	sigV2 := ed25519.Sign(priv, zipBytesV2)
	archivePath := filepath.Join(pkgDir, "market-demo.zip")
	archivePathV2 := filepath.Join(pkgDir, "market-demo-v2.zip")
	if err := os.WriteFile(archivePath, zipBytes, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	if err := os.WriteFile(archivePathV2, zipBytesV2, 0o644); err != nil {
		t.Fatalf("write archive v2: %v", err)
	}
	index := MarketIndex{
		Version:   1,
		PublicKey: base64.StdEncoding.EncodeToString(pub),
		Plugins: []MarketPlugin{{
			ID:               manifest.ID,
			Name:             manifest.Name,
			Version:          manifest.Version,
			APIVersion:       manifest.APIVersion,
			AppVersion:       manifest.AppVersion,
			HostCapabilities: []string{HostCapabilityRuntimeExternalHTTPBaseURL},
			Publisher:        &MarketPublisher{Name: "Test Publisher", Verified: true},
			PublishedAt:      "2026-07-09T00:00:00Z",
			Changelog:        []string{"Initial release"},
			Runtime:          manifest.Runtime,
			Archive:          "packages/market-demo.zip",
			SHA256:           hex.EncodeToString(sum[:]),
			Signature:        base64.StdEncoding.EncodeToString(sig),
			Permissions:      manifest.Permissions,
		}},
	}
	rawIndex, _ := json.Marshal(index)
	indexPath := filepath.Join(root, "plugin-market.json")
	if err := os.WriteFile(indexPath, rawIndex, 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(pluginRoot)
	trustTestPublicKey(t, db, m, pub) // 测试自签插件：把测试公钥注入受信集（反映生产信任模型）
	res, err := m.InstallFromMarket(context.Background(), indexPath, "market-demo", manifest.Permissions)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if res.RestartNeeded {
		t.Fatal("expected hot load without restart")
	}
	if _, err := os.Stat(filepath.Join(pluginRoot, "market-demo", "plugin.json")); err != nil {
		t.Fatalf("installed plugin.json missing: %v", err)
	}
	if _, err := m.Get("market-demo"); err != nil {
		t.Fatalf("expected hot-loaded plugin: %v", err)
	}
	market, err := m.FetchMarket(context.Background(), indexPath)
	if err != nil {
		t.Fatalf("fetch market: %v", err)
	}
	if market.APIVersion != CurrentPluginAPIVersion || market.AppVersion != currentAppVersion() {
		t.Fatalf("expected market compatibility context, got %#v", market)
	}
	if len(market.HostCapabilities) == 0 {
		t.Fatalf("expected market host capabilities, got %#v", market)
	}
	if !market.Plugins[0].Compatible || !market.Plugins[0].Installed || market.Plugins[0].InstalledVersion != "1.0.0" || market.Plugins[0].UpdateAvailable || market.Plugins[0].TrustLevel != "verified-signed" {
		t.Fatal("expected installed flag after install")
	}
	index.Plugins[0].Version = manifestV2.Version
	index.Plugins[0].Archive = "packages/market-demo-v2.zip"
	index.Plugins[0].SHA256 = hex.EncodeToString(sumV2[:])
	index.Plugins[0].Signature = base64.StdEncoding.EncodeToString(sigV2)
	rawIndex, _ = json.Marshal(index)
	if err := os.WriteFile(indexPath, rawIndex, 0o644); err != nil {
		t.Fatalf("rewrite index: %v", err)
	}
	market, err = m.FetchMarket(context.Background(), indexPath)
	if err != nil {
		t.Fatalf("fetch market v2: %v", err)
	}
	if !market.Plugins[0].UpdateAvailable || market.Plugins[0].InstalledVersion != "1.0.0" {
		t.Fatalf("expected update available, got %#v", market.Plugins[0])
	}
	if _, err := m.InstallFromMarket(context.Background(), indexPath, "market-demo", manifest.Permissions); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	view, err := m.Get("market-demo")
	if err != nil || view.Version != "2.0.0" {
		t.Fatalf("expected upgraded plugin version 2.0.0, got view=%#v err=%v", view, err)
	}
	releases, err := m.ListReleases("market-demo", 10)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	if len(releases) < 2 {
		t.Fatalf("expected at least two releases, got %#v", releases)
	}
	var v1ReleaseID uint
	for _, rel := range releases {
		if rel.Version == "1.0.0" {
			v1ReleaseID = rel.ID
			break
		}
	}
	if v1ReleaseID == 0 {
		t.Fatalf("expected v1 release in history, got %#v", releases)
	}
	if _, err := m.Rollback(context.Background(), "market-demo", v1ReleaseID); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	view, err = m.Get("market-demo")
	if err != nil || view.Version != "1.0.0" {
		t.Fatalf("expected rolled back version 1.0.0, got view=%#v err=%v", view, err)
	}
	if err := m.Uninstall("market-demo"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginRoot, "market-demo")); !os.IsNotExist(err) {
		t.Fatalf("expected plugin directory removed, err=%v", err)
	}
}

func TestSetEnabledBlocksOnMissingDependency(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "plugins")
	dir := filepath.Join(pluginRoot, "dependency-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	manifest := Manifest{
		ID:           "dependency-demo",
		Name:         "Dependency Demo",
		Version:      "1.0.0",
		APIVersion:   CurrentPluginAPIVersion,
		Runtime:      RuntimeExternalHTTP,
		Entry:        &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
		Dependencies: []PluginDependency{{ID: "json-toolbox", Reason: "JSON helpers are required"}},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewJSONToolboxPlugin())
	m.SetPluginRoot(pluginRoot)
	if err := m.LoadExternal(pluginRoot); err != nil {
		t.Fatalf("load external: %v", err)
	}
	if err := m.SetEnabled("dependency-demo", true); err == nil || !strings.Contains(err.Error(), "json-toolbox") {
		t.Fatalf("expected dependency enable error, got %v", err)
	}
	view, err := m.Get("dependency-demo")
	if err != nil {
		t.Fatalf("get dependency plugin: %v", err)
	}
	if view.CanEnable || len(view.DependenciesStatus) != 1 || view.DependenciesStatus[0].Satisfied {
		t.Fatalf("expected unmet dependency status, got %#v", view)
	}
	if err := m.SetEnabled("json-toolbox", true); err != nil {
		t.Fatalf("enable dependency plugin: %v", err)
	}
	if err := m.SetEnabled("dependency-demo", true); err != nil {
		t.Fatalf("enable dependency demo: %v", err)
	}
	if err := m.SetEnabled("json-toolbox", false); err == nil || !strings.Contains(err.Error(), "dependency-demo") {
		t.Fatalf("expected dependency disable protection, got %v", err)
	}
	if err := m.Uninstall("json-toolbox"); err == nil || !strings.Contains(err.Error(), "dependency-demo") {
		t.Fatalf("expected dependency uninstall protection, got %v", err)
	}
	view, err = m.Get("dependency-demo")
	if err != nil {
		t.Fatalf("get dependency plugin after enable: %v", err)
	}
	if !view.CanEnable || !view.DependenciesStatus[0].Satisfied {
		t.Fatalf("expected dependency satisfied after enabling json-toolbox, got %#v", view)
	}
}

func TestMarketDependencyInstallableStatus(t *testing.T) {
	root := t.TempDir()
	index := MarketIndex{
		Version: 1,
		Plugins: []MarketPlugin{{
			ID:           "market-with-dependency",
			Name:         "Market With Dependency",
			Version:      "1.0.0",
			APIVersion:   CurrentPluginAPIVersion,
			Runtime:      RuntimeExternalHTTP,
			Archive:      "archives/market-with-dependency.zip",
			Dependencies: []PluginDependency{{ID: "json-toolbox", Reason: "JSON helpers are required"}},
		}},
	}
	rawIndex, _ := json.Marshal(index)
	indexPath := filepath.Join(root, "plugin-market.json")
	if err := os.WriteFile(indexPath, rawIndex, 0o644); err != nil {
		t.Fatalf("write market index: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewJSONToolboxPlugin())
	m.SetPluginRoot(filepath.Join(root, "plugins"))

	market, err := m.FetchMarket(context.Background(), indexPath)
	if err != nil {
		t.Fatalf("fetch market: %v", err)
	}
	item := market.Plugins[0]
	if !item.Installable || !item.DependenciesSatisfied || len(item.DependenciesStatus) != 1 {
		t.Fatalf("expected installed dependency to satisfy market install, got %#v", item)
	}
	if !item.DependenciesStatus[0].Installed || item.DependenciesStatus[0].Enabled || !item.DependenciesStatus[0].Satisfied {
		t.Fatalf("expected disabled installed dependency to remain install-satisfied, got %#v", item.DependenciesStatus[0])
	}
}

func TestInstallFromMarketResolvesDependencyChain(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "plugins")
	pkgDir := filepath.Join(root, "packages")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir packages: %v", err)
	}

	dependencyManifest := Manifest{
		ID:          "market-dependency",
		Name:        "Market Dependency",
		Version:     "1.0.0",
		APIVersion:  CurrentPluginAPIVersion,
		Runtime:     RuntimeExternalHTTP,
		Entry:       &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
		Permissions: []string{"kv:read"},
	}
	appManifest := Manifest{
		ID:         "market-app",
		Name:       "Market App",
		Version:    "1.0.0",
		APIVersion: CurrentPluginAPIVersion,
		Runtime:    RuntimeExternalHTTP,
		Entry:      &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
		Dependencies: []PluginDependency{{
			ID:      dependencyManifest.ID,
			Version: ">=1.0.0",
			Reason:  "Dependency plugin is required",
		}},
		Permissions: []string{"event:read"},
	}

	dependencyZip := makePluginZip(t, dependencyManifest, map[string]string{"plugin.py": "print('dep')\n"})
	appZip := makePluginZip(t, appManifest, map[string]string{"plugin.py": "print('app')\n"})
	dependencySum := sha256.Sum256(dependencyZip)
	appSum := sha256.Sum256(appZip)

	dependencyArchive := filepath.Join(pkgDir, "market-dependency.zip")
	appArchive := filepath.Join(pkgDir, "market-app.zip")
	if err := os.WriteFile(dependencyArchive, dependencyZip, 0o644); err != nil {
		t.Fatalf("write dependency archive: %v", err)
	}
	if err := os.WriteFile(appArchive, appZip, 0o644); err != nil {
		t.Fatalf("write app archive: %v", err)
	}

	index := MarketIndex{
		Version: 1,
		Plugins: []MarketPlugin{
			{
				ID:          dependencyManifest.ID,
				Name:        dependencyManifest.Name,
				Version:     dependencyManifest.Version,
				APIVersion:  dependencyManifest.APIVersion,
				Runtime:     dependencyManifest.Runtime,
				Archive:     "packages/market-dependency.zip",
				SHA256:      hex.EncodeToString(dependencySum[:]),
				Permissions: dependencyManifest.Permissions,
			},
			{
				ID:           appManifest.ID,
				Name:         appManifest.Name,
				Version:      appManifest.Version,
				APIVersion:   appManifest.APIVersion,
				Runtime:      appManifest.Runtime,
				Archive:      "packages/market-app.zip",
				SHA256:       hex.EncodeToString(appSum[:]),
				Permissions:  appManifest.Permissions,
				Dependencies: appManifest.Dependencies,
			},
		},
	}
	rawIndex, _ := json.Marshal(index)
	indexPath := filepath.Join(root, "plugin-market.json")
	if err := os.WriteFile(indexPath, rawIndex, 0o644); err != nil {
		t.Fatalf("write market index: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(pluginRoot)

	market, err := m.FetchMarket(context.Background(), indexPath)
	if err != nil {
		t.Fatalf("fetch market: %v", err)
	}
	var appItem MarketPlugin
	for _, plugin := range market.Plugins {
		if plugin.ID == appManifest.ID {
			appItem = plugin
			break
		}
	}
	if !appItem.Installable || !appItem.DependenciesSatisfied {
		t.Fatalf("expected app item to be installable, got %#v", appItem)
	}
	if len(appItem.InstallPlan) != 2 {
		t.Fatalf("expected two-step install plan, got %#v", appItem.InstallPlan)
	}
	if appItem.InstallPlan[0].ID != dependencyManifest.ID || appItem.InstallPlan[0].Action != installPlanActionInstall {
		t.Fatalf("expected dependency install step first, got %#v", appItem.InstallPlan)
	}
	if appItem.InstallPlan[1].ID != appManifest.ID || appItem.InstallPlan[1].Action != installPlanActionInstall {
		t.Fatalf("expected app install step last, got %#v", appItem.InstallPlan)
	}
	if len(appItem.DependenciesStatus) != 1 || !appItem.DependenciesStatus[0].Satisfied || !strings.Contains(appItem.DependenciesStatus[0].Message, "will be installed from market") {
		t.Fatalf("expected dependency to be auto-resolved, got %#v", appItem.DependenciesStatus)
	}

	if _, err := m.InstallFromMarketWithOptions(context.Background(), indexPath, appManifest.ID, MarketInstallOptions{
		AcceptedPermissions: appManifest.Permissions,
	}); err == nil || !strings.Contains(err.Error(), dependencyManifest.ID) {
		t.Fatalf("expected missing dependency permission error, got %v", err)
	}

	res, err := m.InstallFromMarketWithOptions(context.Background(), indexPath, appManifest.ID, MarketInstallOptions{
		AcceptedPermissions: appManifest.Permissions,
		AcceptedPermissionsByPlugin: map[string][]string{
			dependencyManifest.ID: dependencyManifest.Permissions,
		},
		AllowUnsigned: true, // 该测试市场源插件未签名，显式 opt-in（测试聚焦依赖链解析，非签名）
	})
	if err != nil {
		t.Fatalf("install with dependency chain: %v", err)
	}
	if len(res.Items) != 2 || res.Items[0].ID != dependencyManifest.ID || res.Items[1].ID != appManifest.ID {
		t.Fatalf("expected install result items for dependency chain, got %#v", res.Items)
	}
	if _, err := os.Stat(filepath.Join(pluginRoot, dependencyManifest.ID, "plugin.json")); err != nil {
		t.Fatalf("dependency plugin missing after install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginRoot, appManifest.ID, "plugin.json")); err != nil {
		t.Fatalf("app plugin missing after install: %v", err)
	}
	if _, err := m.Get(dependencyManifest.ID); err != nil {
		t.Fatalf("expected dependency plugin loaded: %v", err)
	}
	if _, err := m.Get(appManifest.ID); err != nil {
		t.Fatalf("expected app plugin loaded: %v", err)
	}
}

func TestFetchMarketBlocksDependencyCycles(t *testing.T) {
	root := t.TempDir()
	index := MarketIndex{
		Version: 1,
		Plugins: []MarketPlugin{
			{
				ID:           "cycle-a",
				Name:         "Cycle A",
				Version:      "1.0.0",
				APIVersion:   CurrentPluginAPIVersion,
				Runtime:      RuntimeExternalHTTP,
				Archive:      "archives/cycle-a.zip",
				Dependencies: []PluginDependency{{ID: "cycle-b"}},
			},
			{
				ID:           "cycle-b",
				Name:         "Cycle B",
				Version:      "1.0.0",
				APIVersion:   CurrentPluginAPIVersion,
				Runtime:      RuntimeExternalHTTP,
				Archive:      "archives/cycle-b.zip",
				Dependencies: []PluginDependency{{ID: "cycle-a"}},
			},
		},
	}
	rawIndex, _ := json.Marshal(index)
	indexPath := filepath.Join(root, "plugin-market.json")
	if err := os.WriteFile(indexPath, rawIndex, 0o644); err != nil {
		t.Fatalf("write market index: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(filepath.Join(root, "plugins"))

	market, err := m.FetchMarket(context.Background(), indexPath)
	if err != nil {
		t.Fatalf("fetch market: %v", err)
	}
	item := market.Plugins[0]
	if item.Installable || item.DependenciesSatisfied {
		t.Fatalf("expected cycle to block installability, got %#v", item)
	}
	if !strings.Contains(item.InstallReason, "dependency cycle") {
		t.Fatalf("expected cycle reason, got %#v", item)
	}
	if len(item.InstallPlan) != 0 {
		t.Fatalf("expected empty plan for cycle, got %#v", item.InstallPlan)
	}
	if len(item.DependenciesStatus) != 1 || item.DependenciesStatus[0].Satisfied || !strings.Contains(item.DependenciesStatus[0].Message, "dependency cycle") {
		t.Fatalf("expected dependency cycle status, got %#v", item.DependenciesStatus)
	}
}

func TestOptionalHostCapabilityStatus(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "plugins")
	dir := filepath.Join(pluginRoot, "optional-cap-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	manifest := Manifest{
		ID:                       "optional-cap-demo",
		Name:                     "Optional Capability Demo",
		Version:                  "1.0.0",
		APIVersion:               CurrentPluginAPIVersion,
		Runtime:                  RuntimeExternalHTTP,
		Entry:                    &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
		OptionalHostCapabilities: []string{"host.api.notice.write", "host.future.custom"},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(pluginRoot)
	if err := m.LoadExternal(pluginRoot); err != nil {
		t.Fatalf("load external: %v", err)
	}
	view, err := m.Get("optional-cap-demo")
	if err != nil {
		t.Fatalf("get optional capability plugin: %v", err)
	}
	if len(view.AvailableOptionalHostCapabilities) != 1 || view.AvailableOptionalHostCapabilities[0] != "host.api.notice.write" {
		t.Fatalf("expected available optional capability, got %#v", view.AvailableOptionalHostCapabilities)
	}
	if len(view.MissingOptionalHostCapabilities) != 1 || view.MissingOptionalHostCapabilities[0] != "host.future.custom" {
		t.Fatalf("expected missing optional capability, got %#v", view.MissingOptionalHostCapabilities)
	}
}

func TestManagerDevWatcherTracksExternalPluginLifecycle(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "plugins")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatalf("mkdir plugin root: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(pluginRoot)
	if err := m.LoadExternal(pluginRoot); err != nil {
		t.Fatalf("load external: %v", err)
	}
	m.EnableDevWatcher(100 * time.Millisecond)
	defer m.Stop()

	manifest := Manifest{
		ID:      "watch-demo",
		Name:    "Watch Demo",
		Version: "1.0.0",
		Runtime: RuntimeExternalHTTP,
		Entry:   &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
	}
	dir := filepath.Join(pluginRoot, manifest.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.py"), []byte("print('watch-v1')\n"), 0o644); err != nil {
		t.Fatalf("write plugin.py: %v", err)
	}

	waitForPluginState(t, 3*time.Second, func() bool {
		view, err := m.Get(manifest.ID)
		return err == nil && view.Version == "1.0.0"
	}, "watcher should auto-load newly created plugin")

	time.Sleep(200 * time.Millisecond)
	manifest.Version = "2.0.0"
	raw, _ = json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0o644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.py"), []byte("print('watch-v2')\n"), 0o644); err != nil {
		t.Fatalf("rewrite plugin.py: %v", err)
	}

	waitForPluginState(t, 3*time.Second, func() bool {
		view, err := m.Get(manifest.ID)
		return err == nil && view.Version == "2.0.0"
	}, "watcher should auto-reload modified plugin")

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove plugin dir: %v", err)
	}
	waitForPluginState(t, 3*time.Second, func() bool {
		_, err := m.Get(manifest.ID)
		return err != nil && strings.Contains(err.Error(), "not found")
	}, "watcher should unload deleted plugin")
}

func waitForPluginState(t *testing.T, timeout time.Duration, check func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal(message)
}

func makePluginZip(t *testing.T, manifest Manifest, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	raw, _ := json.Marshal(manifest)
	w, err := zw.Create("plugin.json")
	if err != nil {
		t.Fatalf("zip create manifest: %v", err)
	}
	if _, err := w.Write(raw); err != nil {
		t.Fatalf("zip write manifest: %v", err)
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
