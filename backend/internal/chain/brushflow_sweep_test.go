// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// 刷流扫删（RunSweep）回归：体积淘汰与原生限流必须各管各的种子。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"gorm.io/gorm"
)

func tr(hash string, size, upRate int64, lastActive time.Time) downloader.Torrent {
	return downloader.Torrent{Hash: hash, Size: size, UPRate: upRate, LastActivity: lastActive, Tags: []string{"brushflow:1"}}
}

func hashes(ts []downloader.Torrent) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Hash)
	}
	sort.Strings(out)
	return out
}

// TestBFEvictBySize 淘汰集与保留集必须互不相交，且保留集总量回到上限内。
func TestBFEvictBySize(t *testing.T) {
	now := time.Now()
	kept := []downloader.Torrent{
		tr("A", 900e6, 100, now),               // 上传快 → 保留
		tr("B", 900e6, 1, now.Add(-time.Hour)), // 上传慢 → 先淘汰
	}
	evict, remain := bfEvictBySize(kept, 1<<30) // 1 GiB 上限，两条共 ~1.8e9 → 必淘汰一条
	if got := hashes(evict); strings.Join(got, ",") != "B" {
		t.Errorf("应淘汰上传最慢的 B, got %v", got)
	}
	if got := hashes(remain); strings.Join(got, ",") != "A" {
		t.Errorf("保留集只能是 A（旧行为：被淘汰的仍留在 kept 里）, got %v", got)
	}
	for _, h := range remain {
		for _, e := range evict {
			if h.Hash == e.Hash {
				t.Errorf("淘汰项 %s 混在保留集里（会被一起送去设原生限流）", h.Hash)
			}
		}
	}

	// 正控：未超限时不淘汰，保留集原样返回。
	evict2, remain2 := bfEvictBySize(kept, 10<<30)
	if len(evict2) != 0 {
		t.Errorf("未超限不该淘汰, got %v", hashes(evict2))
	}
	if len(remain2) != 2 {
		t.Errorf("未超限保留集应原样 2 条, got %d", len(remain2))
	}
	// 正控：输入切片顺序不被就地改写（排序只在副本上做）。
	if kept[0].Hash != "A" || kept[1].Hash != "B" {
		t.Errorf("bfEvictBySize 不该重排调用方的切片, got %v", hashes(kept))
	}
}

// sweepRecorder 记录假 qB 收到的删种与设限流请求。
type sweepRecorder struct {
	mu         sync.Mutex
	deleted    []string
	limited    []string
	listJSON   string
	shareCalls int
}

// record 从 qB 的 hashes 参数里挑出 infohash（joinHashes 用 "|" 连接，delete 也可能是 "all"）。
func (s *sweepRecorder) record(bucket *[]string, raw string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tok := range strings.FieldsFunc(raw, func(r rune) bool {
		return !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F'))
	}) {
		if len(tok) >= 32 {
			*bucket = append(*bucket, strings.ToLower(tok[:32]))
		}
	}
}

// newBrushSweepHarness 真 RunSweep 链路：真 oper/dlCh + 假 qB（返回固定种子列表并记录调用）。
func newBrushSweepHarness(t *testing.T, torrents map[string]map[string]any, maxGB float64) (*BrushFlowChain, uint, *sweepRecorder) {
	t.Helper()
	rec := &sweepRecorder{}
	payload, err := json.Marshal(torrents)
	if err != nil {
		t.Fatal(err)
	}
	rec.listJSON = string(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/api/v2/auth/login"):
			_, _ = w.Write([]byte("Ok."))
		case strings.HasSuffix(path, "/api/v2/sync/maindata"):
			_, _ = w.Write([]byte(`{"rid":1,"full_update":true,"torrents":` + rec.listJSON + `,"server_state":{}}`))
		case strings.HasSuffix(path, "/api/v2/torrents/delete"):
			_ = r.ParseForm()
			rec.record(&rec.deleted, r.FormValue("hashes"))
			_, _ = w.Write([]byte("Ok."))
		case strings.HasSuffix(path, "/api/v2/torrents/setShareLimits"):
			_ = r.ParseForm()
			rec.record(&rec.limited, r.FormValue("hashes"))
			rec.mu.Lock()
			rec.shareCalls++
			rec.mu.Unlock()
			_, _ = w.Write([]byte("Ok."))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/bf-sweep.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Site{}, &models.Downloader{}, &models.BrushFlowTask{}, &models.BrushFlowSeed{}); err != nil {
		t.Fatal(err)
	}
	crypto := security.NewCrypto("bf-sweep")
	dlOp := oper.NewDownloaderOper(db, crypto)
	d, err := dlOp.Create(oper.DownloaderInput{Name: "qb", Type: "qb", URL: srv.URL, Username: "u", Password: "p", IsDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, crypto)
	base := NewBase(module.New(), event.New())
	dlCh := NewDownloadChain(base, manager.NewManager(dlOp), dlOp, nil)
	brushOp := oper.NewBrushFlowOper(db)
	svc := NewBrushFlowChain(base, siteOp, NewSiteChain(base, siteOp, nil), dlCh, brushOp, nil, nil)
	task := &models.BrushFlowTask{
		Name: "sweep", DownloaderID: d.ID, Enabled: true,
		MaxTotalSizeGB: maxGB, UseShareLimit: true, DeleteRatio: 2, // 规则阈值放宽到 2，本轮只由体积淘汰决定删谁
	}
	if err := brushOp.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	return svc, task.ID, rec
}

// TestRunSweepDoesNotLimitDeletedSeeds 被淘汰删掉的种子不能再出现在「给保留种子设原生限流」的请求里。
// TR/Deluge 的 torrent-set 遇到不存在的 id 会整批报错，混进去就等于真正留下的种子也设不上限
// （做种超限不停）。
func TestRunSweepDoesNotLimitDeletedSeeds(t *testing.T) {
	now := time.Now()
	list := map[string]map[string]any{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": { // 上传快，保留
			"name": "keep", "size": 900e6, "upspeed": float64(100), "state": "stalledUP",
			"tags": "brushflow:1", "added_on": now.Add(-3 * time.Hour).Unix(), "last_activity": now.Unix(),
			"ratio": 1.0, "uploaded": 1000.0,
		},
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb": { // 上传最慢，体积淘汰首选
			"name": "evict", "size": 900e6, "upspeed": float64(1), "state": "stalledUP",
			"tags": "brushflow:1", "added_on": now.Add(-3 * time.Hour).Unix(), "last_activity": now.Add(-time.Hour).Unix(),
			"ratio": 1.0, "uploaded": 500.0,
		},
	}
	svc, taskID, rec := newBrushSweepHarness(t, list, 1) // 1 GB 上限 < 两条 900 MB

	if err := svc.RunSweep(context.Background(), taskID); err != nil {
		t.Fatalf("RunSweep: %v", err)
	}
	rec.mu.Lock()
	deleted := append([]string(nil), rec.deleted...)
	limited := append([]string(nil), rec.limited...)
	rec.mu.Unlock()

	if len(deleted) == 0 {
		t.Fatal("前提：超限应删掉种子，假 qB 未收到 delete 调用")
	}
	var victim string
	for _, h := range deleted {
		victim = h
	}
	for _, h := range limited {
		if h == victim {
			t.Errorf("被删掉的种子 %s… 又被送去设原生限流（旧行为：淘汰项仍留在 kept 里）", h[:8])
		}
	}
	if len(limited) == 0 {
		t.Errorf("保留种子应被设原生限流，假 qB 未收到 setShareLimits 名单")
	}
}
