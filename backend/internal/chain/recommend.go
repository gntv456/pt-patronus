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
}

// NewRecommendChain 构造；provider 与 MediaChain 共用同一份（按优先级顺序）。
func NewRecommendChain(b Base, provider *MediaSourceProvider) *RecommendChain {
	return &RecommendChain{Base: b, provider: provider, cache: map[string]recommendCacheEntry{}}
}

// RecommendFeeds 取热门/趋势信息流。source 非空只取指定源（如 "douban"），
// 否则扇出所有实现 TrendingCapable 的源并合并。force=true 绕过内存缓存。
// 各源并发（semaphore 4）、独立超时、容错（单源失败不影响其它）。
func (c *RecommendChain) RecommendFeeds(ctx context.Context, source, category string, page int, force bool) []media.MediaInfo {
	if page < 1 {
		page = 1
	}
	key := source + "|" + category + "|" + strconv.Itoa(page)
	if !force {
		c.mu.Lock()
		if e, ok := c.cache[key]; ok && time.Since(e.at) <= recommendMemTTL {
			c.mu.Unlock()
			return e.items
		}
		c.mu.Unlock()
	}

	sem := make(chan struct{}, recommendConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var all []media.MediaInfo
	for _, s := range c.provider.Sources() {
		if source != "" && s.Name() != source {
			continue
		}
		tc, ok := s.(media.TrendingCapable)
		if !ok {
			continue // MusicBrainz 等不支持趋势的源跳过
		}
		// CategoryCapable：源声明了支持的 category 时，跳过不支持当前分类的源
		// （「短剧」只由星光集响应；亦避免豆瓣在 anime 等分类回退默认热映污染）。
		// 未实现该接口的源视为支持全部（向后兼容）。
		if cc, ok := s.(media.CategoryCapable); ok && category != "" && !slices.Contains(cc.Categories(), category) {
			continue
		}
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
