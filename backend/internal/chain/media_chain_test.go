package chain

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/media"
	"gorm.io/gorm"
)

// mediaFake 测试用媒体源：按 delay 睡眠后返回 items（模拟慢/快源），实现 TypedSource。
type mediaFake struct {
	name   string
	delay  time.Duration
	items  []media.MediaInfo
	types  []media.MediaType
	called bool
}

func (f *mediaFake) Name() string                                             { return f.name }
func (f *mediaFake) Types() []media.MediaType                                 { return f.types }
func (f *mediaFake) Detail(context.Context, string) (*media.MediaInfo, error) { return nil, nil }
func (f *mediaFake) Search(ctx context.Context, _ string, _ int) ([]media.MediaInfo, error) {
	f.called = true
	select {
	case <-time.After(f.delay):
		return f.items, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// 证明「并发 + 预算」：慢源(5s)被 context 在预算内砍掉，快源(20ms)正常返回，
// 总时长 ≈ 预算而非各源累加。这是修「串行跑出 31s 触发前端 15s 超时」的关键回归保护。
func TestMediaChainSearchConcurrentBudget(t *testing.T) {
	fast := &mediaFake{name: "fast", delay: 20 * time.Millisecond, types: []media.MediaType{media.TypeMovie},
		items: []media.MediaInfo{{Source: "fast", Title: "A", Type: media.TypeMovie}}}
	slow := &mediaFake{name: "slow", delay: 5 * time.Second, types: []media.MediaType{media.TypeMovie},
		items: []media.MediaInfo{{Source: "slow", Title: "B", Type: media.TypeMovie}}}
	c := NewMediaChain(Base{}, &MediaSourceProvider{sources: []media.MediaSource{fast, slow}}, nil, nil) // nil cache, nil cfg

	// 父 ctx 给 300ms 短预算；Search 内部再套 8s → 有效 deadline = 300ms。
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	res, err := c.Search(ctx, "kw", 0, "video")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if elapsed > 1*time.Second {
		t.Errorf("并发+预算应 ~300ms 而非 5s 累加，实际 %v", elapsed)
	}
	if len(res) != 1 || res[0].Title != "A" {
		t.Errorf("慢源被砍、应只含快源结果，got %+v", res)
	}
}

// 证明按 TypedSource 跳过：video 搜时只产音乐的源（MusicBrainz）不被调用，结果为空。
func TestMediaChainSearchSkipsMusicForVideo(t *testing.T) {
	mb := &mediaFake{name: "mb", types: []media.MediaType{media.TypeMusic},
		items: []media.MediaInfo{{Source: "mb", Title: "song", Type: media.TypeMusic}}}
	c := NewMediaChain(Base{}, &MediaSourceProvider{sources: []media.MediaSource{mb}}, nil, nil)

	res, err := c.Search(context.Background(), "kw", 0, "video")
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if mb.called {
		t.Error("只产音乐的源在 video 搜时不应被调用")
	}
	if len(res) != 0 {
		t.Errorf("video 搜跳过音乐源后应无结果，got %+v", res)
	}
}

// newMediaDB 在 newChainDB 基础上追加 migrate PosterCache（Media Search 缓存表）。
func newMediaDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.PosterCache{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// blockingMedia 实现 media.MediaSource + TypedSource；Search 阻塞在 proceed 上、用 atomic 计数，
// 验证 singleflight 合并同 key 并发跨源搜索（回源只发生一次）。
type blockingMedia struct {
	name    string
	items   []media.MediaInfo
	types   []media.MediaType
	calls   int32
	proceed chan struct{}
}

func (b *blockingMedia) Name() string             { return b.name }
func (b *blockingMedia) Types() []media.MediaType { return b.types }
func (b *blockingMedia) Detail(context.Context, string) (*media.MediaInfo, error) {
	return nil, nil
}
func (b *blockingMedia) Search(context.Context, string, int) ([]media.MediaInfo, error) {
	atomic.AddInt32(&b.calls, 1)
	<-b.proceed
	return b.items, nil
}

// TestMediaSearchConcurrentDedup 验证 singleflight：N 个 goroutine 同时搜同一关键词，
// 源 Search 只被调用一次（其余共享 leader 结果），DB 缓存只写一次。
func TestMediaSearchConcurrentDedup(t *testing.T) {
	cache := oper.NewPosterCacheOper(newMediaDB(t))
	src := &blockingMedia{
		name:    "s1",
		items:   []media.MediaInfo{{Source: "s1", Title: "A", Type: media.TypeMovie}},
		types:   []media.MediaType{media.TypeMovie},
		proceed: make(chan struct{}),
	}
	c := NewMediaChain(Base{}, &MediaSourceProvider{sources: []media.MediaSource{src}}, cache, nil)

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([][]media.MediaInfo, n)
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = c.Search(context.Background(), "kw", 0, "video")
		}(i)
	}
	close(start)

	// 等 leader 进入源 Search（calls==1），证明并发被合并到一次回源。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.calls) >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Fatalf("expected exactly 1 in-flight source Search (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("after completion: expected 1 source call, got %d", got)
	}
	for i, e := range errs {
		if e != nil {
			t.Errorf("result[%d] err=%v", i, e)
		}
	}
	for i, r := range results {
		if len(r) != 1 || r[0].Title != "A" {
			t.Errorf("result[%d] = %+v, want one item A", i, r)
		}
	}

	// leader 只 Put 一次：DB PosterCache 该 key 应能命中。
	key := oper.CacheKeyFrom("kw", 0, "video")
	if hit, err := cache.Get(key); err != nil || hit.RawJSON == "" {
		t.Errorf("expected DB cache written once, got hit=%+v err=%v", hit, err)
	}

	// 缓存已写：再次调用应命中 DB，不触发源。
	called := atomic.LoadInt32(&src.calls)
	c.Search(context.Background(), "kw", 0, "video")
	if got := atomic.LoadInt32(&src.calls); got != called {
		t.Errorf("cached call should not re-fetch, calls=%d (was %d)", got, called)
	}
}

// TestMediaSearchDoubleCheckOnDB 验证 singleflight fn 内的 double-check：预先写未过期 DB 缓存后，
// 并发请求应全部命中 DB 缓存（连同 leader 都不进源 Search）。
func TestMediaSearchDoubleCheckOnDB(t *testing.T) {
	cache := oper.NewPosterCacheOper(newMediaDB(t))
	key := oper.CacheKeyFrom("kw", 0, "video")
	pre, _ := json.Marshal([]media.MediaInfo{{Source: "s1", Title: "CACHED", Type: media.TypeMovie}})
	if err := cache.Put(&models.PosterCache{CacheKey: key, RawJSON: string(pre)}); err != nil {
		t.Fatal(err)
	}

	src := &blockingMedia{
		name:    "s1",
		items:   []media.MediaInfo{{Source: "s1", Title: "LIVE", Type: media.TypeMovie}},
		types:   []media.MediaType{media.TypeMovie},
		proceed: make(chan struct{}),
	}
	c := NewMediaChain(Base{}, &MediaSourceProvider{sources: []media.MediaSource{src}}, cache, nil)

	res, err := c.Search(context.Background(), "kw", 0, "video")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Title != "CACHED" {
		t.Errorf("expected DB-cached result, got %+v", res)
	}
	if got := atomic.LoadInt32(&src.calls); got != 0 {
		t.Errorf("DB cache hit should not reach source Search, calls=%d", got)
	}
}
