package chain

// 辅种候选「跳过」与自动化的边界：cron 每轮 Reseed，返回值会被 AutoReseedAdd 逐条
// Resolve("add")。用户否决过的候选可以重新出现在列表里（既定「重新发现」语义，见
// TestReseedRediscoveryAfterSkip），但不得被交给自动添加——那等于自动化推翻显式否决。

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"gorm.io/gorm"
)

// newSkipHarness 假 IYUU（H1 → ourbits sid=7）+ fake 下载器 + 真跨站匹配链路。
func newSkipHarness(t *testing.T) (*KeepSeedChain, *gorm.DB, *fakeDL) {
	t.Helper()
	db := newReseedDB(t)
	crypto := security.NewCrypto("skip-seed")
	siteOp := oper.NewSiteOper(db, crypto)
	if _, err := siteOp.Create(oper.SiteInput{
		Name: "ourbits", Platform: "nexusphp", BaseURL: "https://ourbits.org", Passkey: "PK",
	}); err != nil {
		t.Fatal(err)
	}
	cfgOp := oper.NewSystemConfigOper(db)
	if err := cfgOp.Set(IYUUTokenKey, "TOKEN"); err != nil {
		t.Fatal(err)
	}
	ts := newReseedIYUU(t)
	t.Cleanup(ts.Close)
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeDL{torrents: []downloader.Torrent{{Hash: "H1", Name: "src torrent", SavePath: "/data/seed"}}}
	ch := NewKeepSeedChain(NewBase(module.New(), event.New()), nil, iyuu.New(hc, ts.URL), &fakeProvider{c: fake}, nil, siteOp, cfgOp, db, nil)
	return ch, db, fake
}

func TestReseedSkippedNotHandedToAutoAdd(t *testing.T) {
	ch, db, fake := newSkipHarness(t)
	ctx := context.Background()

	created, err := ch.Reseed(ctx, 1)
	if err != nil || len(created) != 1 {
		t.Fatalf("首轮应建 1 条候选: created=%d err=%v", len(created), err)
	}
	taskID := created[0].ID
	if err := ch.Resolve(ctx, taskID, "skip"); err != nil {
		t.Fatal(err)
	}

	// 下一轮 cron：候选仍被「重新发现」（行恢复 pending，用户在列表里能再决定），
	// 但不得出现在交给 AutoReseedAdd 的返回值里。
	again, err := ch.Reseed(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("用户跳过的候选被交给自动添加：%+v", again)
	}
	var row models.CrossSeedTask
	if err := db.First(&row, taskID).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != "pending" {
		t.Errorf("重新发现应把该行恢复成 pending 供用户再决定, got %q", row.Status)
	}
	// 模拟自动化：拿返回值逐条 add —— 否决过的种子绝不能进下载器
	for _, tk := range again {
		if err := ch.Resolve(ctx, tk.ID, "add"); err != nil {
			t.Fatalf("auto add: %v", err)
		}
	}
	if len(fake.added) != 0 {
		t.Errorf("跳过的种子被自动加进下载器, added=%d", len(fake.added))
	}

	// 正控：failed 行必须仍被交给自动重试（TestReseedRetryAfterResolveFailure 的既定能力）。
	if err := db.Model(&models.CrossSeedTask{}).Where("id = ?", taskID).
		Update("status", "failed").Error; err != nil {
		t.Fatal(err)
	}
	created3, err := ch.Reseed(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(created3) != 1 {
		t.Fatalf("failed 候选应交给自动重试, got %d 条", len(created3))
	}
	if err := ch.Resolve(ctx, created3[0].ID, "add"); err != nil {
		t.Fatal(err)
	}
	if len(fake.added) != 1 {
		t.Errorf("failed 重试应真正加进下载器, added=%d", len(fake.added))
	}
}

// TestUpsertCrossSeedTaskSkippedNotReturned 覆盖本地辅种与 Torznab 补候选共用的写入口。
func TestUpsertCrossSeedTaskSkippedNotReturned(t *testing.T) {
	db := newReseedDB(t)
	ch := &KeepSeedChain{db: db}
	row := models.CrossSeedTask{DownloaderID: 1, InfoHash: "H2", CandidateSite: "site-x", Status: "skipped"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	if _, ok := ch.upsertCrossSeedTask(1, 2, "H2", "src", "site-x", "http://site-x/dl?id=1", "/data", 90); ok {
		t.Error("跳过的候选不该出现在返回值里（会被自动添加）")
	}
	var after models.CrossSeedTask
	if err := db.First(&after, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != "pending" {
		t.Errorf("该行仍应被重新发现为 pending, got %q", after.Status)
	}

	// 正控：failed 行返回（自动重试）。
	if err := db.Model(&models.CrossSeedTask{}).Where("id = ?", row.ID).
		Update("status", "failed").Error; err != nil {
		t.Fatal(err)
	}
	got, ok := ch.upsertCrossSeedTask(1, 2, "H2", "src", "site-x", "http://site-x/dl?id=2", "/data", 90)
	if !ok || got.Status != "pending" {
		t.Errorf("failed 行应复活并交给自动重试, ok=%v status=%q", ok, got.Status)
	}
}
