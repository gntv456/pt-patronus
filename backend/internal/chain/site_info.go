package chain

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// 站免/公告抓取的并发与缓存参数（对齐原版：站免 30min、公告 60s）。
const (
	freeleechCacheTTL    = 30 * time.Minute
	announcementCacheTTL = 60 * time.Second
	infoFetchConcurrency = 8
	infoFetchTimeout     = 15 * time.Second
)

// FreeleechResult 是单站站免聚合结果（含错误，便于容错展示）。
type FreeleechResult struct {
	SiteID   uint                 `json:"site_id"`
	SiteName string               `json:"site_name"`
	Notices  []site.SiteFreeleech `json:"notices"`
	Error    string               `json:"error,omitempty"`
}

// Normalize 保证 Notices 非 nil：nil 切片 JSON 序列化为 null，前端 .length/[0] 会崩
//（未开启站免检测/抓空/未登录兜底时 Notices 为 nil）。All 与 SyncOne 出口统一调用。
func (r FreeleechResult) Normalize() FreeleechResult {
	if r.Notices == nil {
		r.Notices = []site.SiteFreeleech{}
	}
	return r
}

// AnnouncementResult 是单站公告聚合结果。
type AnnouncementResult struct {
	SiteID    uint                    `json:"site_id"`
	SiteName  string                  `json:"site_name"`
	RawText   string                  `json:"raw_text,omitempty"`
	Items     []site.AnnouncementItem `json:"items"`
	FetchedAt time.Time               `json:"fetched_at"`
	Error     string                  `json:"error,omitempty"`
}

// Normalize 保证 Items 非 nil：单站 SyncOne 在公告禁用/抓取失败时 ann 为 nil，
// r.Items 保持零值 nil → 序列化 "items":null 致前端 .length 崩，兜底为 []。
func (r AnnouncementResult) Normalize() AnnouncementResult {
	if r.Items == nil {
		r.Items = []site.AnnouncementItem{}
	}
	return r
}

// Freeleech 抓取单站全站免费/促销（适配器需实现 site.FreeleechCapable）。
func (c *SiteChain) Freeleech(ctx context.Context, s *models.Site) ([]site.SiteFreeleech, error) {
	def := c.defFor(s)
	if !def.Freeleech.Enabled {
		return nil, nil
	}
	token := c.credential(s, def)
	var out []site.SiteFreeleech
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		fc, ok := a.(site.FreeleechCapable)
		if !ok {
			return ErrNotSupported
		}
		ns, e := fc.FetchFreeleech(ctx, def, s.BaseURL, token)
		if e != nil {
			return e
		}
		out = ns
		return nil
	})
	for i := range out { // 盖章站点身份（适配器解析时不知 siteID/name）
		out[i].SiteID = s.ID
		out[i].SiteName = s.Name
	}
	return out, err
}

// Announcements 抓取单站公告（适配器需实现 site.AnnouncementCapable）。
func (c *SiteChain) Announcements(ctx context.Context, s *models.Site) (*site.SiteAnnouncement, error) {
	def := c.defFor(s)
	if !def.Announcement.Enabled {
		return nil, nil
	}
	token := c.credential(s, def)
	var out *site.SiteAnnouncement
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		ac, ok := a.(site.AnnouncementCapable)
		if !ok {
			return ErrNotSupported
		}
		ann, e := ac.FetchAnnouncement(ctx, def, s.BaseURL, token)
		if e != nil {
			return e
		}
		out = ann
		return nil
	})
	if out != nil {
		out.SiteID = s.ID
		out.SiteName = s.Name
		if out.Items == nil {
			out.Items = []site.AnnouncementItem{}
		}
	}
	return out, err
}

// FreeleechAll 跨站并发聚合站免（带 per-site 缓存；force=true 跳过缓存强制刷新）。
// 非 force 时单站 miss 走 singleflight 合并同 siteID 并发抓取（防击穿，sem 仅 leader 占用）。
func (c *SiteChain) FreeleechAll(ctx context.Context, force bool) []FreeleechResult {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("freeleech list sites failed", "err", err)
		return nil
	}
	results := make([]FreeleechResult, len(sites))
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		go func(i int, s models.Site) {
			defer wg.Done()
			// 快速路径：缓存命中（非 force 且未过期）→ 直接用，跳过网络。
			if !force {
				c.flMu.Lock()
				e, ok := c.flCache[s.ID]
				c.flMu.Unlock()
				if ok && time.Since(e.at) <= freeleechCacheTTL {
					results[i] = FreeleechResult{SiteID: s.ID, SiteName: s.Name, Notices: e.notices}.Normalize()
					return
				}
			}
			// miss：force 直接回源；否则 singleflight 合并同 siteID 并发回源。
			var ns []site.SiteFreeleech
			var e error
			if force {
				ns, e = c.fetchFreeleechOnce(ctx, sem, &s)
			} else {
				ns, e = sfDo(&c.flSF, strconv.Itoa(int(s.ID)), func() ([]site.SiteFreeleech, error) {
					// double-check：leader 完成后紧随的 caller 重跑 fn，缓存可能已满。
					c.flMu.Lock()
					ce, ok := c.flCache[s.ID]
					c.flMu.Unlock()
					if ok && time.Since(ce.at) <= freeleechCacheTTL {
						return ce.notices, nil
					}
					return c.fetchFreeleechOnce(ctx, sem, &s)
				})
			}
			r := FreeleechResult{SiteID: s.ID, SiteName: s.Name}
			switch {
			case errors.Is(e, site.ErrNotLoggedIn):
				ns = []site.SiteFreeleech{{SiteID: s.ID, SiteName: s.Name, Status: site.FreeleechFailed,
					Title: "未登录", Content: "cookie 失效，请刷新登录", FetchedAt: time.Now()}}
			case e != nil:
				r.Error = e.Error()
				slog.Debug("freeleech fetch failed", "site", s.Name, "err", e)
			}
			r.Notices = ns
			results[i] = r.Normalize()
		}(i, s)
	}
	wg.Wait()
	return results
}

