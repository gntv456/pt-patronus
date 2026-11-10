// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// 订阅追更端到端链路：真 SiteChain（假适配器搜种）+ 真 DownloadChain（假 qB）+ 真订阅表，
// 跑 checkWithDefaults 验证「搜到 → 洗版门控 → 推种 → 记命中」整条路。
// 起因：电影订阅勾了「优先最佳版本」但未配过滤规则组时，候选 PriOrder 全为 0、
// CurrentPriority 也是 0，旧门控 `pri <= CurrentPriority` 把每条候选都拦掉——
// 订阅一条都不下、也不通知，功能静默失效（剧集侧用「未记录过→放行」没这问题）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

type subscribeE2E struct {
	ch       *SubscribeChain
	subOp    *oper.SubscriptionOper
	dlOp     *oper.DownloaderOper
	db       *gorm.DB
	addCalls *int32
	adp      *fakeAdapter // 测试可在轮次之间改写 adp.torrents（如模拟凭据轮换）
}

func newSubscribeE2E(t *testing.T, torrents []site.Torrent) *subscribeE2E {
	t.Helper()
	var addCalls int32
	qbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(qbSrv.Close)

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sub-e2e.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Subscription{}, &models.SubscriptionHit{},
		&models.Downloader{}, &models.Site{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}

	crypto := security.NewCrypto("sub-e2e-seed")
	siteOp := oper.NewSiteOper(db, crypto)
	if _, err := siteOp.Create(oper.SiteInput{
		Name: "n", Platform: "nexusphp", BaseURL: "https://n.example", IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	dlOp := oper.NewDownloaderOper(db, crypto)
	if _, err := dlOp.Create(oper.DownloaderInput{
		Name: "qb", Type: "qb", URL: qbSrv.URL, Username: "u", Password: "p", IsDefault: true,
	}); err != nil {
		t.Fatal(err)
	}

	adp := &fakeAdapter{schema: "nexusphp", torrents: torrents}
	mm := module.New()
	mm.Register(adp)
	base := NewBase(mm, event.New())
	siteCh := NewSiteChain(base, siteOp, site.NewRegistry())
	dlCh := NewDownloadChain(base, manager.NewManager(dlOp), dlOp, nil)
	subOp := oper.NewSubscriptionOper(db)
	ch := NewSubscribeChain(siteCh, dlCh, dlOp, subOp, nil, nil, nil, nil)
	return &subscribeE2E{ch: ch, subOp: subOp, dlOp: dlOp, db: db, addCalls: &addCalls, adp: adp}
}

func movieTorrent(name, hash string) site.Torrent {
	return site.Torrent{
		Title:       name,
		DownloadURL: "magnet:?xt=urn:btih:" + hash,
		SiteName:    "n",
	}
}

// TestSubscribeMovieBestVersionWithoutRules 未配规则组的洗版电影必须下到首版（旧行为：0 条）。
func TestSubscribeMovieBestVersionWithoutRules(t *testing.T) {
	env := newSubscribeE2E(t, []site.Torrent{
		movieTorrent("Some.Movie.2024.1080p.WEB-DL-GROUP", "aaaa1111"),
		movieTorrent("Some.Movie.2024.720p.WEB-DL-GROUP", "bbbb2222"),
	})
	truth := true
	dl, err := env.dlOp.List()
	if err != nil || len(dl) == 0 {
		t.Fatalf("downloader: %v err=%v", dl, err)
	}
	sub := &models.Subscription{
		Source: "tmdb", SourceID: "m:9", Title: "Some Movie", Type: "movie", Category: "movie",
		Status: "subscribed", BestVersion: &truth, DownloaderID: dl[0].ID,
	}
	if err := env.subOp.Create(sub); err != nil {
		t.Fatal(err)
	}

	pushed, err := env.ch.checkWithDefaults(context.Background(), sub, GlobalDefaults{})
	if err != nil {
		t.Fatalf("checkWithDefaults: %v", err)
	}
	if pushed != 1 {
		t.Errorf("未配规则组的洗版电影应下到首版 1 条, pushed=%d（旧行为：门控把全部候选拦掉→0）", pushed)
	}
	if got := atomic.LoadInt32(env.addCalls); got != 1 {
		t.Errorf("推给下载器次数=%d want 1", got)
	}
	if ok, err := env.subOp.HasAnyHit(sub.ID); err != nil || !ok {
		t.Errorf("应记到命中行（下轮去重依据）: %v err=%v", ok, err)
	}

	// 第二轮：同站又出现另一个 0 优先级版本——无画质信息且已有基线，不得再下（防多站重复下）。
	again, err := env.subOp.Get(sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	pushed2, err := env.ch.checkWithDefaults(context.Background(), again, GlobalDefaults{})
	if err != nil {
		t.Fatalf("checkWithDefaults round2: %v", err)
	}
	if pushed2 != 0 {
		t.Errorf("已有基线时不应重复推送, pushed=%d", pushed2)
	}
	if got := atomic.LoadInt32(env.addCalls); got != 1 {
		t.Errorf("推给下载器次数应仍为 1, got %d", got)
	}
}

// TestSubscribeMoviePlainPushesOnce 正控：未开洗版的电影订阅照常首版即完成（链路其它环节未坏）。
func TestSubscribeMoviePlainPushesOnce(t *testing.T) {
	env := newSubscribeE2E(t, []site.Torrent{movieTorrent("Some.Movie.2024.1080p.WEB-DL-GROUP", "cccc3333")})
	falseB := false
	dl, err := env.dlOp.List()
	if err != nil || len(dl) == 0 {
		t.Fatalf("downloader: %v err=%v", dl, err)
	}
	sub := &models.Subscription{
		Source: "tmdb", SourceID: "m:8", Title: "Some Movie", Type: "movie", Category: "movie",
		Status: "subscribed", BestVersion: &falseB, DownloaderID: dl[0].ID,
	}
	if err := env.subOp.Create(sub); err != nil {
		t.Fatal(err)
	}
	pushed, err := env.ch.checkWithDefaults(context.Background(), sub, GlobalDefaults{})
	if err != nil {
		t.Fatalf("checkWithDefaults: %v", err)
	}
	if pushed != 1 || atomic.LoadInt32(env.addCalls) != 1 {
		t.Fatalf("非洗版电影应推 1 条, pushed=%d adds=%d", pushed, atomic.LoadInt32(env.addCalls))
	}
	got, err := env.subOp.Get(sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" {
		t.Errorf("非洗版电影推过即应 completed, got %q", got.Status)
	}
}

// TestTorrentKeyStableAcrossCredentialRotation 去重键只看种子身份，不看会轮换的凭据。
// 旧行为是 sha1(整条 URL)：Gazelle(MTeam 系) 的 authkey/torrent_pass、NexusPHP 的
// passkey/hash 一变，同一颗种子就成了「没推过的新种子」。
func TestTorrentKeyStableAcrossCredentialRotation(t *testing.T) {
	same := [][2]string{
		{"https://m.example/torrents.php?action=download&id=42&authkey=OLD&torrent_pass=PA",
			"https://m.example/torrents.php?action=download&id=42&authkey=NEW&torrent_pass=PB"},
		{"https://n.example/download.php?id=7&passkey=AAA",
			"https://n.example/download.php?id=7&passkey=BBB"},
		{"https://n.example/download.php?hash=H1&id=7",
			"https://n.example/download.php?id=7&hash=H2"},
		{"https://N.Example/download.php?id=7&passkey=A",
			"https://n.example/download.php?id=7"},
		{"magnet:?xt=urn:btih:AAAA1111&authkey=OLD",
			"magnet:?xt=urn:btih:AAAA1111&authkey=NEW"},
	}
	for i, pair := range same {
		if a, b := torrentKey(pair[0]), torrentKey(pair[1]); a != b {
			t.Errorf("case %d: 同种子换凭据应同键, got %q vs %q\n\t%s\n\t%s", i, a, b, pair[0], pair[1])
		}
	}
	// 正控：归一化不能把不同种子并成一个键。
	diff := [][2]string{
		{"https://n.example/download.php?id=7&passkey=A", "https://n.example/download.php?id=8&passkey=A"},
		{"https://a.example/download.php?id=7", "https://b.example/download.php?id=7"},
		{"https://a.example/torrents.php?action=download&id=7", "https://a.example/torrents.php?action=report&id=7"},
		{"magnet:?xt=urn:btih:AAAA1111", "magnet:?xt=urn:btih:BBBB2222"},
	}
	for i, pair := range diff {
		if a, b := torrentKey(pair[0]), torrentKey(pair[1]); a == b {
			t.Errorf("case %d: 不同种子被并成同一键 %q\n\t%s\n\t%s", i, a, pair[0], pair[1])
		}
	}
}

// TestSubscribeNoRepushAfterCredentialRotation 端到端：凭据轮换（等价于进程重启后重新取
// authkey）之后，已推过的种子必须仍被识别为「推过」。否则每轮都会重推：下载器里还留着的
// 只是刷日志/再发一条命中通知，用户做完种删掉的则会被真的重新下载。
func TestSubscribeNoRepushAfterCredentialRotation(t *testing.T) {
	env := newSubscribeE2E(t, []site.Torrent{{
		Title:       "Some.Movie.2024.1080p.WEB-DL-GROUP",
		DownloadURL: "magnet:?xt=urn:btih:CCCC3333&authkey=OLD&torrent_pass=P1",
		SiteName:    "n",
	}})
	dl, err := env.dlOp.List()
	if err != nil || len(dl) == 0 {
		t.Fatalf("downloader: %v err=%v", dl, err)
	}
	sub := &models.Subscription{
		Source: "tmdb", SourceID: "m:7", Title: "Some Movie", Type: "movie", Category: "movie",
		Status: "subscribed", DownloaderID: dl[0].ID,
	}
	if err := env.subOp.Create(sub); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	pushed, err := env.ch.checkWithDefaults(ctx, sub, GlobalDefaults{})
	if err != nil {
		t.Fatalf("round1: %v", err)
	}
	if pushed != 1 || atomic.LoadInt32(env.addCalls) != 1 {
		t.Fatalf("round1 应推 1 条, pushed=%d adds=%d", pushed, atomic.LoadInt32(env.addCalls))
	}

	// 同一颗种子、换了凭据（MTeam 重启后 action=index 拿到的就是新 authkey/torrent_pass）。
	env.adp.torrents = []site.Torrent{{
		Title:       "Some.Movie.2024.1080p.WEB-DL-GROUP",
		DownloadURL: "magnet:?xt=urn:btih:CCCC3333&authkey=NEW&torrent_pass=P2",
		SiteName:    "n",
	}}
	again, err := env.subOp.Get(sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	pushed2, err := env.ch.checkWithDefaults(ctx, again, GlobalDefaults{})
	if err != nil {
		t.Fatalf("round2: %v", err)
	}
	if pushed2 != 0 {
		t.Errorf("凭据轮换后被当成新种子重推：pushed=%d want 0（旧行为：去重键=整条 URL 哈希）", pushed2)
	}
	if got := atomic.LoadInt32(env.addCalls); got != 1 {
		t.Errorf("推给下载器次数应仍为 1, got %d", got)
	}
}
