// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"gorm.io/gorm"
)

// rssAutoDLEnv RSS→自动下载 全链夹具：假 feed + 假 qB（前 N 次 add 返回 500 模拟临时故障）+ SQLite。
type rssAutoDLEnv struct {
	ch       *RssChain
	itemOp   *oper.RssItemOper
	dlOp     *oper.DownloaderOper
	addCalls *int32
	feedURL  string
	qbURL    string
}

// newRssAutoDLEnv withDownloader=false 时不建下载器（覆盖「AutoDownload 已开但无下载器」）。
func newRssAutoDLEnv(t *testing.T, feedBody string, addFailures int32, withDownloader bool) *rssAutoDLEnv {
	t.Helper()
	var addCalls int32
	qbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			if atomic.AddInt32(&addCalls, 1) <= addFailures {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(qbSrv.Close)

	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(feedBody))
	}))
	t.Cleanup(feedSrv.Close)

	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/rss-autodl.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.RssSubscription{}, &models.RssItem{}, &models.Downloader{}, &models.Site{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	crypto := security.NewCrypto("rss-autodl-test-seed")
	dlOp := oper.NewDownloaderOper(db, crypto)
	if withDownloader {
		if _, err := dlOp.Create(oper.DownloaderInput{
			Name: "mock-qb", Type: "qb", URL: qbSrv.URL, Username: "u", Password: "p", IsDefault: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true}) // httptest=127.0.0.1
	if err != nil {
		t.Fatal(err)
	}
	base := NewBase(module.New(), event.New())
	itemOp := oper.NewRssItemOper(db)
	dlCh := NewDownloadChain(base, manager.NewManager(dlOp), dlOp, nil)
	ch := NewRssChain(base, oper.NewRssOper(db), itemOp, dlCh, dlOp, nil, nil, hc, nil)
	return &rssAutoDLEnv{ch: ch, itemOp: itemOp, dlOp: dlOp, addCalls: &addCalls, feedURL: feedSrv.URL, qbURL: qbSrv.URL}
}

// rssAutoDLFeed 单条目磁力 feed（磁力直推下载器，无需站点抓种字节）。
const rssAutoDLFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>AutoDL Feed</title>
<item><title>Movie.2026.1080p.WEB-DL</title><link>magnet:?xt=urn:btih:deadbeefdeadbeefdeadbeefdeadbeefdeadbeef</link><guid>adl-1</guid></item>
</channel></rss>`

// TestRssAutoDownloadRetriesAfterAddFailure 推种失败（下载器临时故障）必须回滚去重行：
// 否则本条被永久视为已见，下轮 Check 不再重试，种子静默丢失。
func TestRssAutoDownloadRetriesAfterAddFailure(t *testing.T) {
	env := newRssAutoDLEnv(t, rssAutoDLFeed, 1, true)
	dl, err := env.dlOp.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	sub := &models.RssSubscription{ID: 301, Name: "auto", URL: env.feedURL, AutoDownload: true, DownloaderID: dl.ID}

	// 第 1 轮：add 500 → 条目应回滚（不落去重行）。
	res, err := env.ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("check1 err: %v", err)
	}
	if res.New != 1 {
		t.Fatalf("check1 new=%d want 1", res.New)
	}
	if got := *env.addCalls; got != 1 {
		t.Fatalf("check1 addCalls=%d want 1", got)
	}
	if items, _ := env.itemOp.ListBySubscription(sub.ID, 10); len(items) != 0 {
		t.Fatalf("add 失败后应回滚去重行（否则永不重试），got %d 行", len(items))
	}
	// 失败必须进本轮账面：只回 ok:true/new_count=1 时，界面「新增 1 条」而条目列表为空，
	// 用户看不到自动下载一直没成功。
	if res.DownloadFailed != 1 || len(res.DownloadErrors) == 0 {
		t.Fatalf("推种失败要计数并带原因, got failed=%d errors=%v", res.DownloadFailed, res.DownloadErrors)
	}

	// 第 2 轮：add 恢复 → 重新发现、成功入队并标记已下载。
	res, err = env.ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("check2 err: %v", err)
	}
	if res.New != 1 {
		t.Fatalf("check2 new=%d want 1（回滚后下轮应重见重试）", res.New)
	}
	if res.DownloadFailed != 0 {
		t.Fatalf("恢复后不应再计失败, got %d", res.DownloadFailed)
	}
	if got := *env.addCalls; got != 2 {
		t.Fatalf("check2 addCalls=%d want 2", got)
	}
	items, _ := env.itemOp.ListBySubscription(sub.ID, 10)
	if len(items) != 1 || !items[0].Downloaded {
		t.Fatalf("重试成功后应有 1 行 downloaded=true，got %+v", items)
	}
}

// TestRssAutoDownloadDedupAfterSuccess 成功路径：入库+入队+标记，二次检查零新增、不重复推种。
func TestRssAutoDownloadDedupAfterSuccess(t *testing.T) {
	env := newRssAutoDLEnv(t, rssAutoDLFeed, 0, true)
	dl, err := env.dlOp.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	sub := &models.RssSubscription{ID: 302, Name: "auto", URL: env.feedURL, AutoDownload: true, DownloaderID: dl.ID}
	if res, err := env.ch.Check(context.Background(), sub); err != nil || res.New != 1 {
		t.Fatalf("check1 new=%d err=%v want 1", res.New, err)
	}
	if res, err := env.ch.Check(context.Background(), sub); err != nil || res.New != 0 {
		t.Fatalf("check2 new=%d err=%v want 0（去重）", res.New, err)
	}
	if got := *env.addCalls; got != 1 {
		t.Fatalf("addCalls=%d want 1（重复推种）", got)
	}
	items, _ := env.itemOp.ListBySubscription(sub.ID, 10)
	if len(items) != 1 || !items[0].Downloaded {
		t.Fatalf("应有 1 行 downloaded=true，got %+v", items)
	}
}

// TestRssAutoDownloadNoDownloaderPending 无下载器时不静默吞掉：AutoDownload 已开但系统无下载器，
// 条目应回滚待配置下载器后重试，而不是落成「已见未下载」的永久死行。
func TestRssAutoDownloadNoDownloaderPending(t *testing.T) {
	env := newRssAutoDLEnv(t, rssAutoDLFeed, 0, false)
	sub := &models.RssSubscription{ID: 303, Name: "auto", URL: env.feedURL, AutoDownload: true}

	res, err := env.ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("check1 err: %v", err)
	}
	if res.New != 1 {
		t.Fatalf("check1 new=%d want 1（发现条数照常上报）", res.New)
	}
	if items, _ := env.itemOp.ListBySubscription(sub.ID, 10); len(items) != 0 {
		t.Fatalf("无下载器时应回滚待重试，got %d 行", len(items))
	}

	// 配置下载器后：下轮重见并成功入队。
	if _, err := env.dlOp.Create(oper.DownloaderInput{Name: "mock-qb", Type: "qb", URL: env.qbURL, IsDefault: true}); err != nil {
		t.Fatal(err)
	}

	res, err = env.ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("check2 err: %v", err)
	}
	if res.New != 1 {
		t.Fatalf("check2 new=%d want 1", res.New)
	}
	items, _ := env.itemOp.ListBySubscription(sub.ID, 10)
	if len(items) != 1 || !items[0].Downloaded {
		t.Fatalf("配置下载器后应入队成功，got %+v", items)
	}
}

// TestRssNonAutoDownloadKeepsRows 未开自动下载：条目照常入库浏览，去重生效（回滚逻辑不得误伤）。
func TestRssNonAutoDownloadKeepsRows(t *testing.T) {
	env := newRssAutoDLEnv(t, rssAutoDLFeed, 0, true)
	sub := &models.RssSubscription{ID: 304, Name: "browse", URL: env.feedURL}
	if res, err := env.ch.Check(context.Background(), sub); err != nil || res.New != 1 {
		t.Fatalf("check1 new=%d err=%v want 1", res.New, err)
	}
	items, _ := env.itemOp.ListBySubscription(sub.ID, 10)
	if len(items) != 1 || items[0].Downloaded {
		t.Fatalf("未开自动下载应保留 downloaded=false 行，got %+v", items)
	}
	if res, _ := env.ch.Check(context.Background(), sub); res.New != 0 {
		t.Fatalf("check2 new=%d want 0（去重）", res.New)
	}
	if got := *env.addCalls; got != 0 {
		t.Fatalf("未开自动下载不应推种，addCalls=%d", got)
	}
}
