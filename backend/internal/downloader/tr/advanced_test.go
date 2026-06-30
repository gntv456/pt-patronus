package tr

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// TestTRAdvancedOps 用 httptest mock Transmission RPC，验证 P2 新方法：
// SetShareLimits 的 -2/-1/>=0 → seedRatioMode/Limit 映射、队列 top/bottom、
// 不支持能力返回 ErrNotImplemented、ListPeers 解析。
func TestTRAdvancedOps(t *testing.T) {
	var lastMethod string
	var lastArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transmission/rpc" {
			w.WriteHeader(404)
			return
		}
		// 首次无 session-id → 409 握手
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			w.Header().Set("X-Transmission-Session-Id", "SID")
			w.WriteHeader(409)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method    string         `json:"method"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(body, &req)
		lastMethod = req.Method
		lastArgs = req.Arguments
		switch req.Method {
		case "torrent-get":
			_, _ = w.Write([]byte(`{"result":"success","arguments":{"torrents":[{"peers":[
				{"address":"1.2.3.4","clientName":"TR","progress":0.7,"rateToClient":50,"rateToPeer":60,"flagStr":"U"}
			]}]}}`))
		default:
			_, _ = w.Write([]byte(`{"result":"success","arguments":{}}`))
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "u", "p")
	ctx := context.Background()

	// —— SetShareLimits: -2 无限 → mode=2；-1 全局 → mode=0 ——
	if err := c.SetShareLimits(ctx, []string{"h"}, downloader.ShareLimits{RatioLimit: -2, SeedingTimeLimit: -1}); err != nil {
		t.Fatalf("SetShareLimits -2: %v", err)
	}
	if lastMethod != "torrent-set" {
		t.Errorf("method=%s, want torrent-set", lastMethod)
	}
	if lastArgs["seedRatioMode"] != float64(2) {
		t.Errorf("unlimited seedRatioMode=%v", lastArgs["seedRatioMode"])
	}
	if lastArgs["seedIdleMode"] != float64(0) {
		t.Errorf("global seedIdleMode=%v", lastArgs["seedIdleMode"])
	}

	// —— -1 全局 → mode=0；时长 -2 → idle mode=2 ——
	if err := c.SetShareLimits(ctx, []string{"h"}, downloader.ShareLimits{RatioLimit: -1, SeedingTimeLimit: -2}); err != nil {
		t.Fatal(err)
	}
	if lastArgs["seedRatioMode"] != float64(0) {
		t.Errorf("global seedRatioMode=%v", lastArgs["seedRatioMode"])
	}
	if lastArgs["seedIdleMode"] != float64(2) {
		t.Errorf("unlimited seedIdleMode=%v", lastArgs["seedIdleMode"])
	}

	// —— >=0 自定义 → mode=1 + limit 值 ——
	if err := c.SetShareLimits(ctx, []string{"h"}, downloader.ShareLimits{RatioLimit: 3.0, SeedingTimeLimit: 90}); err != nil {
		t.Fatal(err)
	}
	if lastArgs["seedRatioMode"] != float64(1) {
		t.Errorf("override seedRatioMode=%v", lastArgs["seedRatioMode"])
	}
	if lastArgs["seedRatioLimit"] != 3.0 {
		t.Errorf("seedRatioLimit=%v", lastArgs["seedRatioLimit"])
	}
	if lastArgs["seedIdleMode"] != float64(1) {
		t.Errorf("override seedIdleMode=%v", lastArgs["seedIdleMode"])
	}
	if lastArgs["seedIdleLimit"] != float64(90) {
		t.Errorf("seedIdleLimit=%v", lastArgs["seedIdleLimit"])
	}

	// —— 队列：top/bottom 走 queuePosition；up/down 返 ErrNotImplemented ——
	if err := c.SetQueuePriority(ctx, []string{"h"}, "top"); err != nil {
		t.Fatalf("queue top: %v", err)
	}
	if lastArgs["queuePosition"] != float64(0) {
		t.Errorf("top queuePosition=%v", lastArgs["queuePosition"])
	}
	if err := c.SetQueuePriority(ctx, []string{"h"}, "bottom"); err != nil {
		t.Fatal(err)
	}
	if lastArgs["queuePosition"] != float64(2147483647) {
		t.Errorf("bottom queuePosition=%v", lastArgs["queuePosition"])
	}
	if err := c.SetQueuePriority(ctx, []string{"h"}, "up"); err != downloader.ErrNotImplemented {
		t.Errorf("queue up: want ErrNotImplemented, got %v", err)
	}
	if err := c.SetQueuePriority(ctx, []string{"h"}, "down"); err != downloader.ErrNotImplemented {
		t.Errorf("queue down: want ErrNotImplemented, got %v", err)
	}

	// —— 不支持的能力统一返 ErrNotImplemented ——
	for name, fn := range map[string]func() error{
		"forceStart":      func() error { return c.SetForceStart(ctx, []string{"h"}, true) },
		"toggleSeq":       func() error { return c.ToggleSequentialDownload(ctx, []string{"h"}) },
		"toggleFirstLast": func() error { return c.ToggleFirstLastPiece(ctx, []string{"h"}) },
		"createCategory":  func() error { return c.CreateCategory(ctx, "n", "") },
		"editCategory":    func() error { return c.EditCategory(ctx, "n", "") },
		"deleteCategory":  func() error { return c.DeleteCategories(ctx, []string{"n"}) },
	} {
		if err := fn(); err != downloader.ErrNotImplemented {
			t.Errorf("%s: want ErrNotImplemented, got %v", name, err)
		}
	}

	// —— ListPeers：torrent-get peers 解析 ——
	peers, err := c.ListPeers(ctx, "h")
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("peers len=%d", len(peers))
	}
	p := peers[0]
	if p.IP != "1.2.3.4" || p.Client != "TR" || p.Progress != 0.7 || p.DLRate != 50 || p.UPRate != 60 || p.Flags != "U" {
		t.Errorf("peer parsed wrong: %+v", p)
	}
}

// TestTRTorrentDetailFields 验证 toTorrent 把 TR torrent-get 新字段映射到统一模型
// （做种/下载数、比率上限 mode 推导、队列、完成时间）。TR 无 swarm 总数/可用性/强制/顺序，留零值。
func TestTRTorrentDetailFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transmission/rpc" {
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			w.Header().Set("X-Transmission-Session-Id", "SID")
			w.WriteHeader(409)
			return
		}
		_, _ = w.Write([]byte(`{"result":"success","arguments":{"torrents":[{
			"id":1,"name":"T","hashString":"abc","totalSize":1000,"downloadedEver":500,"uploadedEver":2000,
			"percentDone":0.5,"uploadRatio":2.0,"status":5,"rateDownload":10,"rateUpload":20,
			"downloadDir":"/d","labels":[],"addedDate":1700000000,
			"peersConnected":4,"peersSendingToUs":3,"peersGettingFromUs":1,
			"seedRatioLimit":2.5,"seedRatioMode":1,"seedIdleLimit":60,"seedIdleMode":1,
			"queuePosition":2,"doneDate":1700001000
		}]}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "u", "p")
	ts, err := c.ListTorrents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("len=%d", len(ts))
	}
	d := ts[0]
	checks := map[string]bool{
		"NumSeeds=3":      d.NumSeeds == 3,     // peersSendingToUs
		"NumLeechs=1":     d.NumLeechs == 1,    // peersGettingFromUs
		"RatioLimit=2.5":  d.RatioLimit == 2.5, // mode1=override → 具体值
		"SeedingTime=60":  d.SeedingTimeLimit == 60,
		"Priority=2":      d.Priority == 2,
		"Uploaded=2000":   d.Uploaded == 2000,
		"CompletedAt set": !d.CompletedAt.IsZero(),
		// TR 无：留零值
		"NumComplete=0":    d.NumComplete == 0,
		"Availability=0":   d.Availability == 0,
		"ForceStart=false": !d.ForceStart,
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("field %s wrong: %+v", name, d)
		}
	}
}
