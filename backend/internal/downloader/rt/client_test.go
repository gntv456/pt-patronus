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
		if strings.Contains(string(b), "mode=list") || strings.Contains(string(b), "mode%3Dlist") {
			// form may be mode=list
		}
		_ = r.ParseForm()
		if r.Form.Get("mode") == "list" {
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{"t": map[string]any{"ABC123": sampleRow()}, "cid": 1}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
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

func TestRTNotImplemented(t *testing.T) {
	c, _ := newRTClient(t)
	ctx := context.Background()
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
		"GetSpeedLimits":   func() error { _, e := c.GetSpeedLimits(ctx); return e },
		"SetSpeedLimits":   func() error { return c.SetSpeedLimits(ctx, downloader.Limits{}) },
		"ListFiles":        func() error { _, e := c.ListFiles(ctx, "h"); return e },
		"SetFilePriority":  func() error { return c.SetFilePriority(ctx, "h", nil, 0) },
		"ListPeers":        func() error { _, e := c.ListPeers(ctx, "h"); return e },
		"ListTrackers":     func() error { _, e := c.ListTrackers(ctx, "h"); return e },
		"AddTrackers":      func() error { return c.AddTrackers(ctx, "h", nil) },
		"RemoveTrackers":   func() error { return c.RemoveTrackers(ctx, "h", nil) },
	}
	for name, fn := range checks {
		if err := fn(); err != downloader.ErrNotImplemented {
			t.Errorf("%s: want ErrNotImplemented, got %v", name, err)
		}
	}
}
