package savept

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// saveptURL 是第三方 PT 站点健康监控站。数据公开，无需登录 cookie。
const saveptURL = "https://savept.icu"

// cacheTTL 监护室数据缓存有效期（全局共享，非 per-user）。
const cacheTTL = 6 * time.Hour

// fetchTimeout 抓取 savept.icu 的超时上限。
const fetchTimeout = 30 * time.Second

// Service 抓取并缓存 savept.icu 数据，按用户本地站点做匹配。
type Service struct {
	hc       *httpclient.Client
	siteOp   *oper.SiteOper
	mu       sync.Mutex
	cache    *Result
	cachedAt time.Time
}

// NewService 构造。
func NewService(hc *httpclient.Client, siteOp *oper.SiteOper) *Service {
	return &Service{hc: hc, siteOp: siteOp}
}

// Fetch 返回监护室结果（卡片 + 公告 + 汇总 + 本地匹配）。
// 全局抓取结果缓存 6h（force 或过期重抓）；本地站点匹配每次实时计算（站点可能随时增删）。
// 抓取失败（Cloudflare 拦截 / 网络异常）时，若存在旧缓存则降级返回并标记 Stale，
// 避免整页空白；仅当无任何缓存时才向上抛错。
func (s *Service) Fetch(ctx context.Context, force bool) (*Result, error) {
	base := s.cached(force)
	fromCache := base != nil // 命中未过期缓存
	degraded := false        // 抓取失败、降级返回旧缓存
	if base == nil {
		fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
		defer cancel()
		body, err := s.fetchPage(fctx)
		if err != nil {
			stale := s.staleSnapshot()
			if stale == nil {
				return nil, err
			}
			base = stale
			degraded = true
			slog.Warn("savept fetch failed, serving stale cache", "err", err)
		} else {
			cards, alerts := parseSavept(string(body))
			if cards == nil {
				cards = []SaveptSiteCard{}
			}
			if alerts == nil {
				alerts = []SaveptAlert{}
			}
			base = &Result{
				Cards:     cards,
				Alerts:    alerts,
				FetchedAt: time.Now(),
			}
			s.mu.Lock()
			s.cache = base
			s.cachedAt = time.Now()
			s.mu.Unlock()
		}
	}

	// 本地匹配（每次实时，独立副本避免污染缓存）
	out := *base
	out.Cards = append([]SaveptSiteCard(nil), base.Cards...)
	sites, err := s.siteOp.List()
	if err != nil {
		slog.Warn("savept list local sites failed", "err", err)
	} else {
		matchCards(out.Cards, sites)
	}
	out.Summary = summarize(out.Cards)
	out.Cached = fromCache || degraded
	out.Stale = degraded
	if degraded {
		out.Notice = "savept.icu 抓取失败（可能被 Cloudflare 拦截），以下为缓存快照，数据可能已过期"
	}
	return &out, nil
}

// cached 返回未过期的缓存副本（force 时返回 nil 触发重抓）。
func (s *Service) cached(force bool) *Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if force || s.cache == nil || time.Since(s.cachedAt) > cacheTTL {
		return nil
	}
	return s.cache
}

// staleSnapshot 返回当前缓存的只读快照（不判过期）。fetch 失败时降级用——
// 即使缓存已超过 cacheTTL，也优先返回它而非空白。
func (s *Service) staleSnapshot() *Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cache
}

// fetchPage GET savept.icu 首页。
func (s *Service) fetchPage(ctx context.Context) ([]byte, error) {
	resp, err := s.hc.Get(ctx, saveptURL, "", true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, errBlocked
	}
	return resp.Body, nil
}

// errBlocked 表示被 Cloudflare 拦截（savept.icu 偶发）。
type cloudflareErr struct{}

var errBlocked = cloudflareErr{}

func (cloudflareErr) Error() string { return "savept.icu blocked by cloudflare" }
