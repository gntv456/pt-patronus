package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/media"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"golang.org/x/sync/singleflight"
)

// mediaCacheTTL 搜索/元数据缓存有效期。影视元数据相对稳定，但过期重查可避免
// 上游源临时失败（如豆瓣 429）被永久冻结成「只有音乐」之类的坏结果集。
const mediaCacheTTL = 24 * time.Hour

// mediaSearchBudget 单次跨源搜索的总时限。各源并发，互不阻塞；不可达的源（如国内
// 直连 api.themoviedb.org / musicbrainz.org）只占用自身预算、被 context 砍掉，
// 不再像旧串行实现那样把多个 15s 超时累加成 30s+ 触发前端 axios(15s) 超时。
// 留出余量在前端 15s 之下。
const mediaSearchBudget = 8 * time.Second

// MediaChain 编排媒体元数据匹配：跨源搜索（并发，按优先级合并）→ 类型过滤 → 去重 → 缓存。
// 源来自 MediaSourceProvider（SystemConfig 可热更新），故 UI 改凭据即时生效。
// 另持多评分聚合（AggregatedRating）+ 订阅质量配置（MediaQualityConfig，存 SystemConfig `media.quality`）。
type MediaChain struct {
	Base
	provider *MediaSourceProvider
	cache    *oper.PosterCacheOper
	cfg      *oper.SystemConfigOper
	sf       singleflight.Group

	ratingCacheMu sync.RWMutex
	ratingCache   map[string]ratingCacheEntry // 聚合评分内存缓存（key=normalized title+year+type）
}

// NewMediaChain 构造；provider 提供按优先级顺序的源（影视主源在前）。cfg 用于读写质量配置。
func NewMediaChain(b Base, provider *MediaSourceProvider, cache *oper.PosterCacheOper, cfg *oper.SystemConfigOper) *MediaChain {
	return &MediaChain{Base: b, provider: provider, cache: cache, cfg: cfg}
}

// MediaConfig 返回当前生效的媒体源设置（原始值；调用方负责脱敏展示）。
func (c *MediaChain) MediaConfig() MediaSettings {
	return c.provider.LoadSettings()
}

// Provider 暴露媒体源 provider（日历等直接取 TMDB 源能力用；调用方只读）。
func (c *MediaChain) Provider() *MediaSourceProvider { return c.provider }

// SetMediaConfig 应用新设置（空字段不修改）并 rebuild 源，即时生效；
// 同时清空搜索缓存——旧结果集（可能源自失效凭据/被墙域名）不应再被命中。
func (c *MediaChain) SetMediaConfig(s MediaSettings) error {
	if err := c.provider.ApplySettings(s); err != nil {
		return err
	}
	if c.cache != nil {
		_ = c.cache.ClearSearch()
	}
	return nil
}

// Detail 按源名 + source_id 取富详情（演职/推荐/时长/出品国等）。供「媒体详情页」用。
func (c *MediaChain) Detail(ctx context.Context, source, id string) (*media.MediaInfo, error) {
	for _, s := range c.provider.Sources() {
		if s.Name() == source {
			return s.Detail(ctx, id)
		}
	}
	return nil, fmt.Errorf("unknown media source %q", source)
}

// CelebrityCapable 是可选源能力：按源内人员 id 返回演员简介（点 cast 头像拉取）。
type CelebrityCapable interface {
	Celebrity(ctx context.Context, personageID string) (*media.CelebrityProfile, error)
}

// Celebrity 按源名 + 演员源内 id 拉演员简介。源无该能力则报错（目前仅豆瓣实现）。
func (c *MediaChain) Celebrity(ctx context.Context, source, personageID string) (*media.CelebrityProfile, error) {
	for _, s := range c.provider.Sources() {
		if s.Name() == source {
			if cc, ok := s.(CelebrityCapable); ok {
				return cc.Celebrity(ctx, personageID)
			}
			return nil, fmt.Errorf("source %q 不支持演员简介", source)
		}
	}
	return nil, fmt.Errorf("unknown media source %q", source)
}

// Search 跨源搜索。命中（且未过期）缓存直接返回；否则**并发**逐源搜索、按 filter
// 过滤、按源优先级合并、写缓存。
//
// 并发 + 总预算(mediaSearchBudget) 是关键：旧实现串行跑各源，任一不可达源（TMDB/
// MusicBrainz 在国内常被墙）会占满自身 15s 超时并累加，30s+ 必然触发前端 axios(15s)
// 「请求超时」。现在各源并行，不可达源被 context 在预算内砍掉，可达源（豆瓣/Bangumi）
// 正常返回，总时长 ≈ min(预算, 最慢单源)。
//
// filter 控制返回哪些类型（见 media.TypeMatches）：
//   - ""      全部（MatchPoster 等内部用，保留音乐）
//   - "video" 影视：movie+tv，排除音乐——「影视」搜索默认
//   - "movie"/"tv"/"music" 精确
//
// filter 同时进缓存键；并按 media.TypedSource 跳过产物必不在 filter 内的源
// （如 video 搜时跳过只产音乐的 MusicBrainz，省一次无用且常慢的海外请求）。
// 源失败用 Warn 记录（含 context 超时），便于排查。
func (c *MediaChain) Search(ctx context.Context, keyword string, year int, filter string) ([]media.MediaInfo, error) {
	key := oper.CacheKeyFrom(keyword, year, filter)
	// 快速路径：命中 DB 缓存直接返回（不经 singleflight）。
	if cached, ok := c.cachedSearch(key); ok {
		return cached, nil
	}
	// miss → singleflight 合并同 key 并发跨源搜索（防击穿）。
	return sfDo(&c.sf, key, func() ([]media.MediaInfo, error) {
		// double-check DB：leader 完成后紧随的 caller 重跑 fn，DB 可能已写。
		if cached, ok := c.cachedSearch(key); ok {
			return cached, nil
		}
		return c.searchSources(ctx, keyword, year, filter, key)
	})
}

