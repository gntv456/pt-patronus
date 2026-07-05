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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
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
	if err := db.AutoMigrate(&models.PluginState{}, &models.PluginLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestManagerHandlesEnabledPluginEvent(t *testing.T) {
	db := newPluginTestDB(t)

	bus := event.New()
	m := NewManager(db, bus, nil)
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
	m := NewManager(db, bus, nil)
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
	bus.Publish(context.Background(), "plugin.test", map[string]any{"message": "ok"})
	if !eventHit {
		t.Fatal("expected external plugin event endpoint to be called")
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
		Runtime:     RuntimeExternalHTTP,
		Entry:       &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
		Permissions: []string{"event:read"},
	}
	zipBytes := makePluginZip(t, manifest, map[string]string{"plugin.py": "print('ok')\n"})
	sum := sha256.Sum256(zipBytes)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sig := ed25519.Sign(priv, zipBytes)
	archivePath := filepath.Join(pkgDir, "market-demo.zip")
	if err := os.WriteFile(archivePath, zipBytes, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	index := MarketIndex{
		Version:   1,
		PublicKey: base64.StdEncoding.EncodeToString(pub),
		Plugins: []MarketPlugin{{
			ID:          manifest.ID,
			Name:        manifest.Name,
			Version:     manifest.Version,
			Runtime:     manifest.Runtime,
			Archive:     "packages/market-demo.zip",
			SHA256:      hex.EncodeToString(sum[:]),
			Signature:   base64.StdEncoding.EncodeToString(sig),
			Permissions: manifest.Permissions,
		}},
	}
	rawIndex, _ := json.Marshal(index)
	indexPath := filepath.Join(root, "plugin-market.json")
	if err := os.WriteFile(indexPath, rawIndex, 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil)
	m.SetPluginRoot(pluginRoot)
	res, err := m.InstallFromMarket(context.Background(), indexPath, "market-demo")
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
	if !market.Plugins[0].Installed {
		t.Fatal("expected installed flag after install")
	}
	if err := m.Uninstall("market-demo"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginRoot, "market-demo")); !os.IsNotExist(err) {
		t.Fatalf("expected plugin directory removed, err=%v", err)
	}
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
