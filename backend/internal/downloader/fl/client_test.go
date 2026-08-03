package fl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// flFake 记录最后一次请求；failPath 命中时该路径返回 500（错误路径测试用）。
// seqOn 控制 GET /api/torrents 返回的 isSequential（ToggleSequential 双向测试用）。
type flFake struct {
	mu       sync.Mutex
	path     string
	method   string
	body     string
	failPath string
	seqOn    bool
}

func (f *flFake) record(r *http.Request, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.path = r.URL.Path
	f.method = r.Method
	f.body = body
}

func (f *flFake) snapshot() (string, string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.path, f.method, f.body
}

func sampleTorrent() map[string]any {
	return map[string]any{
		"hash": "ABC123", "name": "Ubuntu", "bytesDone": 1000, "sizeBytes": 2000,
		"percentComplete": 50, "ratio": 1.5, "downRate": 200, "upRate": 100,
		"downTotal": 1000, "upTotal": 500, "directory": "/data", "dateAdded": 1700000000,
		"isComplete": false, "status": []string{"downloading"}, "tags": []string{"iso"},
		"seedsConnected": 3, "peersConnected": 4, "seedsTotal": 10, "peersTotal": 20, "priority": 2,
	}
}

// registerFLMocks 注册全套 jesec 端点 mock；failPath 命中的数据路径返回 500。
// 鉴权路径（/auth/*、/api/auth/*）不受 failPath 影响，保证错误路径测试仍能登录。
func registerFLMocks(mux *http.ServeMux, f *flFake) {
	mux.HandleFunc("/auth/verify", func(w http.ResponseWriter, r *http.Request) {
		// jesec 探测：404 + 非空 body。detectAPI 须读完 body 才能让 keep-alive 连接复用。
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})
	mux.HandleFunc("/api/auth/authenticate", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		http.SetCookie(w, &http.Cookie{Name: "jwt", Value: "tok"})
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	mux.HandleFunc("/api/client/connection-test", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"isConnected":true}`))
	})

	// 客户端全局设置：GET 返回限速，PATCH 记录
	mux.HandleFunc("/api/client/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == f.failPath {
			w.WriteHeader(500)
			return
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"throttleGlobalDownSpeed":1000000,"throttleGlobalUpSpeed":2000000,"directoryDefault":"/data"}`))
			return
		}
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		w.WriteHeader(200)
	})

	// 种子列表
	mux.HandleFunc("/api/torrents", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == f.failPath {
			w.WriteHeader(500)
			return
		}
		t := sampleTorrent()
		if f.seqOn {
			t["isSequential"] = true
		}
		f.record(r, "")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "torrents": map[string]any{"ABC123": t},
		})
	})

	// 单 hash 端点（hash 一律大写 ABC123）
	mux.HandleFunc("/api/torrents/ABC123/contents", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == f.failPath {
			w.WriteHeader(500)
			return
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[
				{"index":0,"path":"movie/a.mkv","filename":"a.mkv","sizeBytes":1000,"percentComplete":100,"priority":1},
				{"index":1,"path":"movie/b.mkv","filename":"b.mkv","sizeBytes":2000,"percentComplete":0,"priority":0}
			]`))
			return
		}
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/torrents/ABC123/peers", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == f.failPath {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(`[
			{"address":"1.2.3.4:6881","clientVersion":"qBittorrent/4.5.2","completedPercent":50,"downloadRate":100,"uploadRate":200,"isEncrypted":true,"isIncoming":false},
			{"address":"5.6.7.8:6881","clientVersion":"Transmission/3.0","completedPercent":100,"downloadRate":0,"uploadRate":50,"isEncrypted":false,"isIncoming":true}
		]`))
	})
	mux.HandleFunc("/api/torrents/ABC123/trackers", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == f.failPath {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(`[
			{"url":"http://tracker.one/announce","type":1},
			{"url":"udp://tracker.two:1337","type":2}
		]`))
	})

	// jesec 写端点（POST/PATCH，无 hash 段）
	for _, p := range []struct{ path, key string }{
		{"/api/torrents/add-urls", "add-urls"},
		{"/api/torrents/add-files", "add-files"},
		{"/api/torrents/start", "start"},
		{"/api/torrents/stop", "stop"},
		{"/api/torrents/delete", "delete"},
		{"/api/torrents/check-hash", "check-hash"},
		{"/api/torrents/reannounce", "reannounce"},
		{"/api/torrents/priority", "priority"},
		{"/api/torrents/tags", "tags"},
		{"/api/torrents/sequential", "sequential"},
		{"/api/torrents/trackers", "set-trackers"},
		{"/api/torrents/move", "move"},
	} {
		p := p
		mux.HandleFunc(p.path, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == f.failPath {
				w.WriteHeader(500)
				return
			}
			b, _ := io.ReadAll(r.Body)
			f.record(r, string(b))
			w.WriteHeader(200)
		})
	}
}

func newFLClient(t *testing.T) (*Client, *flFake) {
	t.Helper()
	f := &flFake{}
	mux := http.NewServeMux()
	registerFLMocks(mux, f)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

// newFLClientFailing 构造一个 failPath 命中即返回 500 的客户端（错误路径测试用）。
func newFLClientFailing(t *testing.T, failPath string) (*Client, *flFake) {
	t.Helper()
	f := &flFake{failPath: failPath}
	mux := http.NewServeMux()
	registerFLMocks(mux, f)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func newLegacyFLClient(t *testing.T) (*Client, *flFake) {
	t.Helper()
	f := &flFake{}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/verify", func(w http.ResponseWriter, r *http.Request) {
		// 写一段 body 模拟 legacy 401 响应；detectAPI 必须读完才能复用连接。
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"unauthorized","reason":"session expired"}`))
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc("/auth/authenticate", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "jwt", Value: "tok"})
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	mux.HandleFunc("/api/client/connection-test", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"isConnect":true}`))
	})
	mux.HandleFunc("/api/activity-stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		payload, _ := json.Marshal(map[string]any{"ABC123": sampleTorrent()})
		_, _ = w.Write([]byte("event: TORRENT_LIST_FULL_UPDATE\n"))
		_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
	})
	mux.HandleFunc("/api/client/add-files", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.record(r, r.FormValue("destination"))
		if _, _, err := r.FormFile("torrents"); err != nil {
			t.Errorf("legacy torrent file: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

// —— 基础用例 ——

func TestFLConnectionAndList(t *testing.T) {
	c, _ := newFLClient(t)
	ctx := context.Background()
	if err := c.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	list, err := c.ListTorrents(ctx)
	if err != nil {
		t.Fatalf("ListTorrents: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1, got %d", len(list))
	}
	tt := list[0]
	if tt.Name != "Ubuntu" || tt.Hash != "abc123" {
		t.Errorf("name/hash: %s/%s", tt.Name, tt.Hash)
	}
	if tt.Progress != 0.5 {
		t.Errorf("progress: %v", tt.Progress)
	}
	if tt.State != "downloading" {
		t.Errorf("state: %s", tt.State)
	}
	if tt.Category != "iso" {
		t.Errorf("category: %s", tt.Category)
	}
	if tt.UPRate != 100 || tt.DLRate != 200 {
		t.Errorf("rates: %d/%d", tt.UPRate, tt.DLRate)
	}
}

func TestFLLegacyConnectionListAndFileAdd(t *testing.T) {
	c, f := newLegacyFLClient(t)
	ctx := context.Background()
	if err := c.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	list, err := c.ListTorrents(ctx)
	if err != nil {
		t.Fatalf("ListTorrents: %v", err)
	}
	if len(list) != 1 || list[0].Hash != "abc123" {
		t.Fatalf("legacy list: %+v", list)
	}
	if err := c.AddTorrent(ctx, downloader.AddOptions{
		FileName: "source.torrent", FileContent: []byte("torrent"), SavePath: "/data",
	}); err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	if f.path != "/api/client/add-files" || f.body != "/data" {
		t.Errorf("legacy add: path=%s destination=%s", f.path, f.body)
	}
}

func TestFLCoreOps(t *testing.T) {
	c, f := newFLClient(t)
	ctx := context.Background()
	if err := c.AddTorrent(ctx, downloader.AddOptions{URL: "magnet:?xt=urn:btih:aa", Category: "tv"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.path, "add-urls") || !strings.Contains(f.body, "magnet:") {
		t.Errorf("add: path=%s body=%s", f.path, f.body)
	}
	if err := c.Pause(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.path, "stop") {
		t.Errorf("pause path: %s", f.path)
	}
	if err := c.Resume(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.path, "start") {
		t.Errorf("resume path: %s", f.path)
	}
	if err := c.Delete(ctx, []string{"abc123"}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.path, "delete") || !strings.Contains(f.body, "deleteData") {
		t.Errorf("delete: path=%s body=%s", f.path, f.body)
	}
}

func TestFLAdvanced(t *testing.T) {
	c, f := newFLClient(t)
	ctx := context.Background()
	if err := c.Recheck(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.path, "check-hash") {
		t.Errorf("recheck: %s", f.path)
	}
	if err := c.SetQueuePriority(ctx, []string{"abc123"}, "top"); err != nil {
		t.Fatal(err)
	}
	_, method, body := f.snapshot()
	if !strings.Contains(method, http.MethodPatch) || !strings.Contains(body, `"priority":3`) {
		t.Errorf("prio: method=%s body=%s", method, body)
	}
	if err := c.SetQueuePriority(ctx, []string{"abc123"}, "bogus"); err == nil {
		t.Error("invalid queue should error")
	}
	if err := c.SetTags(ctx, []string{"abc123"}, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if _, m, _ := f.snapshot(); m != http.MethodPatch {
		t.Errorf("tags method: %s", m)
	}
}

// —— 新增方法的用例 ——

func TestFLSetLocation(t *testing.T) {
	c, f := newFLClient(t)
	ctx := context.Background()
	if err := c.SetLocation(ctx, []string{"abc123"}, "/new/path"); err != nil {
		t.Fatal(err)
	}
	_, _, body := f.snapshot()
	// jesec move schema 要求三字段齐备
	for _, want := range []string{`"moveFiles":true`, `"isBasePath":false`, `"isCheckHash":false`, "/new/path"} {
		if !strings.Contains(body, want) {
			t.Errorf("move body missing %s: %s", want, body)
		}
	}
}

func TestFLTagsAddRemove(t *testing.T) {
	ctx := context.Background()

	// AddTags：原 tag=["iso"]，追加 "tv" → SetTags 收到 ["iso","tv"]
	c, f := newFLClient(t)
	if err := c.AddTags(ctx, []string{"abc123"}, []string{"tv"}); err != nil {
		t.Fatal(err)
	}
	_, _, body := f.snapshot()
	if !strings.Contains(body, "iso") || !strings.Contains(body, "tv") {
		t.Errorf("addTags body: %s", body)
	}

	// RemoveTags：原 tag=["iso"]，移除 "iso" → SetTags 收到空列表
	if err := c.RemoveTags(ctx, []string{"abc123"}, []string{"iso"}); err != nil {
		t.Fatal(err)
	}
	_, _, body = f.snapshot()
	if !strings.Contains(body, `"tags":[]`) {
		t.Errorf("removeTags body should have empty tags: %s", body)
	}
}

func TestFLSequentialToggle(t *testing.T) {
	ctx := context.Background()

	// 默认 isSequential=false → 翻转为开（PATCH isSequential:true）
	c, f := newFLClient(t)
	if err := c.ToggleSequentialDownload(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	path, method, body := f.snapshot()
	if path != "/api/torrents/sequential" || method != http.MethodPatch {
		t.Errorf("sequential off→on: path=%s method=%s", path, method)
	}
	if !strings.Contains(body, `"isSequential":true`) {
		t.Errorf("sequential body: %s", body)
	}

	// isSequential=true → 翻转为关（PATCH isSequential:false）
	f.seqOn = true
	if err := c.ToggleSequentialDownload(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	if _, _, body := f.snapshot(); !strings.Contains(body, `"isSequential":false`) {
		t.Errorf("sequential on→off body: %s", body)
	}
}

func TestFLSpeedLimits(t *testing.T) {
	c, f := newFLClient(t)
	ctx := context.Background()

	lim, err := c.GetSpeedLimits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lim.DownloadLimit != 1000000 || lim.UploadLimit != 2000000 {
		t.Errorf("limits: %+v", lim)
	}

	if err := c.SetSpeedLimits(ctx, downloader.Limits{UploadLimit: 500000, DownloadLimit: 300000}); err != nil {
		t.Fatal(err)
	}
	path, method, body := f.snapshot()
	if path != "/api/client/settings" || method != http.MethodPatch {
		t.Errorf("setSpeedLimits: path=%s method=%s", path, method)
	}
	if !strings.Contains(body, "throttleGlobalDownSpeed") || !strings.Contains(body, "300000") {
		t.Errorf("setSpeedLimits body: %s", body)
	}
}

func TestFLTrackers(t *testing.T) {
	ctx := context.Background()
	c, f := newFLClient(t)

	// ListTrackers：jesec 字段是 type 不是 tier
	tr, err := c.ListTrackers(ctx, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(tr) != 2 {
		t.Fatalf("want 2 trackers, got %d", len(tr))
	}
	if tr[0].Status != "http" || tr[1].Status != "udp" {
		t.Errorf("tracker status: %s / %s", tr[0].Status, tr[1].Status)
	}

	// AddTrackers：读当前 2 个 → 并入第 3 个 → SET 收到 3 个
	if err := c.AddTrackers(ctx, "abc123", []string{"https://tracker.three/announce"}); err != nil {
		t.Fatal(err)
	}
	_, _, body := f.snapshot()
	if !strings.Contains(body, "tracker.three") || !strings.Contains(body, "tracker.one") {
		t.Errorf("addTrackers body: %s", body)
	}

	// RemoveTrackers：读当前 2 个 → 减去 one → SET 仅剩 two
	if err := c.RemoveTrackers(ctx, "abc123", []string{"http://tracker.one/announce"}); err != nil {
		t.Fatal(err)
	}
	_, _, body = f.snapshot()
	if !strings.Contains(body, "tracker.two") || strings.Contains(body, "tracker.one") {
		t.Errorf("removeTrackers body: %s", body)
	}
}

func TestFLFiles(t *testing.T) {
	c, _ := newFLClient(t)
	ctx := context.Background()
	files, err := c.ListFiles(ctx, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d", len(files))
	}
	if files[0].Name != "movie/a.mkv" || files[0].Progress != 1.0 {
		t.Errorf("file[0]: name=%s prog=%v", files[0].Name, files[0].Progress)
	}
	if files[1].Progress != 0 {
		t.Errorf("file[1] prog: %v", files[1].Progress)
	}
}

func TestFLPeers(t *testing.T) {
	c, _ := newFLClient(t)
	ctx := context.Background()
	peers, err := c.ListPeers(ctx, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 {
		t.Fatalf("want 2 peers, got %d", len(peers))
	}
	if peers[0].IP != "1.2.3.4:6881" || peers[0].Progress != 0.5 {
		t.Errorf("peer[0]: ip=%s prog=%v", peers[0].IP, peers[0].Progress)
	}
	if peers[0].Flags != "E" {
		t.Errorf("peer[0] flags: %s (want E for encrypted)", peers[0].Flags)
	}
	if peers[1].Flags != "I" {
		t.Errorf("peer[1] flags: %s (want I for incoming)", peers[1].Flags)
	}
}

func TestFLSetFilePriority(t *testing.T) {
	c, f := newFLClient(t)
	ctx := context.Background()
	if err := c.SetFilePriority(ctx, "abc123", []int{0, 1}, 2); err != nil {
		t.Fatal(err)
	}
	path, method, body := f.snapshot()
	if !strings.Contains(path, "contents") || method != http.MethodPatch {
		t.Errorf("filePrio: path=%s method=%s", path, method)
	}
	if !strings.Contains(body, `"priority":2`) || !strings.Contains(body, `"indices":[0,1]`) {
		t.Errorf("filePrio body: %s", body)
	}
}

// —— 错误路径用例 ——

func TestFLNewMethodErrors(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		failPath string
		fn       func(c *Client) error
	}{
		{
			"RemoveTags", "/api/torrents/tags",
			func(c *Client) error { return c.RemoveTags(ctx, []string{"abc123"}, []string{"iso"}) },
		},
		{
			"AddTags", "/api/torrents/tags",
			func(c *Client) error { return c.AddTags(ctx, []string{"abc123"}, []string{"tv"}) },
		},
		{
			"ToggleSequential", "/api/torrents/sequential",
			func(c *Client) error { return c.ToggleSequentialDownload(ctx, []string{"abc123"}) },
		},
		{
			"GetSpeedLimits", "/api/client/settings",
			func(c *Client) error { _, err := c.GetSpeedLimits(ctx); return err },
		},
		{
			"SetSpeedLimits", "/api/client/settings",
			func(c *Client) error { return c.SetSpeedLimits(ctx, downloader.Limits{UploadLimit: 1}) },
		},
		{
			"AddTrackers", "/api/torrents/trackers",
			func(c *Client) error { return c.AddTrackers(ctx, "abc123", []string{"https://x/y"}) },
		},
		{
			"RemoveTrackers", "/api/torrents/trackers",
			func(c *Client) error { return c.RemoveTrackers(ctx, "abc123", []string{"http://tracker.one/announce"}) },
		},
		{
			"ListFiles", "/api/torrents/ABC123/contents",
			func(c *Client) error { _, err := c.ListFiles(ctx, "abc123"); return err },
		},
		{
			"ListPeers", "/api/torrents/ABC123/peers",
			func(c *Client) error { _, err := c.ListPeers(ctx, "abc123"); return err },
		},
		{
			"ListTrackers", "/api/torrents/ABC123/trackers",
			func(c *Client) error { _, err := c.ListTrackers(ctx, "abc123"); return err },
		},
		{
			"SetLocation", "/api/torrents/move",
			func(c *Client) error { return c.SetLocation(ctx, []string{"abc123"}, "/p") },
		},
		{
			"SetFilePriority", "/api/torrents/ABC123/contents",
			func(c *Client) error { return c.SetFilePriority(ctx, "abc123", []int{0}, 1) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newFLClientFailing(t, tc.failPath)
			if err := tc.fn(c); err == nil {
				t.Errorf("%s: want error when %s returns 500", tc.name, tc.failPath)
			}
		})
	}
}

// —— 仍未实现的特性（保留 ErrNotImplemented）——

func TestFLNotImplemented(t *testing.T) {
	c, _ := newFLClient(t)
	ctx := context.Background()
	checks := map[string]func() error{
		"SetForceStart":    func() error { return c.SetForceStart(ctx, nil, true) },
		"CreateCategory":   func() error { return c.CreateCategory(ctx, "n", "") },
		"EditCategory":     func() error { return c.EditCategory(ctx, "n", "") },
		"DeleteCategories": func() error { return c.DeleteCategories(ctx, nil) },
		"SetShareLimits":   func() error { return c.SetShareLimits(ctx, nil, downloader.ShareLimits{}) },
		"ToggleFirstLast":  func() error { return c.ToggleFirstLastPiece(ctx, nil) },
		"SetLimits":        func() error { return c.SetLimits(ctx, nil, downloader.Limits{}) },
	}
	for name, fn := range checks {
		if err := fn(); err != downloader.ErrNotImplemented {
			t.Errorf("%s: want ErrNotImplemented, got %v", name, err)
		}
	}
}
