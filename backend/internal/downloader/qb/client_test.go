package qb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

func TestQBClientEndToEnd(t *testing.T) {
	var (
		lastPath   string
		lastForm   string
		loginCount int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		switch r.URL.Path {
		case "/api/v2/auth/login":
			loginCount++
			w.WriteHeader(200)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.0.0"))
		case "/api/v2/sync/maindata":
			if r.URL.Query().Get("rid") == "" || r.URL.Query().Get("rid") == "0" {
				_, _ = w.Write([]byte(`{
					"rid":1,"full_update":true,
					"torrents":{"a":{"name":"A","size":100,"progress":0.5,"state":"uploadingUP","ratio":2.0,"dlspeed":0,"upspeed":1024,"added_on":1700000000}},
					"server_state":{"dl_info_speed":0,"up_info_speed":2048},
					"tags":["seed"]
				}`))
			} else {
				_, _ = w.Write([]byte(`{"rid":2,"torrents":{"a":{"ratio":3.0}}}`))
			}
		case "/api/v2/torrents/add":
			_ = r.ParseForm()
			lastForm = r.PostForm.Encode()
			w.WriteHeader(200)
		case "/api/v2/torrents/stop", "/api/v2/torrents/start":
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}

	// TestConnection → login + version(5.0.0)
	if err := c.TestConnection(context.Background()); err != nil {
		t.Fatalf("test conn: %v", err)
	}
	if !c.ge5() {
		t.Error("expected ge5=true for v5.0.0")
	}

	// ListTorrents → 全量同步
	ts, err := c.ListTorrents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 || ts[0].Name != "A" || ts[0].UPRate != 1024 {
		t.Errorf("list = %+v", ts)
	}

	// GlobalStats
	st, err := c.GlobalStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.UPRate != 2048 {
		t.Errorf("uprate = %d", st.UPRate)
	}

	// Add（版本>=5 → stopped 字段）
	lastForm = ""
	if err := c.AddTorrent(context.Background(),
		downloader.AddOptions{URL: "magnet:?xt=urn:btih:abc", Paused: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lastForm, "stopped=true") {
		t.Errorf("expected stopped=true for qB5, form=%q", lastForm)
	}
	if !strings.Contains(lastForm, "magnet") {
		t.Errorf("url not posted: %q", lastForm)
	}

	// Pause（版本>=5 → /stop）
	if err := c.Pause(context.Background(), []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if lastPath != "/api/v2/torrents/stop" {
		t.Errorf("expected /torrents/stop for qB5, got %s", lastPath)
	}

	if loginCount < 1 {
		t.Errorf("login not called")
	}
}

// TestQBClientRetriesOn401 验证 401 也触发重登重试（不只 403）——反向代理等场景会返 401。
func TestQBClientRetriesOn401(t *testing.T) {
	var loginCount, verCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			loginCount++
			w.WriteHeader(200)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			verCalls++
			if verCalls == 1 {
				w.WriteHeader(401) // 首次未授权
				return
			}
			_, _ = w.Write([]byte("v5.0.0")) // 重登后成功
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.TestConnection(context.Background()); err != nil {
		t.Fatalf("expected 401 self-heal, got %v", err)
	}
	if verCalls != 2 {
		t.Errorf("version calls=%d, want 2 (401 then 200)", verCalls)
	}
	if loginCount < 2 {
		t.Errorf("loginCount=%d, want >=2 (re-login after 401)", loginCount)
	}
}

// TestQBClientRidRegressionResync 验证 qB 重启 rid 回退时自动全量重拉，而非卡死在旧 rid。
func TestQBClientRidRegressionResync(t *testing.T) {
	var syncCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(200)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			syncCalls++
			if r.URL.Query().Get("rid") == "100" {
				// qB 重启后 rid 计数回退、却返非 full 的增量（会触发丢弃路径）
				_, _ = w.Write([]byte(`{"rid":2,"full_update":false,"torrents":{"x":{"name":"partial"}}}`))
				return
			}
			// rid=0 全量重拉
			_, _ = w.Write([]byte(`{"rid":2,"full_update":true,"torrents":{"a":{"name":"FullA","size":10}},"server_state":{},"tags":[]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	// 模拟 qB 重启前已同步到高 rid。
	c.stateMu.Lock()
	c.state.Rid = 100
	c.state.Torrents["stale"] = map[string]any{"name": "stale"}
	c.stateMu.Unlock()

	if err := c.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if syncCalls != 2 {
		t.Fatalf("syncCalls=%d, want 2 (regression → full resync)", syncCalls)
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.state.Rid != 2 {
		t.Errorf("Rid=%d, want 2 (full resync)", c.state.Rid)
	}
	if _, ok := c.state.Torrents["stale"]; ok {
		t.Error("stale torrent should be replaced by full resync")
	}
	if _, ok := c.state.Torrents["a"]; !ok {
		t.Error("full-resync torrent 'a' missing")
	}
}

// TestQBAddFailsBodyReturnsError qB 添加失败是 HTTP 200 + "Fails."：只查状态码会把失败
// 当成功（RSS 标记已下载、辅种记 added，静默丢种且不再重试）。URL 与文件两条路径都要报错。
func TestQBAddFailsBodyReturnsError(t *testing.T) {
	fails := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(200)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			w.WriteHeader(200)
			if fails {
				_, _ = w.Write([]byte("Fails."))
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c, err := New(srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.AddTorrent(ctx, downloader.AddOptions{URL: "magnet:?xt=urn:btih:x"}); err == nil {
		t.Error("Fails. 响应必须报错（URL 路径）")
	}
	if err := c.AddTorrent(ctx, downloader.AddOptions{FileContent: []byte("d4:infode"), FileName: "a.torrent"}); err == nil {
		t.Error("Fails. 响应必须报错（文件路径）")
	}
	// 正控：空 body = qB 成功响应 → 不得报错。
	fails = false
	if err := c.AddTorrent(ctx, downloader.AddOptions{URL: "magnet:?xt=urn:btih:y"}); err != nil {
		t.Errorf("成功响应（空 body）不应报错: %v", err)
	}
}
