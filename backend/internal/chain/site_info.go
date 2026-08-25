package chain

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// 站免/公告抓取的并发与缓存参数（对齐原版：站免 30min、公告 60s）。
const (
	freeleechCacheTTL    = 30 * time.Minute
	announcementCacheTTL = 5 * time.Minute
	infoFetchConcurrency = 8
	infoFetchTimeout     = 15 * time.Second
	connectivityFreshTTL = 2 * time.Minute // SSE 进页面新鲜度节流：近此窗口内已测的站直接复用缓存，避免每次打开页面全量重测
)

// FreeleechResult 是单站站免聚合结果（含错误，便于容错展示）。
type FreeleechResult struct {
	SiteID   uint                 `json:"site_id"`
	SiteName string               `json:"site_name"`
	Notices  []site.SiteFreeleech `json:"notices"`
	Error    string               `json:"error,omitempty"`
}

// Normalize 保证 Notices 非 nil：nil 切片 JSON 序列化为 null，前端 .length/[0] 会崩
// （未开启站免检测/抓空/未登录兜底时 Notices 为 nil）。All 与 SyncOne 出口统一调用。
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

// FreeleechAll 跨站并发聚合站免（阻塞，等全部站完成；带 per-site 缓存；force=true 跳过缓存）。
// 给 MCP 工具/SiteDetail 等需一次性拿全量的调用方用；前端页面要"加载一个显示一个"用 FreeleechStream。
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
		safego.Go("site_info:130", func() {
			defer wg.Done()
			results[i] = c.resolveFreeleech(ctx, sem, s, force)
		})
	}
	wg.Wait()
	return results
}

// FreeleechStream 流式聚合站免：每站解析完立即 emit（供 NDJSON/SSE 端点「加载一个显示一个」）。
// 缓存命中的站秒级 emit，慢站（context deadline 15s）最后到达，不阻塞已就绪站的展示；
// SWR/缓存/负缓存逻辑同聚合（复用 resolveFreeleech）。emit 由调用方保证并发安全（多 goroutine 同时回调）。
func (c *SiteChain) FreeleechStream(ctx context.Context, force bool, emit func(FreeleechResult)) {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("freeleech stream list sites failed", "err", err)
		return
	}
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for _, s := range sites {
		wg.Add(1)
		safego.Go("site_info:152", func() {
			defer wg.Done()
			emit(c.resolveFreeleech(ctx, sem, s, force))
		})
	}
	wg.Wait()
}

// resolveFreeleech 解析单站站免结果（缓存命中/SWR/miss/force 同聚合逻辑），供聚合与流式复用。
func (c *SiteChain) resolveFreeleech(ctx context.Context, sem chan struct{}, s models.Site, force bool) FreeleechResult {
	if !force {
		c.flMu.Lock()
		e, ok := c.flCache[s.ID]
		c.flMu.Unlock()
		// SWR：缓存存在（含已过期）→ 立即返回旧值（秒回）；过期则后台异步刷新，避免每 30min
		// TTL 到期时首个请求阻塞 ~15s 等全网站免（对齐公告 resolveAnnouncement 的 SWR 模式）。
		// fetchFreeleechOnce 仅成功才写缓存，故 ok 即代表有有效旧值；无缓存才走 singleflight 回源。
		if ok {
			if time.Since(e.at) > freeleechCacheTTL {
				c.refreshFreeleechAsync(s)
			}
			return FreeleechResult{SiteID: s.ID, SiteName: s.Name, Notices: e.notices}.Normalize()
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
	return r.Normalize()
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

// refreshFreeleechAsync SWR 后台刷新：站免缓存过期时立即返回旧值后，异步拉最新写缓存。
// 独立 ctx（不绑请求）+ sfDo 合并同 siteID（防刷新风暴）+ recover 防 panic。容量 1 的 sem
// 仅占槽不阻塞（真实限流靠 sfDo 合并 + 站点有界），与公告路径 refreshAnnouncementsAsync 等效。
func (c *SiteChain) refreshFreeleechAsync(s models.Site) {
	go func() {
		defer func() { _ = recover() }()
		_, _ = sfDo(&c.flSF, strconv.Itoa(int(s.ID)), func() ([]site.SiteFreeleech, error) {
			c.flMu.Lock()
			ce, ok := c.flCache[s.ID]
			c.flMu.Unlock()
			if ok && time.Since(ce.at) <= freeleechCacheTTL { // double-check：可能已被其他请求刷新
				return ce.notices, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), infoFetchTimeout)
			defer cancel()
			return c.fetchFreeleechOnce(ctx, make(chan struct{}, 1), &s)
		})
	}()
}

// annSFResult 是公告单次回源的 singleflight 结果（ann + 展示用 errMsg）。
type annSFResult struct {
	ann    *site.SiteAnnouncement
	errMsg string
}

// AnnouncementsAll 跨站并发聚合公告（带 per-site 缓存；force=true 跳过缓存）。
// 等全部站完成后一次性返回（MCP 工具/cron 用）；前端页面要"加载一个显示一个"用 AnnouncementsStream。
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
		safego.Go("site_info:262", func() {
			defer wg.Done()
			results[i] = c.resolveAnnouncement(ctx, sem, s, force)
		})
	}
	wg.Wait()
	return results
}

