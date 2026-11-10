// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package ut

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// fake 记录最后一次 /gui/ 请求的关键参数，供断言。
type fake struct {
	mu     sync.Mutex
	raw    string
	action string
	hashes []string // 所有 hash 参数值
	s, v   string
}

func (f *fake) record(q url.Values, raw string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.raw = raw
	f.action = q.Get("action")
	f.hashes = q["hash"]
	f.s = q.Get("s")
	f.v = q.Get("v")
}

// sampleRow 构造一个 list=1 的种子元组（关键索引填值，补齐到 27）。
func sampleRow() []any {
	row := make([]any, 27)
	row[0] = "ABC123DEF456"       // HASH
	row[1] = float64(1)           // STATUS=Started
	row[2] = "Ubuntu"             // NAME
	row[3] = float64(1024)        // SIZE
	row[4] = float64(500)         // PROGRESS per-mille → 0.5
	row[5] = float64(600)         // DOWNLOADED
	row[6] = float64(800)         // UPLOADED
	row[7] = float64(2000)        // RATIO per-mille → 2.0
	row[8] = float64(100)         // UPSPEED
	row[9] = float64(50)          // DOWNSPEED
	row[11] = "iso"               // LABEL
	row[12] = float64(2)          // PEERS CONNECTED
	row[13] = float64(10)         // PEERS IN SWARM
	row[15] = float64(3)          // SEEDS CONNECTED
	row[16] = float64(20)         // SEEDS IN SWARM
	row[17] = float64(65536)      // AVAILABILITY → 1.0
	row[18] = float64(1)          // QUEUE ORDER
	row[23] = float64(1700000000) // DATE_ADDED
	row[24] = float64(1700000500) // COMPLETION_DATE
	row[26] = "/data"             // SAVE_PATH
	return row
}

// newUTClient 启动一个 mock uT WebUI 服务并返回 client + 请求记录器。
func newUTClient(t *testing.T) (*Client, *fake) {
	t.Helper()
	f := &fake{}
	mux := http.NewServeMux()
	mux.HandleFunc("/gui/token.html", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "GUID", Value: "g"})
		_, _ = w.Write([]byte(`<html><div id='token'>TKN</div></html>`))
	})
	mux.HandleFunc("/gui/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("token") != "TKN" {
			w.WriteHeader(400)
			return
		}
		if q.Get("list") == "1" {
			w.Header().Set("Content-Type", "application/json")
			b, _ := json.Marshal(map[string]any{"torrents": [][]any{sampleRow()}})
			_, _ = w.Write(b)
			return
		}
		f.record(q, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"build":47178}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestUTConnectionAndList(t *testing.T) {
	c, _ := newUTClient(t)
	ctx := context.Background()
	if err := c.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	list, err := c.ListTorrents(ctx)
	if err != nil {
		t.Fatalf("ListTorrents: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 torrent, got %d", len(list))
	}
	tt := list[0]
	if tt.Name != "Ubuntu" {
		t.Errorf("name: %s", tt.Name)
	}
	if tt.Progress != 0.5 {
		t.Errorf("progress: %v", tt.Progress)
	}
	if tt.Ratio != 2.0 {
		t.Errorf("ratio: %v", tt.Ratio)
	}
	if tt.State != "downloading" {
		t.Errorf("state: %s", tt.State)
	}
	if tt.Category != "iso" {
		t.Errorf("category: %s", tt.Category)
	}
	if tt.UPRate != 100 || tt.DLRate != 50 {
		t.Errorf("rates up=%d dl=%d", tt.UPRate, tt.DLRate)
	}
	if tt.NumSeeds != 3 || tt.NumComplete != 20 {
		t.Errorf("seeds: %d/%d", tt.NumSeeds, tt.NumComplete)
	}
	// CompletedAt 由元组索引 24（COMPLETION_DATE）映射
	if !tt.CompletedAt.Equal(time.Unix(1700000500, 0)) {
		t.Errorf("CompletedAt: %v want 1700000500", tt.CompletedAt)
	}
}

