package chain

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/media"

	"golang.org/x/sync/singleflight"
)

// 推荐信息流（热门/趋势）：并发扇出 + 内存缓存。
const (
	recommendMemTTL      = 30 * time.Minute // 趋势按日变化，30min 缓存足够
	recommendConcurrency = 4
	recommendTimeout     = 15 * time.Second
)

type recommendCacheEntry struct {
	items []media.MediaInfo
	at    time.Time
}

// RecommendChain 编排跨源热门/趋势信息流（豆瓣热映 / TMDB trending / Bangumi 热门）。
// 仅内存缓存（趋势是临时数据，无需 DB 持久化）。源需实现 media.TrendingCapable 才被纳入。
// 源来自 MediaSourceProvider（与 MediaChain 共用），故 UI 改凭据对推荐也即时生效。
type RecommendChain struct {
	Base
	provider *MediaSourceProvider
	mu       sync.Mutex
	cache    map[string]recommendCacheEntry
	sf       singleflight.Group
}

// NewRecommendChain 构造；provider 与 MediaChain 共用同一份（按优先级顺序）。
func NewRecommendChain(b Base, provider *MediaSourceProvider) *RecommendChain {
	return &RecommendChain{Base: b, provider: provider, cache: map[string]recommendCacheEntry{}}
}

// RecommendFeeds 取热门/趋势信息流。source 非空只取指定源（如 "douban"），
// 否则扇出所有实现 TrendingCapable 的源并合并。force=true 绕过内存缓存与 singleflight。
// 各源并发（semaphore 4）、独立超时、容错（单源失败不影响其它）。
// 非 force 时 miss 走 singleflight 合并同 key 并发回源（防击穿）。
func (c *RecommendChain) RecommendFeeds(ctx context.Context, source, category string, page int, force bool) []media.MediaInfo {
	if page < 1 {
		page = 1
	}
	key := source + "|" + category + "|" + strconv.Itoa(page)
	if !force {
		// 快速路径：命中内存缓存直接返回（不经 singleflight）。
		c.mu.Lock()
		if e, ok := c.cache[key]; ok && time.Since(e.at) <= recommendMemTTL {
			c.mu.Unlock()
			return e.items
		}
		c.mu.Unlock()
		// miss → singleflight 合并同 key 并发回源。
		all, _ := sfDo(&c.sf, key, func() ([]media.MediaInfo, error) {
			// double-check：leader 完成后紧随的 caller 会重跑 fn，此时缓存可能已满。
			c.mu.Lock()
			if e, ok := c.cache[key]; ok && time.Since(e.at) <= recommendMemTTL {
				c.mu.Unlock()
				return e.items, nil
			}
			c.mu.Unlock()
			return c.fetchRecommend(ctx, source, category, page, key), nil
		})
		return all
	}
	// force=true 绕过缓存与 singleflight，强制回源并刷新缓存。
	return c.fetchRecommend(ctx, source, category, page, key)
}

// fetchRecommend 扇出所有 TrendingCapable 源并合并结果；有结果时写入内存缓存。
func (c *RecommendChain) fetchRecommend(ctx context.Context, source, category string, page int, key string) []media.MediaInfo {
	sem := make(chan struct{}, recommendConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var all []media.MediaInfo
	for _, s := range c.filteredTrendingSources(source, category) {
		tc := s.(media.TrendingCapable)
		wg.Add(1)
		go func(name string, tc media.TrendingCapable) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, recommendTimeout)
			defer cancel()
			items, err := tc.Trending(cctx, category, page)
			if err != nil {
				if errors.Is(err, media.ErrNotConfigured) {
					return
				}
				slog.Debug("recommend source failed", "source", name, "err", err)
				return
			}
			mu.Lock()
			all = append(all, items...)
			mu.Unlock()
		}(s.Name(), tc)
	}
	wg.Wait()

	// 仅在有结果时缓存——全部源失败/空时不写缓存，避免瞬时失败被钉成空 30min。
	if len(all) > 0 {
		c.mu.Lock()
		c.cache[key] = recommendCacheEntry{items: all, at: time.Now()}
		c.mu.Unlock()
	}
	return all
}

// filteredTrendingSources 按当前筛选选出应扇出的趋势源（fetchRecommend 与流式推送共用）：
// source 非空只留同名源；仅留实现 TrendingCapable 的源（MusicBrainz 等跳过）；
// CategoryCapable 声明分类时跳过不支持当前分类的源（「短剧」只由星光集响应，亦避免豆瓣
// 在 anime 等分类回退默认热映污染），未实现该接口视为支持全部（向后兼容）。
func (c *RecommendChain) filteredTrendingSources(source, category string) []media.MediaSource {
	var out []media.MediaSource
	for _, s := range c.provider.Sources() {
		if source != "" && s.Name() != source {
			continue
		}
		if _, ok := s.(media.TrendingCapable); !ok {
			continue
		}
		if cc, ok := s.(media.CategoryCapable); ok && category != "" && !slices.Contains(cc.Categories(), category) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// RecommendFeedsStream 流式推送趋势信息流：每源 Trending 完成即调用 emit(items)，
// 供 SSE 端点「加载一个显示一个」。命中内存缓存（非 force）时一次性 emit 全量后返回
// （命中缓存本就瞬时，无需逐源扇出）。
// 与 RecommendFeeds 不同：不走 singleflight（回调流与单值合并不兼容），但缓存快速路径
// 已大幅压低并发回源压力；全部完成后按现有逻辑写缓存。
// 注意：emit 会被多个源 goroutine 并发调用，调用方（handler）须自行串行化实际写入。
func (c *RecommendChain) RecommendFeedsStream(ctx context.Context, source, category string, page int, force bool, emit func(items []media.MediaInfo)) {
	if page < 1 {
		page = 1
	}
	key := source + "|" + category + "|" + strconv.Itoa(page)
	// 缓存快速路径：命中即一次性吐全量。
	if !force {
		c.mu.Lock()
		if e, ok := c.cache[key]; ok && time.Since(e.at) <= recommendMemTTL {
			c.mu.Unlock()
			if len(e.items) > 0 {
				emit(e.items)
			}
			return
		}
		c.mu.Unlock()
	}
	// miss / force：并发扇出，每源完成即 emit；同时聚合用于缓存。
	srcs := c.filteredTrendingSources(source, category)
	sem := make(chan struct{}, recommendConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var all []media.MediaInfo
	for _, s := range srcs {
		tc := s.(media.TrendingCapable)
		wg.Add(1)
		go func(name string, tc media.TrendingCapable) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, recommendTimeout)
			defer cancel()
			items, err := tc.Trending(cctx, category, page)
			if err != nil {
				if errors.Is(err, media.ErrNotConfigured) {
					return
				}
				slog.Debug("recommend source failed", "source", name, "err", err)
				return
			}
			mu.Lock()
			all = append(all, items...)
			mu.Unlock()
			if len(items) > 0 {
				emit(items)
			}
		}(s.Name(), tc)
	}
	wg.Wait()
	if len(all) > 0 {
		c.mu.Lock()
		c.cache[key] = recommendCacheEntry{items: all, at: time.Now()}
		c.mu.Unlock()
	}
}
