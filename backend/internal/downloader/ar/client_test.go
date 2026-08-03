package ar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

type arFake struct {
	mu     sync.Mutex
	method string
	params json.RawMessage
	body   string
}

func (f *arFake) record(method string, params json.RawMessage, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.method = method
	f.params = params
	f.body = body
}

func sampleBTTask() string {
	return `{"gid":"GID1","status":"active","totalLength":"2000","completedLength":"1000","uploadLength":"400","downloadSpeed":"50","uploadSpeed":"100","dir":"/data","infoHash":"abc","numSeeders":"3","connections":"2","bittorrent":{"info":{"name":"Ubuntu"},"announceList":[["http://t"]]}}`
}

func newARClient(t *testing.T) (*Client, *arFake) {
	t.Helper()
	f := &arFake{}
	mux := http.NewServeMux()
	mux.HandleFunc("/jsonrpc", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(b, &req)
		f.record(req.Method, req.Params, string(b))
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "aria2.getVersion":
			_, _ = w.Write([]byte(`{"id":"1","jsonrpc":"2.0","result":{"version":"1.36.0"}}`))
		case "system.multicall":
			task := sampleBTTask()
			res := fmt.Sprintf(`{"id":"1","jsonrpc":"2.0","result":[[[%s]],[[]],[[]]]}`, task)
			_, _ = w.Write([]byte(res))
		case "aria2.getGlobalStat":
			_, _ = w.Write([]byte(`{"id":"1","jsonrpc":"2.0","result":{"downloadSpeed":"200","uploadSpeed":"100"}}`))
		case "aria2.getGlobalOption":
			_, _ = w.Write([]byte(`{"id":"1","jsonrpc":"2.0","result":{"max-overall-upload-limit":"102400","max-overall-download-limit":"204800"}}`))
		case "aria2.getFiles":
			_, _ = w.Write([]byte(`{"id":"1","jsonrpc":"2.0","result":[{"index":"1","path":"a.mkv","length":"1024","completedLength":"512","selected":"true"}]}`))
		case "aria2.getPeers":
			_, _ = w.Write([]byte(`{"id":"1","jsonrpc":"2.0","result":[{"ip":"1.2.3.4","port":"6881","peerId":"PeerX","downloadSpeed":"10","uploadSpeed":"5"}]}`))
		case "aria2.tellStatus":
			_, _ = w.Write([]byte(`{"id":"1","jsonrpc":"2.0","result":` + sampleBTTask() + `}`))
		case "aria2.removeDownloadResult":
			// 模拟 Aria2 实际错误：仅对未 removed 的 GID 调 removeDownloadResult 会返 fault。
			// 用于验证 Delete 现在会传播该错误而非吞掉。
			var p []any
			_ = json.Unmarshal(req.Params, &p)
			if len(p) >= 1 {
				if _, ok := p[len(p)-1].(string); ok {
					_, _ = w.Write([]byte(`{"id":"1","jsonrpc":"2.0","error":{"code":1,"message":"GID not found"}}`))
					return
				}
			}
			_, _ = w.Write([]byte(`{"id":"1","jsonrpc":"2.0","result":"ok"}`))
		default:
			_, _ = w.Write([]byte(`{"id":"1","jsonrpc":"2.0","result":"ok"}`))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return New(srv.URL, "", "secret"), f
}

func TestARConnectionAndList(t *testing.T) {
	c, f := newARClient(t)
	ctx := context.Background()
	if err := c.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if !strings.Contains(f.body, "token:secret") {
		t.Errorf("token not injected: %s", f.body)
	}
	list, err := c.ListTorrents(ctx)
	if err != nil {
		t.Fatalf("ListTorrents: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1, got %d", len(list))
	}
	tt := list[0]
	if tt.Hash != "GID1" || tt.InfoHash != "abc" || tt.Name != "Ubuntu" {
		t.Errorf("hash/infoHash/name: %s/%s/%s", tt.Hash, tt.InfoHash, tt.Name)
	}
	if tt.Progress != 0.5 {
		t.Errorf("progress: %v", tt.Progress)
	}
	if tt.State != "downloading" {
		t.Errorf("state: %s", tt.State)
	}
	if tt.UPRate != 100 || tt.DLRate != 50 {
		t.Errorf("rates: up=%d dl=%d", tt.UPRate, tt.DLRate)
	}
	st, err := c.GlobalStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.DLRate != 200 || st.UPRate != 100 {
		t.Errorf("stats: %+v", st)
	}
}

func TestARCoreOps(t *testing.T) {
	c, f := newARClient(t)
	ctx := context.Background()
	if err := c.AddTorrent(ctx, downloader.AddOptions{URL: "magnet:?xt=urn:btih:aa", SavePath: "/d", Paused: true}); err != nil {
		t.Fatal(err)
	}
	if f.method != "aria2.addUri" || !strings.Contains(f.body, "magnet:") {
		t.Errorf("addUri: method=%s body=%s", f.method, f.body)
	}
	if err := c.Pause(ctx, []string{"GID1"}); err != nil {
		t.Fatal(err)
	}
	if f.method != "aria2.pause" {
		t.Errorf("pause: %s", f.method)
	}
	if err := c.Resume(ctx, []string{"GID1"}); err != nil {
		t.Fatal(err)
	}
	if f.method != "aria2.unpause" {
		t.Errorf("resume: %s", f.method)
	}
	// Delete：remove 成功后 removeDownloadResult 也须调用且错误须传播（修复前用 _,_ 吞错）。
	// 此 mock 对 removeDownloadResult 返 fault，故 Delete 应报错。
	if err := c.Delete(ctx, []string{"GID1"}, false); err == nil {
		t.Fatal("Delete should propagate removeDownloadResult error, got nil")
	}
}

func TestARAdvanced(t *testing.T) {
	c, f := newARClient(t)
	ctx := context.Background()
	if err := c.SetQueuePriority(ctx, []string{"GID1"}, "top"); err != nil {
		t.Fatal(err)
	}
	if f.method != "aria2.changePosition" || !strings.Contains(f.body, "POS_SET") {
		t.Errorf("queue: method=%s body=%s", f.method, f.body)
	}
	if err := c.SetQueuePriority(ctx, []string{"GID1"}, "bogus"); err == nil {
		t.Error("invalid queue should error")
	}
	if err := c.SetLimits(ctx, []string{"GID1"}, downloader.Limits{UploadLimit: 100, DownloadLimit: 200}); err != nil {
		t.Fatal(err)
	}
	if f.method != "aria2.changeOption" {
		t.Errorf("limits: %s", f.method)
	}
	lim, err := c.GetSpeedLimits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lim.UploadLimit != 102400 || lim.DownloadLimit != 204800 {
		t.Errorf("speedlimits: %+v", lim)
	}
	files, err := c.ListFiles(ctx, "GID1")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "a.mkv" || files[0].Index != 0 {
		t.Errorf("files: %+v", files)
	}
	peers, err := c.ListPeers(ctx, "GID1")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].IP != "1.2.3.4" {
		t.Errorf("peers: %+v", peers)
	}
	tr, err := c.ListTrackers(ctx, "GID1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tr) != 1 || tr[0].URL != "http://t" {
		t.Errorf("trackers: %+v", tr)
	}
}

