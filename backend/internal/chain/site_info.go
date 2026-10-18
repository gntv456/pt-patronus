package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/errtext"
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
	// Unsupported = 这一站的站免压根没接线（站点定义未开启该 capability），与「抓到了，
	// 当前没有促销」分开：前者是配置事实，后者是站点现状（同 MessageResult.Unsupported）。
	Unsupported bool `json:"unsupported,omitempty"`
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
	// Unsupported = 这一站的公告没接线（站点定义未开启该 capability），区别于「抓到了，暂无公告」。
	Unsupported bool `json:"unsupported,omitempty"`
}

// FreeleechWired / AnnouncementWired 该站的站免/公告是否接了线（站点定义开启对应 capability）。
// 判定各只写这一处：聚合面与单站同步面共用，抄第二遍必漂移。
func (c *SiteChain) FreeleechWired(s *models.Site) bool { return c.defFor(s).Freeleech.Enabled }

// AnnouncementWired 见 FreeleechWired。
func (c *SiteChain) AnnouncementWired(s *models.Site) bool { return c.defFor(s).Announcement.Enabled }

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
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
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
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
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

// flPanicResult 崩站的兜底行（JSON 面与流式面共用，防两处口径漂移）：Notices 经 Normalize
// 归一为 []，零值行的 site_id=0 + notices=nil 既匿名又会让前端 .length 崩。
func flPanicResult(s models.Site, r any) FreeleechResult {
	return FreeleechResult{
		SiteID:   s.ID,
		SiteName: s.Name,
		Error:    fmt.Sprintf("站免抓取异常: %v", r),
	}.Normalize()
}

// FreeleechAll 跨站并发聚合站免（阻塞，等全部站完成；带 per-site 缓存；force=true 跳过缓存）。
// 给 MCP 工具/SiteDetail 等需一次性拿全量的调用方用；前端页面要"加载一个显示一个"用 FreeleechStream。
// 站点清单读失败交回错误，不渲染成「全部站无站免」假绿。
func (c *SiteChain) FreeleechAll(ctx context.Context, force bool) ([]FreeleechResult, error) {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("freeleech list sites failed", "err", err)
		return nil, err
	}
	results := make([]FreeleechResult, len(sites))
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		safego.Go("site_info:130", func() {
			defer wg.Done()
			// per-site 隔离：崩站交出它自己那一行（站名 + 空 notices + 原因）。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("freeleech site panic", "site", s.Name, "err", r)
					results[i] = flPanicResult(s, r)
				}
			}()
			results[i] = c.resolveFreeleech(ctx, sem, s, force)
		})
	}
	wg.Wait()
	return results, nil
}

// FreeleechStream 流式聚合站免：每站解析完立即 emit（供 NDJSON/SSE 端点「加载一个显示一个」）。
// 缓存命中的站秒级 emit，慢站（context deadline 15s）最后到达，不阻塞已就绪站的展示；
// SWR/缓存/负缓存逻辑同聚合（复用 resolveFreeleech）。emit 由调用方保证并发安全（多 goroutine 同时回调）。
// 返回值：站点清单读失败的错误消息（""=正常），供端点在流末尾补 done 行留痕。
func (c *SiteChain) FreeleechStream(ctx context.Context, force bool, emit func(FreeleechResult)) string {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("freeleech stream list sites failed", "err", err)
		return err.Error()
	}
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for _, s := range sites {
		wg.Add(1)
		safego.Go("site_info:152", func() {
			defer wg.Done()
			// per-site 隔离：崩站也必须 emit 一行，否则前端只看到「少一站」。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("freeleech stream site panic", "site", s.Name, "err", r)
					emit(flPanicResult(s, r))
				}
			}()
			emit(c.resolveFreeleech(ctx, sem, s, force))
		})
	}
	wg.Wait()
	return ""
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
			return FreeleechResult{SiteID: s.ID, SiteName: s.Name, Notices: e.notices,
				Unsupported: !c.FreeleechWired(&s)}.Normalize()
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
	return c.flRow(&s, ns, e)
}

// flRow 组装单站站免行（聚合尾与单站同步共用，防两处口径漂移）：未登录合成可读提示行、
// 其它失败走中文翻译口、nil notices 归一为 []，并标上「这一站没接线」判定。
func (c *SiteChain) flRow(s *models.Site, ns []site.SiteFreeleech, e error) FreeleechResult {
	r := FreeleechResult{SiteID: s.ID, SiteName: s.Name, Unsupported: !c.FreeleechWired(s)}
	switch {
	case errors.Is(e, site.ErrNotLoggedIn):
		ns = []site.SiteFreeleech{{SiteID: s.ID, SiteName: s.Name, Status: site.FreeleechFailed,
			Title: "未登录", Content: "cookie 失效，请刷新登录", FetchedAt: time.Now()}}
	case e != nil:
		r.Error = errtext.Text(e)
		slog.Debug("freeleech fetch failed", "site", s.Name, "err", e)
	}
	r.Notices = ns
	return r.Normalize()
}

// SyncFreeleech 单站强制刷新站免（/freeleech/sync/:id）：走与聚合面同一条 flRow 口径，
// 并回写缓存——handler 自行拼结果时，「没接线」判定和中文原因两处都会掉。
func (c *SiteChain) SyncFreeleech(ctx context.Context, s *models.Site) FreeleechResult {
	ns, e := c.fetchFreeleechOnce(ctx, make(chan struct{}, 1), s)
	return c.flRow(s, ns, e)
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
