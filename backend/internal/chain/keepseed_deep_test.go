// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"github.com/hxpt/ptpatronus/internal/keepseed"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// failingDL：AddTorrent 恒失败，用于把 Resolve(add) 推到 status=failed 路径。
type failingDL struct{ torrents []downloader.Torrent }

func (f *failingDL) ListTorrents(ctx context.Context) ([]downloader.Torrent, error) {
	return f.torrents, nil
}
func (f *failingDL) AddTorrent(ctx context.Context, _ downloader.AddOptions) error {
	return errors.New("add boom")
}

type failingProvider struct{ c *failingDL }

func (p *failingProvider) Get(ctx context.Context, _ uint) (reseedClient, error) { return p.c, nil }

// reseedHarness 复用 keepseed_test.go 的脚手架，装好 ourbits 站 + IYUU fake + 指定下载器。
func reseedHarness(t *testing.T, dl dlProvider) (*KeepSeedChain, *oper.SiteOper) {
	t.Helper()
	db := newReseedDB(t)
	crypto := security.NewCrypto("seed")
	siteOp := oper.NewSiteOper(db, crypto)
	cfgOp := oper.NewSystemConfigOper(db)
	if _, err := siteOp.Create(oper.SiteInput{Name: "ourbits", Platform: "nexusphp", BaseURL: "https://ourbits.org", Passkey: "PK"}); err != nil {
		t.Fatal(err)
	}
	if err := cfgOp.Set(IYUUTokenKey, "TOKEN"); err != nil {
		t.Fatal(err)
	}
	ts := newReseedIYUU(t)
	t.Cleanup(ts.Close)
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := NewKeepSeedChain(NewBase(module.New(), event.New()), nil, iyuu.New(hc, ts.URL), dl, nil, siteOp, cfgOp, db, nil)
	return ch, siteOp
}