func TestUTAddMagnetSetsProps(t *testing.T) {
	c, f := newUTClient(t)
	ctx := context.Background()
	magnet := "magnet:?xt=urn:btih:abcdef0123456789abcdef0123456789abcdef01&dn=t"
	err := c.AddTorrent(ctx, downloader.AddOptions{
		URL:      magnet,
		Category: "影视",
		Paused:   true,
	})
	if err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	// postAddProps 顺序：category(setprops label) → pause，最后一次应为 pause
	if f.action != "pause" {
		t.Errorf("last action: %s (want pause)", f.action)
	}
	if len(f.hashes) == 0 || f.hashes[0] != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Errorf("hash: %v", f.hashes)
	}
}

func TestUTPauseResumeDeleteCategory(t *testing.T) {
	c, f := newUTClient(t)
	ctx := context.Background()

	if err := c.Pause(ctx, []string{"h1", "h2"}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if f.action != "pause" || len(f.hashes) != 2 {
		t.Errorf("pause: action=%s hashes=%v", f.action, f.hashes)
	}
	if err := c.Resume(ctx, []string{"h1"}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if f.action != "start" {
		t.Errorf("resume action: %s", f.action)
	}
	if err := c.Delete(ctx, []string{"h1"}, true); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if f.action != "removedatatorrent" {
		t.Errorf("delete(data) action: %s", f.action)
	}
	if err := c.Delete(ctx, []string{"h1"}, false); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if f.action != "removetorrent" {
		t.Errorf("delete action: %s", f.action)
	}
	if err := c.SetCategory(ctx, []string{"h1"}, "tag1"); err != nil {
		t.Fatalf("SetCategory: %v", err)
	}
	if f.action != "setprops" || f.s != "label" || f.v != "tag1" {
		t.Errorf("SetCategory: action=%s s=%s v=%s", f.action, f.s, f.v)
	}
}

func TestUTStats(t *testing.T) {
	c, _ := newUTClient(t)
	s, err := c.GlobalStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.UPRate != 100 || s.DLRate != 50 || s.TotalSize != 1024 {
		t.Errorf("stats: %+v", s)
	}
}

func TestUTInvalidToken(t *testing.T) {
	// /gui/ 持续 400（token 始终失效）→ 重试仍失败 → ListTorrents 报错
	mux := http.NewServeMux()
	mux.HandleFunc("/gui/token.html", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><div>TKN</div></html>`))
	})
	mux.HandleFunc("/gui/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, _ := New(srv.URL, "u", "p")
	if _, err := c.ListTorrents(context.Background()); err == nil {
		t.Fatalf("want error on persistently-invalid token")
	}
}

func TestUTBadCredentials(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/gui/token.html", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, _ := New(srv.URL, "u", "p")
	if err := c.TestConnection(context.Background()); err != downloader.ErrInvalidCredentials {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

// TestUTMultipartTokenRetry 验证 doPostMultipart 在 400（token 失效）后清 token、重取、重试一次。
// 与 do 的重试行为一致（add-file 也应自愈），不再静默丢 400 给上层。
func TestUTMultipartTokenRetry(t *testing.T) {
	var (
		mu         sync.Mutex
		tokenCalls int
		postCalls  int
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/gui/token.html", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		tokenCalls++
		tok := "OLD"
		if tokenCalls == 2 { // 第二次（重取）返回新 token
			tok = "NEW"
		}
		mu.Unlock()
		_, _ = w.Write([]byte(`<html><div>` + tok + `</div></html>`))
	})
	mux.HandleFunc("/gui/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method == http.MethodPost {
			mu.Lock()
			postCalls++
			mu.Unlock()
			if q.Get("token") == "OLD" { // 旧 token 失效
				w.WriteHeader(400)
				return
			}
			_, _ = w.Write([]byte(`{"build":47178}`))
			return
		}
		w.WriteHeader(400)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, _ := New(srv.URL, "u", "p")
	err := c.AddTorrent(context.Background(), downloader.AddOptions{
		FileContent: []byte("dummy"),
		FileName:    "x.torrent",
	})
	if err != nil {
		t.Fatalf("AddTorrent multipart retry: %v", err)
	}
	mu.Lock()
	tc, pc := tokenCalls, postCalls
	mu.Unlock()
	if tc != 2 {
		t.Errorf("token.html calls: %d want 2", tc)
	}
	if pc != 2 {
		t.Errorf("multipart POST calls: %d want 2", pc)
	}
}