func TestARNotImplemented(t *testing.T) {
	c, _ := newARClient(t)
	ctx := context.Background()
	checks := map[string]func() error{
		"SetCategory":      func() error { return c.SetCategory(ctx, nil, "x") },
		"Recheck":          func() error { return c.Recheck(ctx, nil) },
		"Reannounce":       func() error { return c.Reannounce(ctx, nil) },
		"SetLocation":      func() error { return c.SetLocation(ctx, nil, "/x") },
		"SetForceStart":    func() error { return c.SetForceStart(ctx, nil, true) },
		"ListCategories":   func() error { _, e := c.ListCategories(ctx); return e },
		"ListTags":         func() error { _, e := c.ListTags(ctx); return e },
		"SetTags":          func() error { return c.SetTags(ctx, nil, nil) },
		"AddTags":          func() error { return c.AddTags(ctx, nil, nil) },
		"RemoveTags":       func() error { return c.RemoveTags(ctx, nil, nil) },
		"CreateCategory":   func() error { return c.CreateCategory(ctx, "n", "") },
		"EditCategory":     func() error { return c.EditCategory(ctx, "n", "") },
		"DeleteCategories": func() error { return c.DeleteCategories(ctx, nil) },
		"SetShareLimits":   func() error { return c.SetShareLimits(ctx, nil, downloader.ShareLimits{}) },
		"ToggleSequential": func() error { return c.ToggleSequentialDownload(ctx, nil) },
		"ToggleFirstLast":  func() error { return c.ToggleFirstLastPiece(ctx, nil) },
		"AddTrackers":      func() error { return c.AddTrackers(ctx, "h", nil) },
		"RemoveTrackers":   func() error { return c.RemoveTrackers(ctx, "h", nil) },
	}
	for name, fn := range checks {
		if err := fn(); err != downloader.ErrNotImplemented {
			t.Errorf("%s: want ErrNotImplemented, got %v", name, err)
		}
	}
}
