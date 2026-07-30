package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// TestParseMediaWebhook 覆盖 Emby/JF JSON、Plex JSON、Plex form-encoded、空载荷。
func TestParseMediaWebhook(t *testing.T) {
	// Emby/JF：顶层 Title + Event
	title, event := parseMediaWebhook([]byte(`{"Title":"Inception","Event":"play"}`))
	if title != "Inception" || event != "play" {
		t.Errorf("emby: title=%q event=%q", title, event)
	}

	// Plex JSON：Metadata.title + event
	title, event = parseMediaWebhook([]byte(`{"event":"media.scrobble","Metadata":{"title":"Inception"}}`))
	if title != "Inception" || event != "media.scrobble" {
		t.Errorf("plex json: title=%q event=%q", title, event)
	}

	// Plex form-encoded
	title, event = parseMediaWebhook([]byte(`event=media.play&metadata[title]=Foo+Bar`))
	if title != "Foo+Bar" || event != "media.play" {
		t.Errorf("plex form: title=%q event=%q", title, event)
	}

	// 空载荷
	title, event = parseMediaWebhook(nil)
	if title != "" || event != "" {
		t.Errorf("empty: title=%q event=%q want both empty", title, event)
	}

	// 缺 event 字段 → 归一为 "play"
	_, event = parseMediaWebhook([]byte(`{"Title":"X"}`))
	if event != "play" {
		t.Errorf("default event = %q want play", event)
	}
}

// newMediaServerTestChain 构建一条接内存 DB 的 MediaServerChain（不依赖真实 media server）。
func newMediaServerTestChain(t *testing.T) (*MediaServerChain, *oper.MediaServerOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ms.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.MediaServer{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	op := oper.NewMediaServerOper(db, security.NewCrypto("test-seed"))
	base := NewBase(module.New(), event.New())
	ch, err := NewMediaServerChain(base, op, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ch, op
}

// TestMediaServerCRUDOper oper 创建 + token 加密往返 + 类型白名单。
func TestMediaServerCRUDOper(t *testing.T) {
	_, op := newMediaServerTestChain(t)

	// 非法类型被拒
	if _, err := op.Create(oper.MediaServerInput{Name: "x", Type: "foo", URL: "http://x"}); err == nil {
		t.Fatal("非法类型应被拒")
	}

	m, err := op.Create(oper.MediaServerInput{
		Name: "emby-home", Type: "emby", URL: "http://10.0.0.1:8096",
		Token: "secret-key", RefreshOnDownload: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tok, err := op.Token(m)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "secret-key" {
		t.Errorf("token round-trip = %q", tok)
	}

	// Update token 空串=保留；显式保留 RefreshOnDownload=true（Update 覆盖全部字段）
	if _, err := op.Update(m.ID, oper.MediaServerInput{
		Name: "renamed", Type: "emby", URL: "http://10.0.0.1:8096", RefreshOnDownload: true,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	m2, _ := op.Get(m.ID)
	tok2, _ := op.Token(m2)
	if tok2 != "secret-key" || m2.Name != "renamed" {
		t.Errorf("update kept token? token=%q name=%q", tok2, m2.Name)
	}

	// ListRefreshOnDownload 只返回开启刷新的
	ms, err := op.ListRefreshOnDownload()
	if err != nil || len(ms) != 1 {
		t.Errorf("ListRefreshOnDownload = %v (want 1)", ms)
	}
}

// TestOnDownloadCompletedRefreshes 下载完成钩子触发对 mock media server 的整库扫描。
func TestOnDownloadCompletedRefreshes(t *testing.T) {
	scanned := make(map[string]bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Library/MediaFolders":
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{
				{"Name": "电影", "Id": "lib1"},
			}})
		default:
			if r.URL.Path == "/Items/lib1/Refresh" {
				scanned["lib1"] = true
			}
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	ch, op := newMediaServerTestChain(t)
	m, err := op.Create(oper.MediaServerInput{
		Name: "emby", Type: "emby", URL: srv.URL, Token: "t", RefreshOnDownload: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = m

	ch.OnDownloadCompleted(context.Background(), "Some.Movie.2024")
	if !scanned["lib1"] {
		t.Error("expected LibraryScan on lib1 after download completion")
	}
}
