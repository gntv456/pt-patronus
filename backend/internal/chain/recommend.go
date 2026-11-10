// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/media"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"golang.org/x/sync/singleflight"
)

// RecommendStreamSummary 流式首屏这一轮的账面：几个源参与、成功推出几批、哪些源没回来。
// 单源超时是 15s、与前端 axios 全局超时同值，「少一源」在界面上只是少几行卡片——
// 用户分辨不出「这源今天就这些」和「这源压根没返回」，故必须随 done 帧回传。
type RecommendStreamSummary struct {
	Sources int      `json:"sources"`
	Batches int      `json:"batches"`
	Failed  []string `json:"failed"`
	Cached  bool     `json:"cached"`
}

// String 给日志用的一行账面。
func (s RecommendStreamSummary) String() string {
	if len(s.Failed) == 0 {
		return "来源 " + strconv.Itoa(s.Sources) + " 个全部返回，批次 " + strconv.Itoa(s.Batches)
	}
	return "来源 " + strconv.Itoa(s.Sources) + " 个，失败 " + strconv.Itoa(len(s.Failed)) + " 个：" + strings.Join(s.Failed, "；")
}

// 推荐信息流（热门/趋势）：并发扇出 + 内存缓存。
const (
	recommendMemTTL      = 30 * time.Minute // 趋势按日变化，30min 缓存足够
	recommendConcurrency = 4
	recommendTimeout     = 15 * time.Second
)

// recommendFeedsBudget 是 JSON 面（非流式）的总预算：单源超时 15s 与前端 axios 全局 15s
// 同值，翻页会「后端还在扇出、客户端已超时」，用户只看见加载更多失败。8s 后不再等慢源，
// 返回先到手的部分结果。声明成 var 只为测试可临时调小（const 无法钉住预算本身）。
var recommendFeedsBudget = 8 * time.Second

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
			items, _ := c.fetchRecommend(ctx, source, category, page, key)
			return items, nil
		})
		return all
	}
	// force=true 绕过缓存与 singleflight，强制回源并刷新缓存。
	items, _ := c.fetchRecommend(ctx, source, category, page, key)
	return items
}

// fetchRecommend 扇出所有 TrendingCapable 源并合并结果；仅当扇出完整（没被预算砍、也没有源
// 报错/崩溃）且有结果时写内存缓存。返回 complete=false 表示这一页并不是各源的全集。
func (c *RecommendChain) fetchRecommend(ctx context.Context, source, category string, page int, key string) ([]media.MediaInfo, bool) {
	cctx, cancel := context.WithTimeout(ctx, recommendFeedsBudget)
	defer cancel()
	sem := make(chan struct{}, recommendConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var all []media.MediaInfo
	failed := 0
	for _, s := range c.filteredTrendingSources(source, category) {
		tc := s.(media.TrendingCapable)
		wg.Add(1)
		name := s.Name()
		safego.Go("recommend:88", func() {
			defer wg.Done()
			// per-source 隔离：崩掉的源也要计入账面——不记就是「N 个源、一个都没失败」，
			// 而那一源的内容静默缺席（同 #74/#123 那族「少一路看不见」）。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("recommend source panicked", "source", name, "err", r)
					mu.Lock()
					failed++
					mu.Unlock()
				}
			}()
			sem <- struct{}{}
			defer func() { <-sem }()
			sctx, cancel := context.WithTimeout(cctx, recommendTimeout)
			defer cancel()
			items, err := tc.Trending(sctx, category, page)
			if err != nil {
				if errors.Is(err, media.ErrNotConfigured) {
					return
				}
				slog.Warn("recommend source failed", "source", name, "err", err)
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			mu.Lock()
			all = append(all, items...)
			mu.Unlock()
		})
	}
	wg.Wait()
	// 「完整」= 预算没砍 + 没有源报错/崩溃。任一源没答话都不能把这次的部分结果钉 30min，
	// 否则缺席的那一源整半小时都不出现（流式面同规矩：见 RecommendFeedsStream 的 len(sum.Failed)==0）。
	complete := cctx.Err() == nil && failed == 0
	if !complete {
		slog.Warn("recommend fan-out incomplete, returning partial results",
			"key", key, "sources", len(c.filteredTrendingSources(source, category)), "failed", failed)
	}

	// 仅在有结果且扇出完整时缓存——部分结果被钉 30min 会让慢源长期缺席；
	// 全部源失败/空时不写缓存，避免瞬时失败被钉成空 30min。
	if len(all) > 0 && complete {
		c.mu.Lock()
		c.cache[key] = recommendCacheEntry{items: all, at: time.Now()}
		c.sweepCacheLocked()
		c.mu.Unlock()
	}
	return all, complete
}