// TestReseedRediscoveryAfterSkip 复现缺陷：辅种候选被用户 skip 后，再次 Reseed 无法重新发现。
//
// 正确行为：用户改主意场景下，skipped 候选应可被重新发现为 pending。
// 根因：CrossSeedTask 唯一索引 (info_hash, candidate_site) 不含 status；Reseed 用
//
//	OnConflict{DoNothing}，而 taskExists 只查 pending/added → skipped 行永久阻塞候选重建。
//
// 预期：本测试在未修复时 FAIL（again=0），修复后 PASS。
func TestReseedRediscoveryAfterSkip(t *testing.T) {
	fake := &fakeDL{torrents: []downloader.Torrent{{Hash: "H1", Name: "src", SavePath: "/data/seed"}}}
	ch, _ := reseedHarness(t, &fakeProvider{c: fake})

	created, err := ch.Reseed(context.Background(), 1)
	if err != nil {
		t.Fatalf("Reseed: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("want 1 initial candidate, got %d", len(created))
	}
	if err := ch.Resolve(context.Background(), created[0].ID, "skip"); err != nil {
		t.Fatalf("Resolve skip: %v", err)
	}

	again, err := ch.Reseed(context.Background(), 1)
	if err != nil {
		t.Fatalf("Reseed again: %v", err)
	}
	// 缺陷①：skipped 行阻塞候选复活 → DB 中无可用 pending（用户改主意后永远看不到该候选）
	var pending int64
	ch.db.Model(&models.CrossSeedTask{}).Where("status = ?", "pending").Count(&pending)
	if pending != 1 {
		t.Errorf("skip 后应重新出现 pending 候选 (got %d) — 缺陷: skipped 行永久阻塞候选复活", pending)
	}
	// 缺陷②：OnConflict{DoNothing} 冲突不插入但仍 append → 返回 ID=0 幽灵候选
	for _, tk := range again {
		if tk.ID == 0 {
			t.Errorf("Reseed 返回幽灵候选 ID=0 (未实际入库) — 缺陷: 冲突未插入仍计入返回值，自动辅种会拿 ID=0 去 Resolve")
		}
	}
}

// TestReseedRetryAfterResolveFailure 复现缺陷：自动辅种场景下 Resolve(add) 失败 → status=failed，
// 下次 cron Reseed 应重新发现候选重试；当前实现 failed 行被 OnConflict{DoNothing} 阻塞 → 永不重试。
func TestReseedRetryAfterResolveFailure(t *testing.T) {
	fp := &failingProvider{c: &failingDL{torrents: []downloader.Torrent{{Hash: "H1", Name: "src", SavePath: "/d"}}}}
	ch, _ := reseedHarness(t, fp)

	created, err := ch.Reseed(context.Background(), 1)
	if err != nil || len(created) != 1 {
		t.Fatalf("initial Reseed: len=%d err=%v", len(created), err)
	}
	if err := ch.Resolve(context.Background(), created[0].ID, "add"); err == nil {
		t.Fatal("Resolve add 应失败（failingDL）")
	}
	var tk models.CrossSeedTask
	if err := ch.db.First(&tk, created[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if tk.Status != "failed" {
		t.Fatalf("status=%q want failed", tk.Status)
	}

	ch.Reseed(context.Background(), 1)
	var pending int64
	ch.db.Model(&models.CrossSeedTask{}).Where("status = ?", "pending").Count(&pending)
	if pending != 1 {
		t.Errorf("failed 后应重新出现 pending 重试 (got %d) — 缺陷: failed 行阻塞候选复活，自动辅种永不重试失败任务", pending)
	}
}

// TestScoreBoundary 精确评分边界（标题 Jaccard 70% + 大小接近度 30%）。
func TestScoreBoundary(t *testing.T) {
	cases := []struct {
		name              string
		wantName, gotName string
		wantSize, gotSize int64
		want              int
	}{
		{"identical", "Movie 2023", "Movie 2023", 1000, 1000, 100},
		{"size within 5pct", "Movie 2023", "Movie 2023", 1000, 950, 97},     // ratio=0.05 → sizeScore=90（≤0.05 档）；title=100 → 70+27=97
		{"size diff over 50pct", "Movie 2023", "Movie 2023", 1000, 100, 70}, // sizeScore=0 → 100*0.7
		{"unrelated title same size", "Alpha", "Beta", 1000, 1000, 30},      // title=0,size=100 → 30
		{"wantSize zero title match", "Movie 2023", "Movie 2023", 0, 0, 100},
		{"empty want", "", "x", 1, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := keepseed.Score(tc.wantName, tc.gotName, tc.wantSize, tc.gotSize); got != tc.want {
				t.Errorf("Score = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestMatchSiteByHostSuffixFallback 后缀回退匹配：iyuuHost=裸域时回退匹配子域站，
// 多个同后缀站时返回首个（确定性）。注意：这是潜在误配点——不同站共享域名后缀时可能错配。
func TestMatchSiteByHostSuffixFallback(t *testing.T) {
	// 后缀回退：裸域匹配第一个子域站
	sites := []models.Site{
		{ID: 1, Name: "a", BaseURL: "https://a.example.com"},
		{ID: 2, Name: "b", BaseURL: "https://b.example.com"},
	}
	got := matchSiteByHost(sites, "example.com")
	if got == nil || got.ID != 1 {
		t.Errorf("suffix fallback want site 1, got %v", got)
	}
	// 精确匹配优先于后缀回退
	sites2 := []models.Site{
		{ID: 1, BaseURL: "https://example.com"},
		{ID: 2, BaseURL: "https://sub.example.com"},
	}
	got2 := matchSiteByHost(sites2, "example.com")
	if got2 == nil || got2.ID != 1 {
		t.Errorf("exact match want site 1, got %v", got2)
	}
	// 无任何匹配
	if matchSiteByHost(sites, "nomatch.org") != nil {
		t.Error("should not match unrelated host")
	}
}
