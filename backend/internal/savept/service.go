package savept

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// saveptURL 是第三方 PT 站点健康监控站。数据公开，无需登录 cookie。
const saveptURL = "https://savept.icu"

// cacheTTL 监护室数据缓存有效期（全局共享，非 per-user）。
const cacheTTL = 6 * time.Hour

// fetchTimeout 抓取 savept.icu 的超时上限。
const fetchTimeout = 30 * time.Second

// Service 抓取并缓存 savept.icu 数据，按用户本地站点做匹配。
type Service struct {
	hc        *httpclient.Client
	siteOp    *oper.SiteOper
	reg       *site.Registry // definitions 注册表；详情里补平台字段（nil 跳过）
	url       string         // 抓取源；默认 saveptURL，测试可注入 httptest 上游
	skipProbe bool           // 跳过独有站连通性探测（测试注入上游时避免对外网发流量）
	sf        singleflight.Group
	mu        sync.Mutex
	cache     *Result
	cachedAt  time.Time
}

// NewService 构造。
func NewService(hc *httpclient.Client, siteOp *oper.SiteOper, reg *site.Registry) *Service {
	return &Service{hc: hc, siteOp: siteOp, reg: reg, url: saveptURL}
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
		// 缓存过期瞬间多个用户并发请求会同时打上游（第三方站，有被 ban 风险），
		// 用 singleflight 合并：只有一个 goroutine 真正抓取，其余共享结果。
		v, err, _ := s.sf.Do("fetch", func() (any, error) {
			if !force {
				// 并发合并进来的调用者：首个 goroutine 可能刚抓完并填了缓存，直接复用。
				// force=true 时不复用——用户主动刷新必须真的重抓上游。
				if hit := s.cached(false); hit != nil {
					return hit, nil
				}
			}
			fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
			defer cancel()
			body, err := s.fetchPage(fctx)
			if err != nil {
				stale := s.staleSnapshot()
				if stale == nil {
					return nil, err
				}
				slog.Warn("savept fetch failed, serving stale cache", "err", err)
				return stale, errStaleFallback
			}
			cards, alerts := parseSavept(string(body))
			if cards == nil {
				cards = []SaveptSiteCard{}
			}
			if alerts == nil {
				alerts = []SaveptAlert{}
			}
			// ptseek 元数据：详情合并（介绍/特色/人数/种子数/logo）+ 独有站追加占位卡。
			// 探测占位站连通性是外部副作用（对 ptseek 300+ 站发请求），仅真实抓取路径执行，
			// 测试上游（s.probeUpstream 同源）时跳过，避免测试对外网发探测流量。
			extra := mergePtseekDetails(cards, ptseekOnce())
			if len(extra) > 0 {
				newCards := extraCards(extra)
				if !s.skipProbe {
					s.probeExtra(ctx, newCards)
				}
				cards = append(cards, newCards...)
			}
			s.applyPlatformDetails(cards)
			res := &Result{
				Cards:     cards,
				Alerts:    alerts,
				FetchedAt: time.Now(),
			}
			s.mu.Lock()
			s.cache = res
			s.cachedAt = time.Now()
			s.mu.Unlock()
			return res, nil
		})
		if err != nil && err != errStaleFallback {
			return nil, err
		}
		base = v.(*Result)
		if err == errStaleFallback {
			degraded = true
		} else {
			// 抓取成功后缓存必已填充（自己填的或共享他人刚填的），此时 fromCache 无意义，
			// 保持 false 让调用方看到「新鲜数据」；仅检查降级共享场景。
			fromCache = false
		}
	}

	// 本地匹配（每次实时，独立副本避免污染缓存）；siteOp 为 nil 时跳过（测试场景）
	out := *base
	out.Cards = append([]SaveptSiteCard(nil), base.Cards...)
	if s.siteOp != nil {
		sites, err := s.siteOp.List()
		if err != nil {
			slog.Warn("savept list local sites failed", "err", err)
		} else {
			matchCards(out.Cards, sites)
		}
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

// fetchPage GET 抓取源首页（URL 可注入，测试用 httptest）。
func (s *Service) fetchPage(ctx context.Context) ([]byte, error) {
	resp, err := s.hc.Get(ctx, s.url, "", true)
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

// errStaleFallback 内部信号：抓取失败但降级返回旧缓存（不向外抛）。
var errStaleFallback = errors.New("savept stale fallback")

// applyPlatformDetails 用本地 definitions 补 detail.schema（平台）。ptseek 已覆盖其余字段，
// 平台信息只有 definitions 有；命中域名即填，未命中跳过。
func (s *Service) applyPlatformDetails(cards []SaveptSiteCard) {
	if s.reg == nil {
		return
	}
	for i := range cards {
		if cards[i].Detail == nil {
			continue
		}
		if id, ok := s.reg.MatchByURL(cards[i].URL); ok {
			if def, ok := s.reg.Definition(id); ok && def.Schema != "" {
				cards[i].Detail.Schema = def.Schema
			}
		}
	}
}

// probeExtra 并发探测 ptseek 独有站的连通性（仅这些卡：savept.icu 原有卡片的状态
// 以数据源为准，不覆盖）。GET 首页：任何完整 HTTP 响应（含 403/5xx——PT 站普遍有 CF 盾）
// 都算「站还活着」；DNS 失败/连接超时/TLS 握手失败或 404/410（域名停放）算不通 → 标 HealthClosed
// （前端渲染为死亡站点）。探测结果进程级缓存：存活站每 10 轮抽查一次，死亡站每轮重试捕捉复活。
const (
	probeConcurrency = 12
	probeTimeout     = 8 * time.Second
)

var (
	probeMu       sync.Mutex
	probeOkMap    = map[string]bool{}
	probeAttempts = 0
)

// probeExtra 探测占位卡连通性并回写 Status/StatusText。
func (s *Service) probeExtra(ctx context.Context, cards []SaveptSiteCard) {
	if s.hc == nil || len(cards) == 0 {
		return
	}
	probeAttempts++
	type job struct {
		idx int
		dom string
		url string
	}
	jobs := make([]job, 0, len(cards))
	probeMu.Lock()
	for i := range cards {
		d := cards[i].Domain
		if d == "" {
			continue
		}
		if ok := probeOkMap[d]; ok && probeAttempts%10 != 1 {
			cards[i].Status = HealthHealthy
			cards[i].StatusText = "运行中"
			continue
		}
		jobs = append(jobs, job{idx: i, dom: d, url: cards[i].URL})
	}
	probeMu.Unlock()
	if len(jobs) == 0 {
		return
	}
	slog.Info("savept probing extra sites", "count", len(jobs))

	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			alive := s.probeOne(ctx, j.url)
			probeMu.Lock()
			probeOkMap[j.dom] = alive
			probeMu.Unlock()
			if alive {
				cards[j.idx].Status = HealthHealthy
				cards[j.idx].StatusText = "运行中"
			} else {
				cards[j.idx].Status = HealthClosed
				cards[j.idx].StatusText = "站点无法访问"
			}
		}(j)
	}
	wg.Wait()
}

// probeOne 探测单站：true=存活。任何完整 HTTP 响应都算活（403/503 是 CF 盾不是死站）；
// 网络层错误（DNS/超时/TLS）或 404/410（停放）算死。
func (s *Service) probeOne(ctx context.Context, rawURL string) bool {
	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	resp, err := s.hc.Get(pctx, rawURL, "", true)
	if err != nil {
		return false
	}
	if resp.StatusCode == 404 || resp.StatusCode == 410 {
		return false
	}
	return true
}
