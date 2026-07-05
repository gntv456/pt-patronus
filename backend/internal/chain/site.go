package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"golang.org/x/net/publicsuffix"
)

// SiteResult 是单站搜索结果（含可能的错误，用于聚合容错）。
type SiteResult struct {
	SiteID   uint           `json:"site_id"`
	SiteName string         `json:"site_name"`
	Torrents []site.Torrent `json:"torrents"`
	Error    string         `json:"error,omitempty"`
}

// SiteChain 编排站点相关业务：搜索、连接测试、用户信息。
type SiteChain struct {
	Base
	sites    *oper.SiteOper
	registry *site.Registry
	records  *oper.CheckinRecordOper // 签到历史落库（可选；未注入则不落库，测试用）
	checkinGuard *spendGuard // 签到 in-flight 去重（防双击/重放/cron 重叠重复签到）

	// 站免/公告 per-site 缓存（站点是全局共享配置，故缓存按 siteID 而非 uid 分桶）。
	flMu     sync.Mutex
	flCache  map[uint]flCacheEntry
	annMu    sync.Mutex
	annCache map[uint]annCacheEntry
	// 后宫 per-site 缓存（邀请库存 + 被邀请人）。
	haremMu    sync.Mutex
	haremCache map[uint]haremCacheEntry
	// 种子详情 per-(site|url) 缓存（详情页较重，短缓存减重复抓取）。
	detailMu    sync.Mutex
	detailCache map[string]detailCacheEntry
}

// flCacheEntry / annCacheEntry 是 per-site 缓存条目。
type (
	flCacheEntry struct {
		notices []site.SiteFreeleech
		at      time.Time
	}
	annCacheEntry struct {
		ann *site.SiteAnnouncement
		at  time.Time
	}
	haremCacheEntry struct {
		status   *site.InviteStatus
		invitees []site.InviteeInfo
		at       time.Time
	}
	detailCacheEntry struct {
		detail *site.TorrentDetail
		at     time.Time
	}
)

// NewSiteChain 构造 SiteChain。
func NewSiteChain(b Base, sites *oper.SiteOper, registry *site.Registry) *SiteChain {
	return &SiteChain{
		Base:     b,
		sites:    sites,
		registry: registry,
		checkinGuard: newSpendGuard(),
		flCache:    make(map[uint]flCacheEntry),
		annCache:   make(map[uint]annCacheEntry),
		haremCache: make(map[uint]haremCacheEntry),
		detailCache: make(map[string]detailCacheEntry),
	}
}

// defFor 返回站点对应的定义；无则回退为仅含 schema 的最小定义。
func (c *SiteChain) defFor(s *models.Site) *site.SiteMetadata {
	// 统一经 EnsureDefaults：站点 YAML 省略的 capability（announcement/messages/freeleech/
	// harem/detail 等）补 NexusPHP 默认 Enabled=true + 默认路径，避免被各 chain 的「!Enabled」
	// 守卫误跳过（如好学 hxpt.yaml 未配 announcement/messages 段 → Enabled=false → 公告/消息恒空）。
	if s.Definition != "" {
		if def, ok := c.registry.Definition(s.Definition); ok {
			return site.EnsureDefaults(def)
		}
	}
	return site.EnsureDefaults(&site.SiteMetadata{ID: s.Definition, Name: s.Name, Schema: s.Platform, URLs: []string{s.BaseURL}})
}

// credential 返回传给适配器的鉴权凭据：MTeam 用 API key（x-api-key），Avistaz 用
// username+password+pid 打包成 JSON 单串（适配器 parseAvistazCred 拆），其余站点用 cookie。
// 集中 schema 感知，未来更多凭据类型可在此扩展。
func (c *SiteChain) credential(s *models.Site, def *site.SiteMetadata) string {
	if def != nil {
		switch def.Schema {
		case "mteam":
			if k, err := c.sites.APIKey(s); err == nil {
				return k
			}
			return ""
		case "avistaz":
			// 三字段打包：Site.Username + Password() + DecryptedPasskey()（pid）。
			pwd, _ := c.sites.Password(s)
			pid := c.sites.DecryptedPasskey(s)
			b, _ := json.Marshal(map[string]string{
				"username": s.Username,
				"password": pwd,
				"pid":      pid,
			})
			return string(b)
		}
	}
	cookie, _ := c.sites.Cookie(s)
	return cookie
}

