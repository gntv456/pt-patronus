package qb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// TestQBAdvancedOps 用 httptest mock qB WebUI，逐一验证 P2 新方法的请求构造
// （端点路径 + 表单参数）。这是「自动联调」：模拟真实 qB 的响应，跑通每个新操作的 HTTP 交互。
func TestQBAdvancedOps(t *testing.T) {
	var lastPath, lastForm string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		lastForm = r.PostForm.Encode()
		lastPath = r.URL.Path
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(200)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.0.0"))
		case "/api/v2/torrents/setShareLimits",
			"/api/v2/torrents/topPriority", "/api/v2/torrents/increasePrio",
			"/api/v2/torrents/decreasePrio", "/api/v2/torrents/bottomPriority",
			"/api/v2/torrents/setForceStart",
			"/api/v2/torrents/toggleSequentialDownload",
			"/api/v2/torrents/toggleFirstLastPiecePrio",
			"/api/v2/torrents/createCategory", "/api/v2/torrents/editCategory",
			"/api/v2/torrents/removeCategories":
			w.WriteHeader(200)
		case "/api/v2/sync/torrentPeers":
			_, _ = w.Write([]byte(`{"rid":1,"peers":{"1.2.3.4:6881":{"client":"qB 5.0","progress":0.5,"dl_speed":100,"up_speed":200,"flags":"D x","country":"CN"}}}`))
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

	// —— SetShareLimits：ratioLimit/seedingTimeLimit 表单 ——
	if err := c.SetShareLimits(ctx, []string{"h1"}, downloader.ShareLimits{RatioLimit: 2.5, SeedingTimeLimit: 60}); err != nil {
		t.Fatalf("SetShareLimits: %v", err)
	}
	if lastPath != "/api/v2/torrents/setShareLimits" {
		t.Errorf("SetShareLimits path=%s", lastPath)
	}
	for _, want := range []string{"hashes=h1", "ratioLimit=2.5", "seedingTimeLimit=60"} {
		if !strings.Contains(lastForm, want) {
			t.Errorf("SetShareLimits form missing %q: %s", want, lastForm)
		}
	}

	// —— SetQueuePriority：action→端点映射 ——
	for _, tc := range []struct{ action, path string }{
		{"top", "/api/v2/torrents/topPriority"},
		{"up", "/api/v2/torrents/increasePrio"},
		{"down", "/api/v2/torrents/decreasePrio"},
		{"bottom", "/api/v2/torrents/bottomPriority"},
	} {
		if err := c.SetQueuePriority(ctx, []string{"h"}, tc.action); err != nil {
			t.Fatalf("queue %s: %v", tc.action, err)
		}
		if lastPath != tc.path {
			t.Errorf("queue %s → %s, want %s", tc.action, lastPath, tc.path)
		}
	}
	if err := c.SetQueuePriority(ctx, []string{"h"}, "sideways"); err == nil {
		t.Error("expected error for invalid queue action")
	}

	// —— SetForceStart：value 表单 ——
	if err := c.SetForceStart(ctx, []string{"h"}, true); err != nil {
		t.Fatalf("SetForceStart: %v", err)
	}
	if lastPath != "/api/v2/torrents/setForceStart" || !strings.Contains(lastForm, "value=true") || !strings.Contains(lastForm, "hashes=h") {
		t.Errorf("SetForceStart: path=%s form=%s", lastPath, lastForm)
	}

	// —— Toggle 顺序下载 / 首尾优先 ——
	if err := c.ToggleSequentialDownload(ctx, []string{"h"}); err != nil {
		t.Fatalf("ToggleSequential: %v", err)
	}
	if lastPath != "/api/v2/torrents/toggleSequentialDownload" {
		t.Errorf("seq path=%s", lastPath)
	}
	if err := c.ToggleFirstLastPiece(ctx, []string{"h"}); err != nil {
		t.Fatalf("ToggleFirstLast: %v", err)
	}
	if lastPath != "/api/v2/torrents/toggleFirstLastPiecePrio" {
		t.Errorf("firstlast path=%s", lastPath)
	}

	// —— ListPeers：torrentPeers 响应解析 ——
	peers, err := c.ListPeers(ctx, "h")
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("peers len=%d", len(peers))
	}
	p := peers[0]
	if p.IP != "1.2.3.4:6881" || p.Client != "qB 5.0" || p.Progress != 0.5 || p.DLRate != 100 || p.UPRate != 200 || p.Flags != "D x" {
		t.Errorf("peer parsed wrong: %+v", p)
	}

	// —— 分类增删 ——
	if err := c.CreateCategory(ctx, "cat1", "/data/cat1"); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if lastPath != "/api/v2/torrents/createCategory" || !strings.Contains(lastForm, "category=cat1") || !strings.Contains(lastForm, "savePath=") {
		t.Errorf("CreateCategory: path=%s form=%s", lastPath, lastForm)
	}
	if err := c.EditCategory(ctx, "cat1", "/data/new"); err != nil {
		t.Fatalf("EditCategory: %v", err)
	}
	if lastPath != "/api/v2/torrents/editCategory" {
		t.Errorf("EditCategory path=%s", lastPath)
	}
	if err := c.DeleteCategories(ctx, []string{"cat1", "cat2"}); err != nil {
		t.Fatalf("DeleteCategories: %v", err)
	}
	if lastPath != "/api/v2/torrents/removeCategories" || !strings.Contains(lastForm, "cat1") || !strings.Contains(lastForm, "cat2") {
		t.Errorf("DeleteCategories: path=%s form=%s", lastPath, lastForm)
	}
}

// TestQBTorrentDetailFields 验证 toTorrent 把 sync/maindata 的详情字段正确映射到统一模型
// （做种数/可用性/比率上限/标志位/完成时间等）——这些字段随 WS 流零请求实时推送，映射错则详情全错。
func TestQBTorrentDetailFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(200)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.0.0"))
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"rid":1,"full_update":true,"torrents":{"a":{
				"name":"A","size":1000,"downloaded":500,"uploaded":2000,"progress":0.5,"ratio":2.0,
				"state":"uploading","dlspeed":10,"upspeed":20,"save_path":"/data","category":"cat","tags":"t1",
				"added_on":1700000000,"completion_on":1700001000,
				"num_seeds":3,"num_leechs":1,"num_complete":10,"num_incomplete":2,
				"availability":0.95,"ratio_limit":2.5,"seeding_time_limit":60,
				"force_start":true,"auto_tmm":true,"seq_dl":true,"f_l_piece_prio":false,"priority":1
			}},"server_state":{},"tags":[]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	ts, err := c.ListTorrents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("len=%d", len(ts))
	}
	d := ts[0]
	checks := map[string]bool{
		"NumSeeds=3":       d.NumSeeds == 3,
		"NumLeechs=1":      d.NumLeechs == 1,
		"NumComplete=10":   d.NumComplete == 10,
		"NumIncomplete=2":  d.NumIncomplete == 2,
		"Availability=.95": d.Availability == 0.95,
		"RatioLimit=2.5":   d.RatioLimit == 2.5,
		"SeedingTime=60":   d.SeedingTimeLimit == 60,
		"ForceStart=true":  d.ForceStart,
		"AutoTMM=true":     d.AutoTMM,
		"Sequential=true":  d.Sequential,
		"FirstLast=false":  !d.FirstLastPiece,
		"Priority=1":       d.Priority == 1,
		"Uploaded=2000":    d.Uploaded == 2000,
		"CompletedAt set":  !d.CompletedAt.IsZero(),
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("field %s wrong: %+v", name, d)
		}
	}
}
