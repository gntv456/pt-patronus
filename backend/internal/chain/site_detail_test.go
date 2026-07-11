package chain

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// fakeDetailAdapter 在 fakeAdapter 基础上加 TorrentDetailCapable；FetchTorrentDetail 阻塞在
// proceed 上、用 atomic 计数，验证 singleflight 合并同 key 并发抓取（回源只发生一次）。
type fakeDetailAdapter struct {
	*fakeAdapter
	detail  *site.TorrentDetail
	calls   int32
	proceed chan struct{}
}

func (f *fakeDetailAdapter) FetchTorrentDetail(context.Context, *site.SiteMetadata, string, string, string) (*site.TorrentDetail, error) {
	atomic.AddInt32(&f.calls, 1)
	<-f.proceed
	return f.detail, nil
}

// TestTorrentDetailConcurrentDedup 验证 singleflight：N 个 goroutine 同时请求同一详情 URL，
// FetchTorrentDetail 只被调用一次（其余共享 leader 结果）。若无 singleflight，calls 会涨到接近 N。
func TestTorrentDetailConcurrentDedup(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://example.com"})

	src := &fakeDetailAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		detail:      &site.TorrentDetail{Title: "A"},
		proceed:     make(chan struct{}),
	}
	mm := module.New()
	mm.Register(src)
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]*site.TorrentDetail, n)
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = ch.TorrentDetail(context.Background(), s, "https://example.com/details.php?id=1")
		}(i)
	}
	close(start)

	// 等 leader 进入 FetchTorrentDetail（calls==1），证明并发被合并到一次抓取。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.calls) >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	// 让一让让 waiter 们有机会「溜过去」直接调（若无 singleflight）。
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Fatalf("expected exactly 1 in-flight FetchTorrentDetail (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("after completion: expected 1 call, got %d", got)
	}
	for i, e := range errs {
		if e != nil {
			t.Errorf("result[%d] err=%v", i, e)
		}
	}
	for i, r := range results {
		if r == nil || r.Title != "A" {
			t.Errorf("result[%d] = %+v, want Title A", i, r)
		}
	}

	// 缓存已写：再次调用应命中缓存，不触发抓取。
	if _, err := ch.TorrentDetail(context.Background(), s, "https://example.com/details.php?id=1"); err != nil {
		t.Errorf("cached call err=%v", err)
	}
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("cached call should not re-fetch, calls=%d", got)
	}
}

// TestTorrentDetailSameHostGuard 确认 singleflight 不会绕过 sameHost 授权守卫
// （跨站 detailURL 应直接拒绝，不进回源）。
func TestTorrentDetailSameHostGuard(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://example.com"})
	src := &fakeDetailAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		detail:      &site.TorrentDetail{Title: "A"},
		proceed:     make(chan struct{}),
	}
	mm := module.New()
	mm.Register(src)
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	_, err := ch.TorrentDetail(context.Background(), s, "https://evil.com/details.php?id=1")
	if err == nil {
		t.Fatal("cross-site detailURL should be rejected by sameHost guard")
	}
	if got := atomic.LoadInt32(&src.calls); got != 0 {
		t.Errorf("sameHost rejection must not reach fetch, calls=%d", got)
	}
}