// dispatchAdapter 找到匹配 schema 的适配器并执行 fn。
// 区分两种情况：无适配器匹配返回 ErrNoAdapter；匹配但 fn 失败则返回该真实错误。
func (c *SiteChain) dispatchAdapter(ctx context.Context, schema string, fn func(site.Adapter) error) error {
	var (
		matched bool
		fnErr   error
	)
	RunModule(ctx, &c.Base, func(m module.Module) (struct{}, module.Control) {
		a, ok := m.(site.Adapter)
		if !ok || a.Schema() != schema {
			return struct{}{}, module.Skip
		}
		matched = true
		fnErr = fn(a)
		return struct{}{}, module.Stop
	})
	if !matched {
		return ErrNoAdapter
	}
	return fnErr
}

// ErrNoAdapter 表示没有已注册的适配器能处理该 schema。
var ErrNoAdapter = errors.New("no registered adapter for site schema")

// ErrNotSupported 表示适配器不支持该操作（如签到）。
var ErrNotSupported = errors.New("operation not supported by adapter")

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
		go func(i int, s models.Site) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			siteCtx, cancel := context.WithTimeout(ctx, searchTimeout)
			defer cancel()

			ts, e := c.Search(siteCtx, &s, keyword, category, 1)
			r := SiteResult{SiteID: s.ID, SiteName: s.Name, Torrents: ts}
			if e != nil {
				r.Torrents = nil
				r.Error = e.Error()
				slog.Debug("site search failed", "site", s.Name, "err", e)
			}
			results[i] = r
		}(i, s)
	}
	wg.Wait()
	return results
}

// TestConnection 测试站点连接与登录态，返回用户等级。
func (c *SiteChain) TestConnection(ctx context.Context, s *models.Site) (string, error) {
	ui, err := c.UserInfo(ctx, s)
	if err != nil {
		return "", err
	}
	return ui.Level, nil
}

// UserInfo 抓取并返回站点用户资料（自动化统计同步用）。
func (c *SiteChain) UserInfo(ctx context.Context, s *models.Site) (*site.UserInfo, error) {
	def := c.defFor(s)
	token := c.credential(s, def)
	var ui *site.UserInfo
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		u, e := a.FetchUserInfo(ctx, def, s.BaseURL, token)
		if e != nil {
			return e
		}
		ui = u
		return nil
	})
	return ui, err
}

// Checkin 对站点执行签到（适配器需实现 site.CheckinCapable），并落库一条签到历史。
// source 标记录来源：cron（定时自动）/ manual（手动触发，含一键签到）。
func (c *SiteChain) Checkin(ctx context.Context, s *models.Site, source string) (*site.CheckinResult, error) {
	// in-flight 去重：防双击/重放/cron 重叠对同一站点重复签到（风险站点可能重复发魔力 + 记录重复落库）。
	guardKey := fmt.Sprintf("checkin:%d", s.ID)
	if !c.checkinGuard.acquire(guardKey) {
		return nil, errors.New("签到正在进行中，请勿重复触发")
	}
	defer c.checkinGuard.release(guardKey)
	def := c.defFor(s)
	token := c.credential(s, def)
	var result *site.CheckinResult
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		cc, ok := a.(site.CheckinCapable)
		if !ok {
			return ErrNotSupported
		}
		r, e := cc.Checkin(ctx, def, s.BaseURL, token)
		if e != nil {
			return e
		}
		result = r
		return nil
	})
	// 不支持签到的站（mteam 等）不落库，避免历史噪音；其余结果（成功/失败/未登录/网络错误）均记。
	if !errors.Is(err, ErrNotSupported) {
		c.recordCheckin(s, source, result, err)
	}
	return result, err
}

