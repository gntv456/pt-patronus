package chain

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// Search 在单个站点搜索。category 为媒体类型过滤（""=综合；movie/tv/music 按 schema 过滤），
// 后置过滤（丢弃音乐等异类）在此统一施加，所有 schema 受益。
func (c *SiteChain) Search(ctx context.Context, s *models.Site, keyword, category string, page int) ([]site.Torrent, error) {
	def := c.defFor(s)
	token := c.credential(s, def)
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

// SearchSites 跨所有启用站点并发聚合搜索（每站独立超时；容错：单站失败不影响其它）。
// 各 goroutine 写入预分配 results 的不同下标，无需加锁；并发安全依据见各适配器实现
// （独立 HTTP + 本地解析）与 oper.Cookie/APIKey 的只读解密。
func (c *SiteChain) SearchSites(ctx context.Context, keyword, category string, siteIDs []uint) []SiteResult {
	sites, err := c.sites.ListActive()
	if err != nil {
		return nil
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

			siteCtx, cancel := context.WithTimeout(ctx, searchTimeout)
			defer cancel()

			ts, e := c.Search(siteCtx, &s, keyword, category, 1)
			for i := range ts {
				ts[i].SiteID = s.ID
				ts[i].SitePriority = s.Priority
			}
			r := SiteResult{SiteID: s.ID, SiteName: s.Name, Torrents: ts}
			if e != nil {
				r.Torrents = nil
				r.Error = e.Error()
				slog.Debug("site search failed", "site", s.Name, "err", e)
			}
			results[i] = r
		})
	}
	wg.Wait()
	return results
}

// SearchSitesStream 是 SearchSites 的流式版：每搜完一个站点立即通过 emit 推送该站结果
// （含失败站，Error 字段承载错误），供 SSE「搜出一个显示一个」。并发/超时/过滤逻辑与
// SearchSites 完全一致，仅把"预分配 results[i]=r"换成"完成后 emit(r)"。
func (c *SiteChain) SearchSitesStream(ctx context.Context, keyword, category string,
	siteIDs []uint, emit func(SiteResult)) {
	sites, err := c.sites.ListActive()
	if err != nil {
		return
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

			siteCtx, cancel := context.WithTimeout(ctx, searchTimeout)
			defer cancel()

			ts, e := c.Search(siteCtx, &s, keyword, category, 1)
			for i := range ts {
				ts[i].SiteID = s.ID
				ts[i].SitePriority = s.Priority
			}
			r := SiteResult{SiteID: s.ID, SiteName: s.Name, Torrents: ts}
			if e != nil {
				r.Torrents = nil
				r.Error = e.Error()
				slog.Debug("site search failed", "site", s.Name, "err", e)
			}
			emit(r)
		})
	}
	wg.Wait()
}
