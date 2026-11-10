// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/media"
)

// fakeSrc 实现 MediaSource + TrendingCapable；记录 Trending 调用次数以验证缓存。
// panicTrending=true 时 Trending 现场 panic（模拟解析器崩在畸形响应，验扇出账面）。
type fakeSrc struct {
	name          string
	items         []media.MediaInfo
	calls         int
	panicTrending bool
}

func (f *fakeSrc) Name() string { return f.name }
func (f *fakeSrc) Search(context.Context, string, int) ([]media.MediaInfo, error) {
	return nil, nil
}
func (f *fakeSrc) Detail(context.Context, string) (*media.MediaInfo, error) { return nil, nil }
func (f *fakeSrc) Trending(context.Context, string, int) ([]media.MediaInfo, error) {
	f.calls++
	if f.panicTrending {
		panic("nil pointer dereference on malformed trending response")
	}
	return f.items, nil
}

// plainSrc 仅实现 MediaSource（无 TrendingCapable）——应被 RecommendChain 跳过。
type plainSrc struct{ name string }

func (p *plainSrc) Name() string                                                   { return p.name }
func (p *plainSrc) Search(context.Context, string, int) ([]media.MediaInfo, error) { return nil, nil }
func (p *plainSrc) Detail(context.Context, string) (*media.MediaInfo, error)       { return nil, nil }

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
// 模拟「短剧」分类——只由寸光集类源（仅声明 duanju）响应，豆瓣类源（声明 movie/tv）
// 不应被调用，避免回退默认热映污染短剧结果。
func TestRecommendFeedsCategoryFilter(t *testing.T) {
	b := NewBase(module.New(), event.New())
	xg := &catSrc{fakeSrc: fakeSrc{name: "cunguangji", items: []media.MediaInfo{{Source: "cunguangji", Title: "短剧A"}}}, cats: []string{"duanju"}}
	db := &catSrc{fakeSrc: fakeSrc{name: "douban", items: []media.MediaInfo{{Source: "douban", Title: "电影B"}}}, cats: []string{"movie", "coming", "tv"}}
	rc := NewRecommendChain(b, &MediaSourceProvider{sources: []media.MediaSource{xg, db}})

	// 「短剧」分类：只取寸光集，豆瓣被跳过
	got := rc.RecommendFeeds(context.Background(), "", "duanju", 1, false)
	if len(got) != 1 || got[0].Title != "短剧A" {
		t.Fatalf("duanju: expected only cunguangji, got %+v", got)
	}
	if db.calls != 0 {
		t.Errorf("duanju: douban (declares movie/tv) should be skipped, called %d", db.calls)
	}

	// 反向：movie 分类时，只声明 duanju 的寸光集被跳过
	got2 := rc.RecommendFeeds(context.Background(), "", "movie", 1, false)
	if len(got2) != 1 || got[0].Title == got2[0].Title {
		t.Fatalf("movie: expected douban only, got %+v", got2)
	}
	if xg.calls != 1 {
		t.Errorf("movie: cunguangji should be skipped (still called once for duanju above), calls=%d", xg.calls)
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

// errSrc：Trending 返回 error，应被跳过不 emit（验证单源失败隔离）。
type errSrc struct{ name string }

func (e *errSrc) Name() string                                                   { return e.name }
func (e *errSrc) Search(context.Context, string, int) ([]media.MediaInfo, error) { return nil, nil }
func (e *errSrc) Detail(context.Context, string) (*media.MediaInfo, error)       { return nil, nil }
func (e *errSrc) Trending(context.Context, string, int) ([]media.MediaInfo, error) {
	return nil, errors.New("boom")
}

// TestRecommendFeedsStream 流式推送：每源完成即 emit，累计 = 全量；缓存命中一次性吐全量。
func TestRecommendFeedsStream(t *testing.T) {
	b := NewBase(module.New(), event.New())
	s1 := &fakeSrc{name: "s1", items: []media.MediaInfo{{Source: "s1", Title: "A"}}}
	s2 := &fakeSrc{name: "s2", items: []media.MediaInfo{{Source: "s2", Title: "B"}}}
	rc := NewRecommendChain(b, &MediaSourceProvider{sources: []media.MediaSource{s1, s2}})

	// ① 多源：emit 累计应等于两源合并（流式不排序，顺序不定）。
	// emit 会被多个源 goroutine 并发调用（RecommendFeedsStream 契约：调用方须
	// 自行串行化）——无锁 append 是数据竞争，曾间歇性丢一个源的 emit。
	var mu sync.Mutex
	var got []media.MediaInfo
	rc.RecommendFeedsStream(context.Background(), "", "movie", 1, true, func(items []media.MediaInfo) {
		mu.Lock()
		got = append(got, items...)
		mu.Unlock()
	})
	if len(got) != 2 {
		t.Fatalf("stream: expected 2 items across emits, got %d", len(got))
	}

	// ② force 回源后写缓存；非 force 再调应命中缓存一次性 emit 全量（只 emit 一次）
	var emits int
	var cached []media.MediaInfo
	rc.RecommendFeedsStream(context.Background(), "", "movie", 1, false, func(items []media.MediaInfo) {
		emits++
		cached = items
	})
	if emits != 1 {
		t.Fatalf("cache hit: expected 1 emit, got %d", emits)
	}
	if len(cached) != 2 {
		t.Fatalf("cache hit: expected 2 in one batch, got %d", len(cached))
	}
}

// TestRecommendFeedsStreamSourceFailure 单源失败不影响其它源 emit。
func TestRecommendFeedsStreamSourceFailure(t *testing.T) {
	b := NewBase(module.New(), event.New())
	ok := &fakeSrc{name: "ok", items: []media.MediaInfo{{Title: "OK"}}}
	bad := &errSrc{name: "bad"}
	rc := NewRecommendChain(b, &MediaSourceProvider{sources: []media.MediaSource{ok, bad}})

	var got []media.MediaInfo
	sum := rc.RecommendFeedsStream(context.Background(), "", "movie", 1, true, func(items []media.MediaInfo) {
		got = append(got, items...)
	})
	if len(got) != 1 || got[0].Title != "OK" {
		t.Fatalf("expected only OK source emitted (bad failed), got %+v", got)
	}
	// 账面要说到「哪一源没回来」：只回条目数时，页面少一源看着就像「今天就这些」。
	if sum.Sources != 2 || sum.Batches != 1 || len(sum.Failed) != 1 {
		t.Errorf("summary 应为「2 源 / 1 批 / 1 失败」，got %+v", sum)
	}
	if len(sum.Failed) == 1 && !strings.Contains(sum.Failed[0], "bad") {
		t.Errorf("failed 要点名源，got %q", sum.Failed[0])
	}

	// 有源失败时不得写缓存：否则这次缺席的慢源会被部分结果钉住 30min。
	again := rc.RecommendFeedsStream(context.Background(), "", "movie", 1, false, func(items []media.MediaInfo) {})
	if again.Cached {
		t.Error("上一轮有源失败，这一轮不该命中缓存（部分结果被钉住会让该源长期缺席）")
	}
	if len(again.Failed) != 1 {
		t.Errorf("重跑仍应报出失败源，got %+v", again)
	}
}

// slowSrc 阻塞到 delay 或 ctx 结束（先到者），模拟「源在但响应慢」。
type slowSrc struct {
	name  string
	delay time.Duration
	items []media.MediaInfo
}

func (s *slowSrc) Name() string { return s.name }
func (s *slowSrc) Search(context.Context, string, int) ([]media.MediaInfo, error) {
	return nil, nil
}
func (s *slowSrc) Detail(context.Context, string) (*media.MediaInfo, error) { return nil, nil }
func (s *slowSrc) Trending(ctx context.Context, _ string, _ int) ([]media.MediaInfo, error) {
	t := time.NewTimer(s.delay)
	defer t.Stop()
	select {
	case <-t.C:
		return s.items, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// TestRecommendFeedsBudgetPartialNotCached JSON 面到预算就返回已到手的部分，且**不写缓存**
// （把部分结果钉 30min 会让慢源长期缺席）。调用方 ctx 不设 deadline——只有预算能兜住扇出，
// 这样断言钉的是预算本身，而不是测试自己传的超时。
func TestRecommendFeedsBudgetPartialNotCached(t *testing.T) {
	old := recommendFeedsBudget
	recommendFeedsBudget = 600 * time.Millisecond
	defer func() { recommendFeedsBudget = old }()

	b := NewBase(module.New(), event.New())
	fast := &fakeSrc{name: "fast", items: []media.MediaInfo{{Source: "fast", Title: "F"}}}
	slow := &slowSrc{name: "slow", delay: 3 * time.Second, items: []media.MediaInfo{{Source: "slow", Title: "S"}}}
	rc := NewRecommendChain(b, &MediaSourceProvider{sources: []media.MediaSource{fast, slow}})

	start := time.Now()
	got := rc.RecommendFeeds(context.Background(), "", "movie", 1, true)
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("应在预算内返回部分结果，实际耗时 %v", el)
	}
	if len(got) != 1 || got[0].Title != "F" {
		t.Fatalf("应只拿到快源的 1 条，got %+v", got)
	}

	rc.RecommendFeeds(context.Background(), "", "movie", 1, false)
	if fast.calls != 2 {
		t.Fatalf("部分结果不该进缓存（第二次应重新扇出），fast.calls=%d", fast.calls)
	}
}
