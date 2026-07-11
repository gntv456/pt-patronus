package chain

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/media"
)

// fakeSrc 实现 MediaSource + TrendingCapable；记录 Trending 调用次数以验证缓存。
type fakeSrc struct {
	name  string
	items []media.MediaInfo
	calls int
}

func (f *fakeSrc) Name() string { return f.name }
func (f *fakeSrc) Search(context.Context, string, int) ([]media.MediaInfo, error) {
	return nil, nil
}
func (f *fakeSrc) Detail(context.Context, string) (*media.MediaInfo, error) { return nil, nil }
func (f *fakeSrc) Trending(context.Context, string, int) ([]media.MediaInfo, error) {
	f.calls++
	return f.items, nil
}

// plainSrc 仅实现 MediaSource（无 TrendingCapable）——应被 RecommendChain 跳过。
type plainSrc struct{ name string }

func (p *plainSrc) Name() string                                       { return p.name }
func (p *plainSrc) Search(context.Context, string, int) ([]media.MediaInfo, error) { return nil, nil }
func (p *plainSrc) Detail(context.Context, string) (*media.MediaInfo, error)        { return nil, nil }

// catSrc 在 fakeSrc 基础上声明 CategoryCapable：只支持 cats 里的分类。
type catSrc struct {
	fakeSrc
	cats []string
}

func (c *catSrc) Categories() []string { return c.cats }

func TestRecommendFeedsMergeAndCache(t *testing.T) {
	b := NewBase(module.New(), event.New())
	s1 := &fakeSrc{name: "s1", items: []media.MediaInfo{{Source: "s1", Title: "A"}}}
	s2 := &fakeSrc{name: "s2", items: []media.MediaInfo{{Source: "s2", Title: "B"}}}
	plain := &plainSrc{name: "plain"} // 无 Trending，应跳过
	rc := NewRecommendChain(b, &MediaSourceProvider{sources: []media.MediaSource{s1, plain, s2}})

	got := rc.RecommendFeeds(context.Background(), "", "movie", 1, false)
	if len(got) != 2 {
		t.Fatalf("expected 2 merged (plain skipped), got %d", len(got))
	}
	if s1.calls != 1 || s2.calls != 1 {
		t.Errorf("each trending source called once, got %d/%d", s1.calls, s2.calls)
	}

	// 二次调用命中内存缓存，不应再次调用源
	got2 := rc.RecommendFeeds(context.Background(), "", "movie", 1, false)
	if len(got2) != 2 {
		t.Fatalf("cached: expected 2, got %d", len(got2))
	}
	if s1.calls != 1 || s2.calls != 1 {
		t.Errorf("cache should skip source calls, got %d/%d", s1.calls, s2.calls)
	}

	// force=true 绕过缓存 → 源再次被调用
	rc.RecommendFeeds(context.Background(), "", "movie", 1, true)
	if s1.calls != 2 || s2.calls != 2 {
		t.Errorf("force should re-invoke, got %d/%d", s1.calls, s2.calls)
	}
}

// source 过滤：只取指定源。
func TestRecommendFeedsSourceFilter(t *testing.T) {
	b := NewBase(module.New(), event.New())
	s1 := &fakeSrc{name: "douban", items: []media.MediaInfo{{Title: "D"}}}
	s2 := &fakeSrc{name: "tmdb", items: []media.MediaInfo{{Title: "T"}}}
	rc := NewRecommendChain(b, &MediaSourceProvider{sources: []media.MediaSource{s1, s2}})

	got := rc.RecommendFeeds(context.Background(), "tmdb", "movie", 1, false)
	if len(got) != 1 || got[0].Title != "T" {
		t.Errorf("source filter = %+v", got)
	}
	if s1.calls != 0 {
		t.Errorf("douban should not be called when source=tmdb, got %d", s1.calls)
	}
}