// SetRecordOper 注入签到记录 oper（落库签到历史）。可选：未注入时签到不落库（单测用）。
func (c *SiteChain) SetRecordOper(op *oper.CheckinRecordOper) {
	c.records = op
}

// recordCheckin 落库一条签到记录（成功/失败均记；oper 未注入则跳过）。
func (c *SiteChain) recordCheckin(s *models.Site, source string, r *site.CheckinResult, err error) {
	if c.records == nil {
		return
	}
	rec := &models.CheckinRecord{SiteID: s.ID, SiteName: s.Name, Source: source}
	if err != nil {
		rec.Error = err.Error()
	} else if r != nil {
		rec.Success = r.Success
		rec.Message = r.Message
		rec.Bonus = r.Bonus
	}
	if e := c.records.Create(rec); e != nil {
		slog.Warn("record checkin failed", "site", s.Name, "err", e)
	}
}

// ListCheckinRecords 列签到历史（按时间倒序）。siteID=0 不过滤；limit<=0 默认 100。
func (c *SiteChain) ListCheckinRecords(siteID uint, limit, offset int) ([]models.CheckinRecord, error) {
	if c.records == nil {
		return nil, nil
	}
	return c.records.List(siteID, limit, offset)
}

// CheckinSiteResult 是单站签到聚合结果（一键签到用；含错误，用于聚合容错）。
type CheckinSiteResult struct {
	SiteID   uint    `json:"site_id"`
	SiteName string  `json:"site_name"`
	Success  bool    `json:"success"`
	Message  string  `json:"message,omitempty"`
	Bonus    float64 `json:"bonus"`
	Error    string  `json:"error,omitempty"`
}

// checkinTimeout 是单站签到超时上限（签到是 POST，部分站点响应慢，给足时间）。
const checkinTimeout = 30 * time.Second

// CheckinAll 跨所有启用站点并发签到（每站独立超时；容错：单站失败不影响其它）。
// 各 goroutine 写入预分配 results 的不同下标，无需加锁；并发安全依据同 SearchSites。
func (c *SiteChain) CheckinAll(ctx context.Context) []CheckinSiteResult {
	sites, err := c.sites.ListActive()
	if err != nil {
		return nil
	}
	results := make([]CheckinSiteResult, len(sites))
	sem := make(chan struct{}, searchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		go func(i int, s models.Site) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			siteCtx, cancel := context.WithTimeout(ctx, checkinTimeout)
			defer cancel()

			r := CheckinSiteResult{SiteID: s.ID, SiteName: s.Name}
			res, e := c.Checkin(siteCtx, &s, "manual")
			if e != nil {
				// ErrNotSupported（如 mteam 未实现 CheckinCapable）/ 网络错误均聚合到 Error，不阻断其它站。
				r.Error = e.Error()
			} else if res != nil {
				r.Success = res.Success
				r.Message = res.Message
				r.Bonus = res.Bonus
			}
			results[i] = r
		}(i, s)
	}
	wg.Wait()
	return results
}

// KeepAlive 保活站点 cookie（适配器需实现 site.KeepAliveCapable）。
// 做一次认证 GET 刷新 session，并返回轮换后的最新 cookie（若有）。
func (c *SiteChain) KeepAlive(ctx context.Context, s *models.Site) (*site.KeepAliveResult, error) {
	def := c.defFor(s)
	token := c.credential(s, def)
	var res *site.KeepAliveResult
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		kc, ok := a.(site.KeepAliveCapable)
		if !ok {
			return ErrNotSupported
		}
		r, e := kc.KeepAlive(ctx, def, s.BaseURL, token)
		if e != nil {
			return e
		}
		res = r
		return nil
	})
	return res, err
}

