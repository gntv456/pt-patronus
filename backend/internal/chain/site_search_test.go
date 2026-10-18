package chain

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// createActive 建一个启用站点，忽略返回值（失败会让后续断言暴露）。
func createActive(t *testing.T, siteOp *oper.SiteOper, name, platform string) {
	t.Helper()
	if _, err := siteOp.Create(oper.SiteInput{
		Name: name, Platform: platform, BaseURL: "https://" + name, IsActive: true,
	}); err != nil {
		t.Fatalf("create site %s: %v", name, err)
	}
}

// TestSiteChainSearchSitesConcurrent 验证跨站搜索并发执行（耗时 ≈ 单站而非 N×单站），
// 且单站失败（mteam 适配器返回 err）不影响其它站点结果。
func TestSiteChainSearchSitesConcurrent(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	for _, n := range []string{"n0", "n1", "n2", "n3"} {
		createActive(t, siteOp, n, "nexusphp")
	}
	createActive(t, siteOp, "m", "mteam")

	mm := module.New()
	mm.Register(
		&fakeAdapter{schema: "nexusphp", delay: 100 * time.Millisecond, torrents: []site.Torrent{{Title: "ok", Seeders: 1}}},
		&fakeAdapter{schema: "mteam", err: errors.New("boom")},
	)
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	start := time.Now()
	results, _ := ch.SearchSites(context.Background(), "kw", "", nil)
	elapsed := time.Since(start)

	// 5 站 × 100ms，并发应远小于串行 500ms；阈值留 4× 余量防 CI 抖动。
	if elapsed >= 400*time.Millisecond {
		t.Errorf("SearchSites not concurrent: elapsed=%v (want <400ms for 5×100ms parallel)", elapsed)
	}
	if len(results) != 5 {
		t.Fatalf("results len=%d want 5", len(results))
	}
	ok, fail := 0, 0
	for _, r := range results {
		if r.Error != "" {
			fail++
			continue
		}
		if len(r.Torrents) != 1 {
			t.Errorf("site %s: want 1 torrent, got %d", r.SiteName, len(r.Torrents))
		}
		ok++
	}
	if ok != 4 || fail != 1 {
		t.Errorf("ok=%d fail=%d want ok=4 fail=1 (mteam err isolated)", ok, fail)
	}
}

// TestSiteChainSearchSitesTimeout 验证搜索尊重 context 超时：一个 hang 住的站点
// 被取消并记为 error，不阻塞整体返回。
func TestSiteChainSearchSitesTimeout(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	createActive(t, siteOp, "fast", "nexusphp")
	createActive(t, siteOp, "slow", "mteam")

	mm := module.New()
	mm.Register(
		&fakeAdapter{schema: "nexusphp", torrents: []site.Torrent{{Title: "f"}}},
		&fakeAdapter{schema: "mteam", delay: 5 * time.Second},
	)
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	results, _ := ch.SearchSites(ctx, "kw", "", nil)
	elapsed := time.Since(start)

	if elapsed >= 1*time.Second {
		t.Errorf("SearchSites blocked past timeout: elapsed=%v", elapsed)
	}
	if len(results) != 2 {
		t.Fatalf("results len=%d want 2", len(results))
	}
	var slowErr string
	for _, r := range results {
		if r.SiteName == "slow" {
			slowErr = r.Error
		}
	}
	if slowErr == "" {
		t.Errorf("slow site should record error after ctx timeout, got empty")
	}
}

// TestSiteChainSearchSitesStream 验证流式版与 SearchSites 行为一致：每站完成即回调一次
// （含失败站），且并发执行（耗时 ≈ 单站而非 N×单站）。
func TestSiteChainSearchSitesStream(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	for _, n := range []string{"n0", "n1", "n2", "n3"} {
		createActive(t, siteOp, n, "nexusphp")
	}
	createActive(t, siteOp, "m", "mteam")

	mm := module.New()
	mm.Register(
		&fakeAdapter{schema: "nexusphp", delay: 100 * time.Millisecond, torrents: []site.Torrent{{Title: "ok", Seeders: 1}}},
		&fakeAdapter{schema: "mteam", err: errors.New("boom")},
	)
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	// emit 被多个 goroutine 并发调用，收集需加锁（handler 层的 beginSSE emitter 自身线程安全）。
	var mu sync.Mutex
	var got []SiteResult
	start := time.Now()
	ch.SearchSitesStream(context.Background(), "kw", "", nil, func(r SiteResult) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
	})
	elapsed := time.Since(start)

	if elapsed >= 400*time.Millisecond {
		t.Errorf("SearchSitesStream not concurrent: elapsed=%v (want <400ms for 5×100ms parallel)", elapsed)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 5 {
		t.Fatalf("emit count=%d want 5", len(got))
	}
	ok, fail := 0, 0
	for _, r := range got {
		if r.Error != "" {
			fail++
			continue
		}
		if len(r.Torrents) != 1 {
			t.Errorf("site %s: want 1 torrent, got %d", r.SiteName, len(r.Torrents))
		}
		ok++
	}
	if ok != 4 || fail != 1 {
		t.Errorf("ok=%d fail=%d want ok=4 fail=1 (mteam err isolated)", ok, fail)
	}
}

// TestSearchListFailurePropagates 验证站点清单读失败必须交回错误（JSON errcode / SSE done 帧
// list_error），不能渲染成「全部站无结果」假绿。
func TestSearchListFailurePropagates(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := siteOp.ListActive(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListActive 仍成功")
	}
	ch := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())

	if _, err := ch.SearchSites(context.Background(), "kw", "", nil); err == nil {
		t.Error("SearchSites：ListActive 失败应交回错误，而不是 nil+nil")
	}
	emitted := 0
	if listErr := ch.SearchSitesStream(context.Background(), "kw", "", nil, func(SiteResult) { emitted++ }); listErr == "" {
		t.Error("SearchSitesStream：ListActive 失败应返回 listErr，供 done 帧带出")
	}
	if emitted != 0 {
		t.Errorf("失败时不应 emit 任何帧，emitted=%d", emitted)
	}
}
