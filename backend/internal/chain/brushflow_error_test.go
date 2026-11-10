// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// fakeBrushAdapter 是模块注册表里的站点适配器替身：固定返回预设种子列表。
type fakeBrushAdapter struct{ torrents []site.Torrent }

func (f *fakeBrushAdapter) Name() string   { return "fake-brush-adapter" }
func (f *fakeBrushAdapter) Priority() int  { return 50 }
func (f *fakeBrushAdapter) Schema() string { return "fake" }

func (f *fakeBrushAdapter) Search(ctx context.Context, def *site.SiteMetadata, baseURL, cookie, keyword, category string, page int) ([]site.Torrent, error) {
	return f.torrents, nil
}

func (f *fakeBrushAdapter) FetchUserInfo(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {
	return nil, nil
}

// newFakeQBServer 起一个只回固定应答的 qB WebUI：登录 Ok、同步空列表、添加计数 Ok。
func newFakeQBServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var adds atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/v2/auth/login"):
			w.Write([]byte("Ok."))
		case strings.HasSuffix(r.URL.Path, "/api/v2/sync/maindata"):
			w.Write([]byte(`{"rid":1,"full_update":true,"torrents":{},"server_state":{}}`))
		case strings.HasSuffix(r.URL.Path, "/api/v2/torrents/add"):
			adds.Add(1)
			w.Write([]byte("Ok."))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &adds
}

// newBrushSelectHarness 组装真实 RunSelect 链路：真 siteCh/dlCh/oper + fake 站点适配器 + fake qB。
// 返回链、DB、任务 ID、适配器（可改 torrents 换候选）与 add 计数。
func newBrushSelectHarness(t *testing.T) (*BrushFlowChain, *gorm.DB, uint, *fakeBrushAdapter, *atomic.Int64) {
	t.Helper()
	qbSrv, adds := newFakeQBServer(t)
	return newBrushSelectHarnessAt(t, qbSrv.URL, adds)
}

// newBrushSelectHarnessAt 同上，但下载器指向调用方给定的 URL（可换成一会 500 的假 qB）。
func newBrushSelectHarnessAt(t *testing.T, qbURL string, adds *atomic.Int64) (*BrushFlowChain, *gorm.DB, uint, *fakeBrushAdapter, *atomic.Int64) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/bf.db"), &gorm.Config{})
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
	crypto := security.NewCrypto("bf-error-test")
	siteOp := oper.NewSiteOper(db, crypto)
	st, err := siteOp.Create(oper.SiteInput{Name: "fakesite", Platform: "fake", BaseURL: "http://fakesite.local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Downloader{Name: "qb", Type: "qb", URL: qbURL}).Error; err != nil {
		t.Fatal(err)
	}
	adapter := &fakeBrushAdapter{torrents: []site.Torrent{
		{ID: "1", Title: "cand-1", DownloadURL: "http://fakesite.local/dl/1", Size: "1 GB", Seeders: 5},
	}}
	mm := module.New()
	mm.Register(adapter)
	base := NewBase(mm, event.New())
	dlOp := oper.NewDownloaderOper(db, crypto)
	dlCh := NewDownloadChain(base, manager.NewManager(dlOp), dlOp, nil)
	brushOp := oper.NewBrushFlowOper(db)
	svc := NewBrushFlowChain(base, siteOp, NewSiteChain(base, siteOp, nil), dlCh, brushOp, nil, nil)

	task := &models.BrushFlowTask{Name: "t", SiteID: st.ID, DownloaderID: 1, MaxSeeds: 5, Enabled: true}
	if err := brushOp.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	return svc, db, task.ID, adapter, adds
}

// TestRunSelectHistFailVisibleAndClears [P1] AddSeed 落库失败必须在任务页可见（last_error），
// 且仅当本轮无失败时才允许清空。
//
// 缺陷：循环内写 last_error 后，函数末尾无条件 SetTaskError("") 同运行清空——
// 用户永远看不到「种子历史写入失败」诊断（下轮 SeedExists 查不到会重复加种）。
func TestRunSelectHistFailVisibleAndClears(t *testing.T) {
	svc, db, taskID, adapter, adds := newBrushSelectHarness(t)
	ctx := context.Background()

	// 注入 AddSeed 落库失败：SELECT 正常（SeedExists 不报错），INSERT 拒绝。
	if err := db.Exec("CREATE TRIGGER bf_seed_fail BEFORE INSERT ON brush_flow_seeds BEGIN SELECT RAISE(ABORT, 'injected insert failure'); END;").Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RunSelect(ctx, taskID); err != nil {
		t.Fatalf("软错误不应上抛: %v", err)
	}
	got, err := oper.NewBrushFlowOper(db).GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.LastError, "种子历史写入失败") {
		t.Errorf("AddSeed 失败应留任务页诊断, got last_error=%q（被同运行末尾无条件清空）", got.LastError)
	}
	if got.TotalAdded != 1 || adds.Load() != 1 {
		t.Errorf("统计/下载器添加应各 +1: total_added=%d adds=%d", got.TotalAdded, adds.Load())
	}

	// 扫删成功不得擦掉选种刚写入的诊断（两者每 10min/5min 交错运行，无条件清空会让诊断
	// 在 5 分钟内消失）。
	if err := svc.RunSweep(ctx, taskID); err != nil {
		t.Fatal(err)
	}
	got, err = oper.NewBrushFlowOper(db).GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.LastError, "种子历史写入失败") {
		t.Errorf("扫删成功擦掉了选种诊断, got last_error=%q", got.LastError)
	}
	// 正向反控：扫删清自己的错误（来源匹配才允许清）。
	if err := oper.NewBrushFlowOper(db).SetTaskError(taskID, "sweep", "sweep boom"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RunSweep(ctx, taskID); err != nil {
		t.Fatal(err)
	}
	got, err = oper.NewBrushFlowOper(db).GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastError != "" {
		t.Errorf("扫删成功应清同源错误, got %q", got.LastError)
	}

	// 正向反控：解除故障后本轮无失败 → 旧诊断应被清空（else 分支真的执行）。
	if err := db.Exec("DROP TRIGGER bf_seed_fail").Error; err != nil {
		t.Fatal(err)
	}
	adapter.torrents = []site.Torrent{
		{ID: "2", Title: "cand-2", DownloadURL: "http://fakesite.local/dl/2", Size: "1 GB", Seeders: 6},
	}
	if err := svc.RunSelect(ctx, taskID); err != nil {
		t.Fatal(err)
	}
	got, err = oper.NewBrushFlowOper(db).GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastError != "" {
		t.Errorf("无失败轮应清空旧诊断, got %q", got.LastError)
	}
	if got.TotalAdded != 2 || adds.Load() != 2 {
		t.Errorf("第二轮应再 +1: total_added=%d adds=%d", got.TotalAdded, adds.Load())
	}
}