// cachedSearch 命中未过期的 DB 缓存时返回 (结果, true)。cache 为 nil 或未命中返回 false。
// 快速路径与 singleflight fn 内的 double-check 共用。
func (c *MediaChain) cachedSearch(key string) ([]media.MediaInfo, bool) {
	if c.cache == nil {
		return nil, false
	}
	hit, err := c.cache.Get(key)
	if err != nil || hit.RawJSON == "" || time.Since(hit.UpdatedAt) >= mediaCacheTTL {
		return nil, false
	}
	var cached []media.MediaInfo
	if json.Unmarshal([]byte(hit.RawJSON), &cached) != nil {
		return nil, false
	}
	return cached, true
}

// searchSources 并发跨源搜索（总预算 mediaSearchBudget）、按源优先级合并 + 类型过滤，
// 有结果时写 DB 缓存。全部参与源都没答话时交回错误——「一个源都没读回来」不能和
// 「库里没这部片」同形（对齐 media_rating「全部 N 个评分源查询失败」的口径）。
func (c *MediaChain) searchSources(ctx context.Context, keyword string, year int, filter, key string) ([]media.MediaInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, mediaSearchBudget)
	defer cancel()

	// 取当前源切片一次（provider 可能被 UI 改动 rebuild，本请求内用同一份快照）。
	sources := c.provider.Sources()

	type srcOut struct {
		items []media.MediaInfo
		err   error
	}
	outs := make([]srcOut, len(sources))
	var wg sync.WaitGroup
	for i, s := range sources {
		// 跳过产物必不在 filter 内的源（TypedSource 可选能力）。
		if ts, ok := s.(media.TypedSource); ok && !media.SourceMatches(ts.Types(), filter) {
			continue
		}
		wg.Add(1)
		safego.Go("media:163", func() {
			defer wg.Done()
			// per-source 隔离：崩掉的源记成它自己的失败——零值槽 err=nil 会被下面当成
			// 「这个源没搜到」，全崩时页面就是「库里没这部片」的假绿。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("media source panicked", "source", s.Name(), "err", r)
					outs[i] = srcOut{err: fmt.Errorf("媒体源异常: %v", r)}
				}
			}()
			items, err := s.Search(ctx, keyword, year)
			outs[i] = srcOut{items: items, err: err} // 各写各的索引，无竞争
		})
	}
	wg.Wait()

	// 按源优先级（slice 顺序）合并 + 类型过滤。
	var all []media.MediaInfo
	attempted, responded := 0, 0
	for i, s := range sources {
		o := outs[i]
		if o.err != nil {
			if errors.Is(o.err, media.ErrNotConfigured) {
				continue // 未配置：既不算参与也不算失败
			}
			attempted++
			slog.Warn("media source search failed", "source", s.Name(), "err", o.err)
			continue
		}
		attempted++
		responded++
		for _, m := range o.items {
			if media.TypeMatches(m.Type, filter) {
				all = append(all, m)
			}
		}
	}
	if attempted > 0 && responded == 0 {
		return nil, fmt.Errorf("全部 %d 个媒体源查询失败", attempted)
	}

	if c.cache != nil && len(all) > 0 {
		raw, _ := json.Marshal(all)
		_ = c.cache.Put(&models.PosterCache{CacheKey: key, RawJSON: string(raw)})
	}
	return all, nil
}

// MatchPoster 返回第一个带海报的结果海报 URL（用于下载器任务海报匹配）。
// filter 传 ""（全部类型），保留音乐海报匹配——影视搜索才排除音乐。
func (c *MediaChain) MatchPoster(ctx context.Context, keyword string, year int) (string, error) {
	if keyword == "" {
		return "", nil // 空关键词扇出去等于「拿任意热门海报」，直接判定无海报
	}
	res, err := c.Search(ctx, keyword, year, "")
	if err != nil {
		return "", err
	}
	for _, m := range res {
		if m.PosterURL != "" {
			return m.PosterURL, nil
		}
	}
	return "", nil
}

// NFO 生成 NFO XML。
func (c *MediaChain) NFO(mi *media.MediaInfo) string {
	return media.NFO(mi)
}
