package dl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// dlFake 记录最后一次 /json 请求的 method 与 params。
type dlFake struct {
	mu     sync.Mutex
	method string
	params json.RawMessage
}

func (f *dlFake) record(method string, params json.RawMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.method = method
	f.params = params
}

// rpcResp 包装 Deluge JSON-RPC 响应（error:null + result）。
func rpcResp(result string) string {
	return `{"id":1,"error":null,"result":` + result + `}`
}

// newDLClient 启动 mock Deluge /json（密码 "secret"），返回 client + 记录器。
func newDLClient(t *testing.T) (*Client, *dlFake) {
	t.Helper()
	f := &dlFake{}
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		f.record(req.Method, req.Params)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "auth.login":
			var p []string
			_ = json.Unmarshal(req.Params, &p)
			if len(p) > 0 && p[0] == "secret" {
				http.SetCookie(w, &http.Cookie{Name: "_session", Value: "s"})
				_, _ = w.Write([]byte(rpcResp("true")))
			} else {
				_, _ = w.Write([]byte(rpcResp("false")))
			}
		case "web.connected":
			_, _ = w.Write([]byte(rpcResp("true")))
		case "core.get_torrents_status":
			torrent := `{"H1":{"hash":"H1","name":"Test","progress":50,"ratio":1.5,"time_added":1700000000,"completed_time":1700000500,"save_path":"/d","label":"cat","state":"Downloading","total_size":2048,"upload_payload_rate":100,"download_payload_rate":200,"total_uploaded":500,"total_done":1000,"num_seeds":3,"num_peers":4,"total_seeds":10,"total_peers":20}}`
			_, _ = w.Write([]byte(rpcResp(torrent)))
		case "core.get_session_status":
			_, _ = w.Write([]byte(rpcResp(`{"upload_rate":100,"download_rate":200,"total_upload":1,"total_download":2}`)))
		case "core.add_torrent_url", "core.add_torrent_file":
			_, _ = w.Write([]byte(rpcResp(`[true,"NEWHASH"]`)))
		case "core.get_config":
			_, _ = w.Write([]byte(rpcResp(`{"max_upload_speed":100,"max_download_speed":200}`)))
		case "core.get_torrent_status":
			res := `{"files":[{"path":"a.mkv","size":1024}],"file_progress":[0.5],"file_priorities":[1],"trackers":[{"url":"http://t","tier":0}],"peers":[["1.2.3.4",6881,"ClientX"]]}`
			_, _ = w.Write([]byte(rpcResp(res)))
		default:
			_, _ = w.Write([]byte(rpcResp("null")))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "", "secret")
	return c, f
}

func TestDLConnectionAndList(t *testing.T) {
	c, _ := newDLClient(t)
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
	if tt.Name != "Test" || tt.Progress != 0.5 || tt.State != "downloading" {
		t.Errorf("torrent: %+v", tt)
	}
	if tt.UPRate != 100 || tt.DLRate != 200 {
		t.Errorf("rates: up=%d dl=%d", tt.UPRate, tt.DLRate)
	}
	if tt.Category != "cat" || tt.NumComplete != 10 {
		t.Errorf("cat=%s complete=%d", tt.Category, tt.NumComplete)
	}
	// CompletedAt 由 completed_time 映射
	if !tt.CompletedAt.Equal(time.Unix(1700000500, 0)) {
		t.Errorf("CompletedAt: %v want 1700000500", tt.CompletedAt)
	}
}

func TestDLAddSetsLabel(t *testing.T) {
	c, f := newDLClient(t)
	err := c.AddTorrent(context.Background(), downloader.AddOptions{
		URL: "magnet:?xt=urn:btih:abcdef", Category: "影视",
	})
	if err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	// add-url 后用返回 hash 调 label.set_torrent
	if f.method != "label.set_torrent" {
		t.Errorf("last method: %s", f.method)
	}
}

func TestDLOps(t *testing.T) {
	c, f := newDLClient(t)
	ctx := context.Background()
	if err := c.Pause(ctx, []string{"h1"}); err != nil {
		t.Fatal(err)
	}
	if f.method != "core.pause_torrent" {
		t.Errorf("pause: %s", f.method)
	}
	if err := c.Resume(ctx, []string{"h1"}); err != nil {
		t.Fatal(err)
	}
	if f.method != "core.resume_torrent" {
		t.Errorf("resume: %s", f.method)
	}
	if err := c.Delete(ctx, []string{"h1", "h2"}, true); err != nil {
		t.Fatal(err)
	}
	if f.method != "core.remove_torrent" {
		t.Errorf("delete: %s", f.method)
	}
	if err := c.SetCategory(ctx, []string{"h1"}, "tag"); err != nil {
		t.Fatal(err)
	}
	if f.method != "label.set_torrent" {
		t.Errorf("SetCategory: %s", f.method)
	}
}

func TestDLStats(t *testing.T) {
	c, _ := newDLClient(t)
	s, err := c.GlobalStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.UPRate != 100 || s.DLRate != 200 {
		t.Errorf("stats: %+v", s)
	}
}

func TestDLBadPassword(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(rpcResp("false"))) // auth.login 恒 false
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "", "wrong")
	if err := c.TestConnection(context.Background()); err != downloader.ErrInvalidCredentials {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

// TestDLAddFailureReturnsError Deluge add 失败返回 [false,"原因"]：此前只取 hash 不校验
// ok，失败被当成功（RSS 标记已下载、辅种记 added）。
func TestDLAddFailureReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "auth.login":
			http.SetCookie(w, &http.Cookie{Name: "_session", Value: "s"})
			_, _ = w.Write([]byte(rpcResp("true")))
		case "web.connected":
			_, _ = w.Write([]byte(rpcResp("true")))
		case "core.add_torrent_url":
			_, _ = w.Write([]byte(rpcResp(`[false,"Duplicate torrent"]`)))
		default:
			_, _ = w.Write([]byte(rpcResp("null")))
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "", "secret")
	err := c.AddTorrent(context.Background(), downloader.AddOptions{URL: "magnet:?xt=urn:btih:x"})
	if err == nil {
		t.Fatal("add 失败必须报错")
	}
	if !strings.Contains(err.Error(), "Duplicate torrent") {
		t.Errorf("错误应含 Deluge 原因: %v", err)
	}
}