// resolveAnnouncement 解析单站公告结果（缓存命中/SWR/miss/force 同聚合逻辑），供聚合与流式复用。
func (c *SiteChain) resolveAnnouncement(ctx context.Context, sem chan struct{}, s models.Site, force bool) AnnouncementResult {
	if !force {
		c.annMu.Lock()
		e, ok := c.annCache[s.ID]
		c.annMu.Unlock()
		// SWR：缓存存在（含已过期）且有 ann → 立即返回旧值（秒回）；过期则后台异步刷新。
		if ok && e.ann != nil {
			if time.Since(e.at) > announcementCacheTTL {
				c.refreshAnnouncementsAsync(s)
			}
			return annResult(s.ID, s.Name, e.ann, "")
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
	return annResult(s.ID, s.Name, ann, res.errMsg)
}

// AnnouncementsStream 流式聚合：每站解析完立即通过 emit 推送（不等全部），供前端"加载一个显示一个"。
// 缓存命中的站秒级 emit，慢站（context deadline 15s）最后到达；缓存/SWR/负缓存逻辑同聚合。
// emit 由调用方保证并发安全（多 goroutine 同时回调）。
func (c *SiteChain) AnnouncementsStream(ctx context.Context, force bool, emit func(AnnouncementResult)) {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("announcements stream list sites failed", "err", err)
		return
	}
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for _, s := range sites {
		wg.Add(1)
		safego.Go("site_info:319", func() {
			defer wg.Done()
			emit(c.resolveAnnouncement(ctx, sem, s, force))
		})
	}
	wg.Wait()
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
	if e != nil {
		// 未登录同样作为错误暴露（与站免路径 FreeleechAll 一致）：否则 cookie 过期会被前端
		// 误显为「暂无公告」，用户无从知晓该站需刷新登录。ErrNotLoggedIn 文本 "not logged in"
		// 不含 "login" 子串，前端 friendlySiteError 匹配不到关键词会落到兜底「该站点获取失败」，
		// 故此处补 "cookie"/"login" 关键词，使其译为「站点登录已失效，请在「站点」页重新配置 Cookie」。
		if errors.Is(e, site.ErrNotLoggedIn) {
			errMsg = "cookie expired or not logged in"
		} else {
			errMsg = e.Error()
			slog.Debug("announcement fetch failed", "site", s.Name, "err", e)
		}
	}
	if e == nil {
		c.annMu.Lock()
		c.annCache[s.ID] = annCacheEntry{ann: ann, at: time.Now()}
		c.annMu.Unlock()
	}
	return annSFResult{ann: ann, errMsg: errMsg}
}

// refreshAnnouncementsAsync SWR 后台刷新：公告缓存过期时立即返回旧值后，异步拉最新写缓存。
// 独立 ctx（不绑请求）+ sfDo 合并同 siteID（防刷新风暴）+ recover 防 panic。容量 1 的 sem
// 仅占槽不阻塞（真实限流靠 sfDo 合并 + 站点有界），与消息路径 refreshMessagesAsync 等效。
func (c *SiteChain) refreshAnnouncementsAsync(s models.Site) {
	go func() {
		defer func() { _ = recover() }()
		_, _ = sfDo(&c.annSF, strconv.Itoa(int(s.ID)), func() (annSFResult, error) {
			c.annMu.Lock()
			ce, ok := c.annCache[s.ID]
			c.annMu.Unlock()
			if ok && ce.ann != nil && time.Since(ce.at) <= announcementCacheTTL { // double-check
				return annSFResult{ann: ce.ann}, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), infoFetchTimeout)
			defer cancel()
			return c.fetchAnnouncementOnce(ctx, make(chan struct{}, 1), &s), nil
		})
	}()
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