// fetchFreeleechOnce 抓单站站免（占一个 sem 槽、独立超时），成功时写缓存。
// 失败/未登录不钉缓存，下次轮询即重试（避免「未登录」卡 30min）。
func (c *SiteChain) fetchFreeleechOnce(ctx context.Context, sem chan struct{}, s *models.Site) ([]site.SiteFreeleech, error) {
	sem <- struct{}{}
	defer func() { <-sem }()
	siteCtx, cancel := context.WithTimeout(ctx, infoFetchTimeout)
	defer cancel()
	ns, e := c.Freeleech(siteCtx, s)
	if e != nil {
		return nil, e
	}
	c.flMu.Lock()
	c.flCache[s.ID] = flCacheEntry{notices: ns, at: time.Now()}
	c.flMu.Unlock()
	return ns, nil
}

// annSFResult 是公告单次回源的 singleflight 结果（ann + 展示用 errMsg）。
type annSFResult struct {
	ann    *site.SiteAnnouncement
	errMsg string
}

// AnnouncementsAll 跨站并发聚合公告（带 per-site 缓存；force=true 跳过缓存）。
// 非 force 时单站 miss 走 singleflight 合并同 siteID 并发抓取（防击穿）。
func (c *SiteChain) AnnouncementsAll(ctx context.Context, force bool) []AnnouncementResult {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("announcements list sites failed", "err", err)
		return nil
	}
	results := make([]AnnouncementResult, len(sites))
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		go func(i int, s models.Site) {
			defer wg.Done()
			if !force {
				c.annMu.Lock()
				e, ok := c.annCache[s.ID]
				c.annMu.Unlock()
				if ok && time.Since(e.at) <= announcementCacheTTL && e.ann != nil {
					results[i] = annResult(s.ID, s.Name, e.ann, "")
					return
				}
			}
			var res annSFResult
			if force {
				res = c.fetchAnnouncementOnce(ctx, sem, &s)
			} else {
				res, _ = sfDo(&c.annSF, strconv.Itoa(int(s.ID)), func() (annSFResult, error) {
					c.annMu.Lock()
					ce, ok := c.annCache[s.ID]
					c.annMu.Unlock()
					if ok && time.Since(ce.at) <= announcementCacheTTL && ce.ann != nil {
						return annSFResult{ann: ce.ann}, nil
					}
					return c.fetchAnnouncementOnce(ctx, sem, &s), nil
				})
			}
			ann := res.ann
			if ann == nil {
				ann = &site.SiteAnnouncement{SiteID: s.ID, SiteName: s.Name, FetchedAt: time.Now()}
			}
			results[i] = annResult(s.ID, s.Name, ann, res.errMsg)
		}(i, s)
	}
	wg.Wait()
	return results
}

// fetchAnnouncementOnce 抓单站公告（占一个 sem 槽、独立超时），成功时写缓存。
// 失败/未登录不钉缓存（announcementCacheTTL 短，下次轮询重试）。
func (c *SiteChain) fetchAnnouncementOnce(ctx context.Context, sem chan struct{}, s *models.Site) annSFResult {
	sem <- struct{}{}
	defer func() { <-sem }()
	siteCtx, cancel := context.WithTimeout(ctx, infoFetchTimeout)
	defer cancel()
	ann, e := c.Announcements(siteCtx, s)
	var errMsg string
	if e != nil && !errors.Is(e, site.ErrNotLoggedIn) {
		errMsg = e.Error()
		slog.Debug("announcement fetch failed", "site", s.Name, "err", e)
	}
	if e == nil {
		c.annMu.Lock()
		c.annCache[s.ID] = annCacheEntry{ann: ann, at: time.Now()}
		c.annMu.Unlock()
	}
	return annSFResult{ann: ann, errMsg: errMsg}
}

func annResult(id uint, name string, ann *site.SiteAnnouncement, errMsg string) AnnouncementResult {
	items := ann.Items
	if items == nil {
		items = []site.AnnouncementItem{} // nil → [] 防 JSON null 致前端 .length 崩
	}
	r := AnnouncementResult{
		SiteID: id, SiteName: name,
		RawText:   ann.RawText,
		Items:     items,
		FetchedAt: ann.FetchedAt,
		Error:     errMsg,
	}
	return r
}
