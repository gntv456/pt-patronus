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

type flFake struct {
	mu     sync.Mutex
	path   string
	method string
	body   string
}

func (f *flFake) record(r *http.Request, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.path = r.URL.Path
	f.method = r.Method
	f.body = body
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

func newFLClient(t *testing.T) (*Client, *flFake) {
	t.Helper()
	f := &flFake{}
	mux := http.NewServeMux()
	// jesec paths only (legacy verify 404 → jesec)
	mux.HandleFunc("/auth/verify", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	mux.HandleFunc("/api/auth/authenticate", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		http.SetCookie(w, &http.Cookie{Name: "jwt", Value: "tok"})
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	mux.HandleFunc("/api/client/connection-test", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"isConnect":true}`))
	})
	mux.HandleFunc("/api/torrents", func(w http.ResponseWriter, r *http.Request) {
		f.record(r, "")
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 1, "torrents": map[string]any{"ABC123": sampleTorrent()},
			})
			return
		}
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/torrents/add-urls", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/torrents/start", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/torrents/stop", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/torrents/delete", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/torrents/check-hash", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/torrents/priority", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/torrents/tags", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.record(r, string(b))
		w.WriteHeader(200)
	})
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
	if !strings.Contains(f.path, "priority") || !strings.Contains(f.body, "3") {
		t.Errorf("prio: path=%s body=%s", f.path, f.body)
	}
	if err := c.SetQueuePriority(ctx, []string{"abc123"}, "bogus"); err == nil {
		t.Error("invalid queue should error")
	}
	if err := c.SetTags(ctx, []string{"abc123"}, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if f.method != http.MethodPatch {
		t.Errorf("tags method: %s", f.method)
	}
}

func TestFLNotImplemented(t *testing.T) {
	c, _ := newFLClient(t)
	ctx := context.Background()
	checks := map[string]func() error{
		"SetForceStart":    func() error { return c.SetForceStart(ctx, nil, true) },
		"RemoveTags":       func() error { return c.RemoveTags(ctx, nil, nil) },
		"CreateCategory":   func() error { return c.CreateCategory(ctx, "n", "") },
		"EditCategory":     func() error { return c.EditCategory(ctx, "n", "") },
		"DeleteCategories": func() error { return c.DeleteCategories(ctx, nil) },
		"SetShareLimits":   func() error { return c.SetShareLimits(ctx, nil, downloader.ShareLimits{}) },
		"ToggleSequential": func() error { return c.ToggleSequentialDownload(ctx, nil) },
		"ToggleFirstLast":  func() error { return c.ToggleFirstLastPiece(ctx, nil) },
		"SetLimits":        func() error { return c.SetLimits(ctx, nil, downloader.Limits{}) },
		"GetSpeedLimits":   func() error { _, e := c.GetSpeedLimits(ctx); return e },
		"SetSpeedLimits":   func() error { return c.SetSpeedLimits(ctx, downloader.Limits{}) },
		"AddTrackers":      func() error { return c.AddTrackers(ctx, "h", nil) },
		"RemoveTrackers":   func() error { return c.RemoveTrackers(ctx, "h", nil) },
	}
	for name, fn := range checks {
		if err := fn(); err != downloader.ErrNotImplemented {
			t.Errorf("%s: want ErrNotImplemented, got %v", name, err)
		}
	}
}