// sweepCacheLocked 清扫过期缓存项（四轮 P2 修复：key 含 page，翻页枚举可无限撑大
// map 且过期条目永不清理；写入时顺手清扫，调用方需持 mu）。
func (c *RecommendChain) sweepCacheLocked() {
	now := time.Now()
	for k, e := range c.cache {
		if now.Sub(e.at) > recommendMemTTL {
			delete(c.cache, k)
		}
	}
}

// filteredTrendingSources 按当前筛选选出应扇出的趋势源（fetchRecommend 与流式推送共用）：
// source 非空只留同名源；仅留实现 TrendingCapable 的源（MusicBrainz 等跳过）；
// CategoryCapable 声明分类时跳过不支持当前分类的源（「短剧」只由寸光集响应，亦避免豆瓣
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
func (c *RecommendChain) RecommendFeedsStream(ctx context.Context, source, category string, page int, force bool, emit func(items []media.MediaInfo)) RecommendStreamSummary {
	sum := RecommendStreamSummary{}
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
				sum.Batches = 1
			}
			sum.Cached = true
			return sum
		}
		c.mu.Unlock()
	}
	// miss / force：并发扇出，每源完成即 emit；同时聚合用于缓存。
	srcs := c.filteredTrendingSources(source, category)
	sum.Sources = len(srcs)
	sem := make(chan struct{}, recommendConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var all []media.MediaInfo
	for _, s := range srcs {
		tc := s.(media.TrendingCapable)
		wg.Add(1)
		name := s.Name()
		safego.Go("recommend:171", func() {
			defer wg.Done()
			// per-source 隔离：崩掉的源必须进 sum.Failed——不记就是 done 帧报「N 个源、0 个失败」，
			// 而那一源的内容静默缺席。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("recommend stream source panicked", "source", name, "err", r)
					mu.Lock()
					sum.Failed = append(sum.Failed, fmt.Sprintf("%s：源异常: %v", name, r))
					mu.Unlock()
				}
			}()
			sem <- struct{}{}
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, recommendTimeout)
			defer cancel()
			items, err := tc.Trending(cctx, category, page)
			if err != nil {
				if errors.Is(err, media.ErrNotConfigured) {
					// 用户压根没配这个源：既不算失败，也不该留在「几个源」的分母里。
					mu.Lock()
					sum.Sources--
					mu.Unlock()
					return
				}
				slog.Debug("recommend source failed", "source", name, "err", err)
				mu.Lock()
				sum.Failed = append(sum.Failed, name+"："+errtext.Text(err))
				mu.Unlock()
				return
			}
			mu.Lock()
			all = append(all, items...)
			if len(items) > 0 {
				sum.Batches++
			}
			mu.Unlock()
			if len(items) > 0 {
				emit(items)
			}
		})
	}
	wg.Wait()
	// 有源没回来时不写缓存：与 JSON 面同一条规矩（#97），
	// 否则这次缺席的慢源会被部分结果钉住 30min，整半小时内那一源的内容都不出现。
	if len(all) > 0 && len(sum.Failed) == 0 {
		c.mu.Lock()
		c.cache[key] = recommendCacheEntry{items: all, at: time.Now()}
		c.sweepCacheLocked()
		c.mu.Unlock()
	}
	return sum
}
