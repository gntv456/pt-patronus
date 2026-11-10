// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/site"
)

// Search 在单个站点搜索。category 为媒体类型过滤（""=综合；movie/tv/music 按 schema 过滤），
// 后置过滤（丢弃音乐等异类）在此统一施加，所有 schema 受益。
func (c *SiteChain) Search(ctx context.Context, s *models.Site, keyword, category string, page int) ([]site.Torrent, error) {
	def := c.defFor(s)
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
	var out []site.Torrent
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		ts, e := a.Search(ctx, def, s.BaseURL, token, keyword, category, page)
		if e != nil {
			return e
		}
		out = ts
		return nil
	})
	if err == nil {
		out = site.FilterByCategory(out, category)
	}
	return out, err
}

// searchConcurrency 限制跨站搜索的最大并发数（站点数通常较小，封顶避免对站点瞬时打满）。
const searchConcurrency = 8

// searchTimeout 是单站搜索的超时上限，单个慢站不会拖垮整体聚合。
const searchTimeout = 15 * time.Second

// searchOneSite 跑单站搜索并产出该站的结果（批处理与流式共用同一具结果形状）。
// 兜住 panic：适配器解析畸形 HTML 时可能在这一 goroutine 里空指针崩溃，而 safego.Go 只记日志
// 不留痕——不兜住的话批处理版 results[i] 停在零值（无站名、无错误）、流式版连一帧都不推，
// 前端就把「这个站崩了」渲染成「这个站没有结果」，用户无从察觉漏站。
func (c *SiteChain) searchOneSite(ctx context.Context, s *models.Site, keyword, category string) (r SiteResult) {
	r = SiteResult{SiteID: s.ID, SiteName: s.Name}
	defer func() {
		if p := recover(); p != nil {
			slog.Error("site search panic", "site", s.Name, "panic", p)
			r = SiteResult{SiteID: s.ID, SiteName: s.Name, Error: fmt.Sprintf("搜索异常(panic): %v", p)}
		}
	}()
	siteCtx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	ts, e := c.Search(siteCtx, s, keyword, category, 1)
	if e != nil {
		slog.Debug("site search failed", "site", s.Name, "err", e)
		return SiteResult{SiteID: s.ID, SiteName: s.Name, Error: errtext.Text(e)}
	}
	for i := range ts {
		ts[i].SiteID = s.ID
		ts[i].SitePriority = s.Priority
	}
	r.Torrents = ts
	return r
}

// SearchSites 跨所有启用站点并发聚合搜索（每站独立超时；容错：单站失败不影响其它）。
// 各 goroutine 写入预分配 results 的不同下标，无需加锁；并发安全依据见各适配器实现
// （独立 HTTP + 本地解析）与 oper.Cookie/APIKey 的只读解密。
// 站点清单读失败交回错误，不渲染成「全部站无结果」假绿。
func (c *SiteChain) SearchSites(ctx context.Context, keyword, category string, siteIDs []uint) ([]SiteResult, error) {
	sites, err := c.sites.ListActive()
	if err != nil {
		return nil, err
	}
	// siteIDs 非空时仅搜索指定的启用站点子集（前端「站点筛选」用；空=全部启用站点）。
	// 复用 sites[:0] 底层数组原地过滤：range 在循环开始即固定遍历范围，append 覆盖前置元素不影响迭代。
	if len(siteIDs) > 0 {
		want := make(map[uint]struct{}, len(siteIDs))
		for _, id := range siteIDs {
			want[id] = struct{}{}
		}
		filtered := sites[:0]
		for _, s := range sites {
			if _, ok := want[s.ID]; ok {
				filtered = append(filtered, s)
			}
		}
		sites = filtered
	}
	results := make([]SiteResult, len(sites))
	sem := make(chan struct{}, searchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		safego.Go("site:308", func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = c.searchOneSite(ctx, &s, keyword, category)
		})
	}
	wg.Wait()
	return results, nil
}

// SearchSitesStream 是 SearchSites 的流式版：每搜完一个站点立即通过 emit 推送该站结果
// （含失败站，Error 字段承载错误），供 SSE「搜出一个显示一个」。并发/超时/过滤逻辑与
// SearchSites 完全一致，仅把"预分配 results[i]=r"换成"完成后 emit(r)"。
// 返回值：站点清单读失败的错误消息（""=正常），供端点随 done 帧带出。
func (c *SiteChain) SearchSitesStream(ctx context.Context, keyword, category string,
	siteIDs []uint, emit func(SiteResult)) string {
	sites, err := c.sites.ListActive()
	if err != nil {
		return err.Error()
	}
	// siteIDs 非空时仅搜索指定的启用站点子集（与 SearchSites 同款原地过滤）。
	if len(siteIDs) > 0 {
		want := make(map[uint]struct{}, len(siteIDs))
		for _, id := range siteIDs {
			want[id] = struct{}{}
		}
		filtered := sites[:0]
		for _, s := range sites {
			if _, ok := want[s.ID]; ok {
				filtered = append(filtered, s)
			}
		}
		sites = filtered
	}
	sem := make(chan struct{}, searchConcurrency)
	var wg sync.WaitGroup
	for _, s := range sites {
		wg.Add(1)
		safego.Go("site:360", func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			emit(c.searchOneSite(ctx, &s, keyword, category))
		})
	}
	wg.Wait()
	return ""
}