// CategoryCapable 过滤：声明了 Categories() 的源在「不支持当前分类」时被跳过。
// 模拟「短剧」分类——只由星光集类源（仅声明 duanju）响应，豆瓣类源（声明 movie/tv）
// 不应被调用，避免回退默认热映污染短剧结果。
func TestRecommendFeedsCategoryFilter(t *testing.T) {
	b := NewBase(module.New(), event.New())
	xg := &catSrc{fakeSrc: fakeSrc{name: "xingguangji", items: []media.MediaInfo{{Source: "xingguangji", Title: "短剧A"}}}, cats: []string{"duanju"}}
	db := &catSrc{fakeSrc: fakeSrc{name: "douban", items: []media.MediaInfo{{Source: "douban", Title: "电影B"}}}, cats: []string{"movie", "coming", "tv"}}
	rc := NewRecommendChain(b, &MediaSourceProvider{sources: []media.MediaSource{xg, db}})

	// 「短剧」分类：只取星光集，豆瓣被跳过
	got := rc.RecommendFeeds(context.Background(), "", "duanju", 1, false)
	if len(got) != 1 || got[0].Title != "短剧A" {
		t.Fatalf("duanju: expected only xingguangji, got %+v", got)
	}
	if db.calls != 0 {
		t.Errorf("duanju: douban (declares movie/tv) should be skipped, called %d", db.calls)
	}

	// 反向：movie 分类时，只声明 duanju 的星光集被跳过
	got2 := rc.RecommendFeeds(context.Background(), "", "movie", 1, false)
	if len(got2) != 1 || got[0].Title == got2[0].Title {
		t.Fatalf("movie: expected douban only, got %+v", got2)
	}
	if xg.calls != 1 {
		t.Errorf("movie: xingguangji should be skipped (still called once for duanju above), calls=%d", xg.calls)
	}
}

// blockingSrc 实现 MediaSource + TrendingCapable；Trending 阻塞在 proceed 上、用 atomic 计数，
// 用于验证 singleflight 合并同 key 并发回源（回源只发生一次）。跟随 entertainment_test.go 的 atomic 风格。
type blockingSrc struct {
	name    string
	items   []media.MediaInfo
	calls   int32
	proceed chan struct{}
}

func (b *blockingSrc) Name() string { return b.name }
func (b *blockingSrc) Search(context.Context, string, int) ([]media.MediaInfo, error) {
	return nil, nil
}
func (b *blockingSrc) Detail(context.Context, string) (*media.MediaInfo, error) { return nil, nil }
func (b *blockingSrc) Trending(context.Context, string, int) ([]media.MediaInfo, error) {
	atomic.AddInt32(&b.calls, 1)
	<-b.proceed
	return b.items, nil
}

// TestRecommendFeedsConcurrentDedup 验证 singleflight：N 个 goroutine 同时请求同一 key，
// Trending 只被调用一次（其余共享 leader 结果）。若无 singleflight，calls 会涨到接近 N。
func TestRecommendFeedsConcurrentDedup(t *testing.T) {
	b := NewBase(module.New(), event.New())
	src := &blockingSrc{
		name:    "s1",
		items:   []media.MediaInfo{{Source: "s1", Title: "A"}},
		proceed: make(chan struct{}),
	}
	rc := NewRecommendChain(b, &MediaSourceProvider{sources: []media.MediaSource{src}})

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([][]media.MediaInfo, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = rc.RecommendFeeds(context.Background(), "", "movie", 1, false)
		}(i)
	}
	close(start)

	// 等 leader 进入 Trending（calls==1 且稳定），证明并发被合并到一次回源。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.calls) >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	// 让一让让 waiter 们有机会「溜过去」直接调 Trending（若无 singleflight）。
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Fatalf("expected exactly 1 in-flight Trending call (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("after completion: expected 1 Trending call, got %d", got)
	}
	for i, r := range results {
		if len(r) != 1 || r[0].Title != "A" {
			t.Errorf("result[%d] = %+v, want one item A", i, r)
		}
	}

	// 缓存已写：再次调用应命中缓存，不触发回源。
	rc.RecommendFeeds(context.Background(), "", "movie", 1, false)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("cached call should not re-fetch, calls=%d", got)
	}
}
