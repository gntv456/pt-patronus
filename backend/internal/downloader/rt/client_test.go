package rt

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

type rtFake struct {
	mu   sync.Mutex
	path string
	body string
	mode string
	hash string
}

func (f *rtFake) record(r *http.Request, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.path = r.URL.Path
	f.body = body
	if strings.Contains(r.Header.Get("Content-Type"), "x-www-form-urlencoded") {
		_ = r.ParseForm()
		f.mode = r.Form.Get("mode")
		f.hash = r.Form.Get("hash")
	}
}

func sampleRow() []any {
	// 30+ fields per PT-depiler
	row := make([]any, 34)
	row[0] = "1"             // is_open
	row[1] = "0"             // is_hash_checking
	row[3] = "1"             // get_state
	row[4] = "Ubuntu"        // name
	row[5] = "2048"          // size
	row[6] = "500"           // completed chunks
	row[7] = "1000"          // size chunks → progress 0.5
	row[8] = "1000"          // downloaded
	row[9] = "500"           // uploaded
	row[10] = "1500"         // ratio *1000 → 1.5
	row[11] = "100"          // ul
	row[12] = "200"          // dl
	row[14] = "iso"          // label
	row[17] = "4"            // peers connected
	row[18] = "3"            // seeds
	row[20] = "2"            // priority
	row[21] = "1700000000"   // state_changed
	row[23] = "0"            // get_hashing
	row[25] = "/data/Ubuntu" // base_path
	row[28] = "1"            // is_active
	row[29] = ""             // msg
	return row
}

// speedLimitsXML 返回一个 system.multicall 响应（get_upload_rate / get_download_rate），
// 用于 GetSpeedLimits 的 mock：上传 512000、下载 1024000 bytes/s。
const speedLimitsXML = `<?xml version="1.0" encoding="UTF-8"?>
<methodResponse><params><param><value><array><data>
<value><array><data><value><string>512000</string></value></data></array></value>
<value><array><data><value><string>1024000</string></value></data></array></value>
</data></array></value></param></params></methodResponse>`