// HR 抓取站点 H&R 列表（适配器需实现 site.HRCapable）。
// 站点定义未显式 hr.enabled: true 时直接返回空，避免对未开启 H&R 的站发请求。
func (c *SiteChain) HR(ctx context.Context, s *models.Site) ([]site.HRTorrent, error) {
	def := c.defFor(s)
	if !def.HR.Enabled {
		return nil, nil
	}
	token := c.credential(s, def)
	var out []site.HRTorrent
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		hc, ok := a.(site.HRCapable)
		if !ok {
			return ErrNotSupported
		}
		ts, e := hc.FetchHR(ctx, def, s.BaseURL, token)
		if e != nil {
			return e
		}
		out = ts
		return nil
	})
	return out, err
}

// detailCacheTTL 详情页缓存有效期（详情较重，且很少变动；过短则失缓存意义）。
const detailCacheTTL = 10 * time.Minute

// TorrentDetail 抓取并解析单条种子详情页（capability dispatch + 短缓存）。
// 先做 sameHost 守卫：detailURL 必须落在站点 baseURL 的同注册域下，防止用 A 站 cookie
// 探测 B 站（socket 层 ssrf=true 已拦私网 IP，此为授权层）。
func (c *SiteChain) TorrentDetail(ctx context.Context, s *models.Site, detailURL string) (*site.TorrentDetail, error) {
	if !sameHost(s.BaseURL, detailURL) {
		return nil, errors.New("detail url host does not match site base url")
	}
	key := fmt.Sprintf("%d|%s", s.ID, detailURL)
	c.detailMu.Lock()
	if e, ok := c.detailCache[key]; ok && time.Since(e.at) <= detailCacheTTL {
		c.detailMu.Unlock()
		return e.detail, nil
	}
	c.detailMu.Unlock()

	def := c.defFor(s)
	token := c.credential(s, def)
	var out *site.TorrentDetail
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		dc, ok := a.(site.TorrentDetailCapable)
		if !ok {
			return ErrNotSupported
		}
		d, e := dc.FetchTorrentDetail(ctx, def, s.BaseURL, token, detailURL)
		if e != nil {
			return e
		}
		out = d
		return nil
	})
	if err == nil && out != nil {
		c.detailMu.Lock()
		// 顺带清理过期条目，防 detailCache 无限增长（按 siteID|URL 为键，无自然上限）。
		now := time.Now()
		for k, e := range c.detailCache {
			if now.Sub(e.at) > detailCacheTTL {
				delete(c.detailCache, k)
			}
		}
		c.detailCache[key] = detailCacheEntry{detail: out, at: now}
		c.detailMu.Unlock()
	}
	return out, err
}

// sameHost 判断 candidateURL 是否落在 baseURL 的同注册域下（同站 cookie 不外泄守卫）。
//
// 用 publicsuffix.EffectiveTLDPlusOne 比对注册域，正确处理 .co.uk / .com.cn 等多段
// 公共后缀（旧实现只取最后两段，会把 a.co.uk 与 b.co.uk 误判为同注册域）。
// 相对/绝对路径先用 base 解析归一：相对路径天然同源放行；非 http(s) 协议一律拒绝。
// 解析失败或无公共后缀（裸 IP / 单标签主机）时回退精确主机比对。
func sameHost(baseURL, candidateURL string) bool {
	b, err := url.Parse(baseURL)
	if err != nil || b.Host == "" {
		return false
	}
	cref, err := url.Parse(candidateURL)
	if err != nil {
		return false
	}
	c := b.ResolveReference(cref)
	if c.Host == "" || (c.Scheme != "http" && c.Scheme != "https") {
		return false
	}
	if br, err := publicsuffix.EffectiveTLDPlusOne(b.Hostname()); err == nil {
		if dr, err := publicsuffix.EffectiveTLDPlusOne(c.Hostname()); err == nil {
			return strings.EqualFold(br, dr)
		}
	}
	// 回退：无公共后缀（IP / 单标签主机）→ 精确主机比对。
	return strings.EqualFold(b.Hostname(), c.Hostname())
}