func newRTClient(t *testing.T) (*Client, *rtFake) {
	t.Helper()
	f := &rtFake{}
	mux := http.NewServeMux()
	mux.HandleFunc("/php/getsettings.php", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/plugins/httprpc/action.php", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		// re-inject body for form parse
		r.Body = io.NopCloser(strings.NewReader(string(b)))
		f.record(r, string(b))
		_ = r.ParseForm()
		mode := r.Form.Get("mode")
		w.Header().Set("Content-Type", "application/json")
		switch mode {
		case "list":
			resp := map[string]any{"t": map[string]any{"ABC123": sampleRow()}, "cid": 1}
			_ = json.NewEncoder(w).Encode(resp)
		case "fls":
			// [path, completed_chunks, size_chunks, size_bytes, priority]
			_ = json.NewEncoder(w).Encode([][]any{
				{"movie.mkv", "100", "200", "2048", "1"},
				{"subs/sub.srt", "0", "50", "512", "0"},
			})
		case "prs":
			// [id, address, client_version, is_incoming, is_encrypted, is_snubbed,
			//  completed_percent, down_total, up_total, down_rate, up_rate, id_html, peer_rate, peer_total, port]
			_ = json.NewEncoder(w).Encode([][]any{
				{"id1", "1.2.3.4", "rtorrent/0.9.8", "0", "1", "0", "75", "1000", "2000", "50", "60", "<i>", "0", "0", "51413"},
			})
		case "trk":
			// [url, type, is_enabled, group, scrape_complete, scrape_incomplete, scrape_downloaded, normal_interval, scrape_time_last]
			_ = json.NewEncoder(w).Encode([][]any{
				{"https://tracker.example.com/announce", "1", "1", "0", "10", "2", "20", "1800", "1700000000"},
				{"http://backup.test/announce", "1", "0", "1", "0", "0", "0", "1800", "0"},
			})
		case "setprio", "setul", "setdl":
			// 写类操作返回各命令的成功标志数组
			_ = json.NewEncoder(w).Encode([]any{0})
		default:
			// XML multicall 路径（无 mode）：recheck/priority/limits 等只校验状态；
			// 唯一需要解析响应的是 GetSpeedLimits（get_upload_rate/get_download_rate）。
			if strings.Contains(string(b), "get_upload_rate") {
				w.Header().Set("Content-Type", "text/xml")
				_, _ = w.Write([]byte(speedLimitsXML))
				return
			}
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{}`))
		}
	})
	mux.HandleFunc("/php/addtorrent.php", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"Success"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return New(srv.URL, "admin", "pass"), f
}

func TestRTConnectionAndList(t *testing.T) {
	c, _ := newRTClient(t)
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
	if tt.Ratio != 1.5 {
		t.Errorf("ratio: %v", tt.Ratio)
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

func TestRTCoreOps(t *testing.T) {
	c, f := newRTClient(t)
	ctx := context.Background()
	if err := c.AddTorrent(ctx, downloader.AddOptions{URL: "magnet:?xt=urn:btih:aa", Category: "tv", Paused: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.path, "addtorrent") {
		t.Errorf("add path: %s", f.path)
	}
	if err := c.Pause(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	if f.mode != "pause" {
		t.Errorf("pause mode: %s", f.mode)
	}
	if err := c.Resume(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, []string{"abc123"}, false); err != nil {
		t.Fatal(err)
	}
	if f.mode != "remove" {
		t.Errorf("delete mode: %s", f.mode)
	}
	if err := c.Delete(ctx, []string{"abc123"}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.body, "d.erase") {
		t.Errorf("erase xml: %s", f.body)
	}
	if err := c.SetCategory(ctx, []string{"abc123"}, "movies"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.body, "d.custom1.set") {
		t.Errorf("label xml: %s", f.body)
	}
}

func TestRTAdvanced(t *testing.T) {
	c, f := newRTClient(t)
	ctx := context.Background()
	if err := c.Recheck(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.body, "d.check_hash") {
		t.Errorf("recheck: %s", f.body)
	}
	if err := c.Reannounce(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	if err := c.SetQueuePriority(ctx, []string{"abc123"}, "top"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.body, "d.priority.set") {
		t.Errorf("prio: %s", f.body)
	}
	if err := c.SetQueuePriority(ctx, []string{"abc123"}, "bogus"); err == nil {
		t.Error("invalid queue should error")
	}
	cats, err := c.ListCategories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 1 || cats[0] != "iso" {
		t.Errorf("cats: %v", cats)
	}
}

// TestRTAdvancedOps 验证新增能力：文件列表/优先级、Peers、Trackers、全局与单种子限速。
// 每条用 httptest mock 的 action.php 走通「请求构造 + 响应解析」全链路。
func TestRTAdvancedOps(t *testing.T) {
	c, f := newRTClient(t)
	ctx := context.Background()

	// —— ListFiles：mode=fls 响应解析 ——
	files, err := c.ListFiles(ctx, "abc123")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d", len(files))
	}
	if files[0].Name != "movie.mkv" || files[0].Size != 2048 || files[0].Priority != 1 {
		t.Errorf("file[0]: %+v", files[0])
	}
	// 100/200 chunks → 0.5
	if files[0].Progress != 0.5 {
		t.Errorf("file[0] progress: %v", files[0].Progress)
	}
	if files[1].Name != "subs/sub.srt" || files[1].Progress != 0 || files[1].Priority != 0 {
		t.Errorf("file[1]: %+v", files[1])
	}

	// —— SetFilePriority：mode=setprio + 多个 v 索引 + s 优先级 ——
	if err := c.SetFilePriority(ctx, "abc123", []int{0, 1}, 2); err != nil {
		t.Fatalf("SetFilePriority: %v", err)
	}
	for _, want := range []string{"mode=setprio", "hash=ABC123", "v=0", "v=1", "s=2"} {
		if !strings.Contains(f.body, want) {
			t.Errorf("SetFilePriority body missing %q: %s", want, f.body)
		}
	}

	// —— ListPeers：mode=prs 响应解析 ——
	peers, err := c.ListPeers(ctx, "abc123")
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("want 1 peer, got %d", len(peers))
	}
	p := peers[0]
	if p.IP != "1.2.3.4" || p.Client != "rtorrent/0.9.8" || p.Progress != 0.75 || p.DLRate != 50 || p.UPRate != 60 {
		t.Errorf("peer parsed wrong: %+v", p)
	}
	// is_incoming=0 is_encrypted=1 is_snubbed=0 → flags="E"
	if p.Flags != "E" {
		t.Errorf("peer flags: %q want E", p.Flags)
	}

	// —— ListTrackers：mode=trk 响应解析（含 enabled/disabled 两态）——
	trackers, err := c.ListTrackers(ctx, "abc123")
	if err != nil {
		t.Fatalf("ListTrackers: %v", err)
	}
	if len(trackers) != 2 {
		t.Fatalf("want 2 trackers, got %d", len(trackers))
	}
	if trackers[0].URL != "https://tracker.example.com/announce" || trackers[0].Tier != 0 || trackers[0].Status != "working" {
		t.Errorf("tracker[0]: %+v", trackers[0])
	}
	if trackers[1].Status != "disabled" {
		t.Errorf("tracker[1] status: %q want disabled", trackers[1].Status)
	}

	// —— GetSpeedLimits：XML multicall 响应解析 ——
	lim, err := c.GetSpeedLimits(ctx)
	if err != nil {
		t.Fatalf("GetSpeedLimits: %v", err)
	}
	if lim.UploadLimit != 512000 || lim.DownloadLimit != 1024000 {
		t.Errorf("speed limits: %+v", lim)
	}

	// —— SetSpeedLimits：mode=setul / mode=setdl（参数名 s）——
	if err := c.SetSpeedLimits(ctx, downloader.Limits{UploadLimit: 100000, DownloadLimit: 200000}); err != nil {
		t.Fatalf("SetSpeedLimits: %v", err)
	}
	// setdl 是第二个请求（后发），body 应含 mode=setdl
	if !strings.Contains(f.body, "mode=setdl") || !strings.Contains(f.body, "s=200000") {
		t.Errorf("setdl body: %s", f.body)
	}

	// —— SetLimits：单种子限速走 d.set_upload_rate / d.set_download_rate ——
	if err := c.SetLimits(ctx, []string{"abc123"}, downloader.Limits{UploadLimit: 50000, DownloadLimit: 80000}); err != nil {
		t.Fatalf("SetLimits: %v", err)
	}
	if !strings.Contains(f.body, "d.set_upload_rate") || !strings.Contains(f.body, "d.set_download_rate") {
		t.Errorf("SetLimits body missing rate commands: %s", f.body)
	}
	// 确保旧的错误命令名已清除
	if strings.Contains(f.body, "d.up.rate.set") || strings.Contains(f.body, "d.down.rate.set") {
		t.Errorf("SetLimits still uses read-only getter .set: %s", f.body)
	}
}

func TestRTNotImplemented(t *testing.T) {
	c, _ := newRTClient(t)
	ctx := context.Background()
	// 仅保留 ruTorrent 架构上做不到的能力（见各方法注释）
	checks := map[string]func() error{
		"SetForceStart":    func() error { return c.SetForceStart(ctx, nil, true) },
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
