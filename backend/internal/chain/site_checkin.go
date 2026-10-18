package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/site"
)

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
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
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
		rec.Error = errtext.Text(err)
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
// CountCheckinRecords 签到记录总数（与 ListCheckinRecords 同筛选）。
func (c *SiteChain) CountCheckinRecords(siteID uint) (int64, error) {
	if c.records == nil {
		return 0, errors.New("checkin record store unavailable")
	}
	return c.records.Count(siteID)
}

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

// checkinOneSite 跑单站签到并产出该站结果。兜住 panic：safego.Go 只记日志，
// 不兜住则 results[i] 停在零值——一键签到的结果里出现一条「无名、无错误」的记录，
// 用户既不知道是哪个站出的问题，也不知道这个站到底没签上。
func (c *SiteChain) checkinOneSite(ctx context.Context, s *models.Site) (r CheckinSiteResult) {
	r = CheckinSiteResult{SiteID: s.ID, SiteName: s.Name}
	defer func() {
		if p := recover(); p != nil {
			slog.Error("site checkin panic", "site", s.Name, "panic", p)
			r = CheckinSiteResult{SiteID: s.ID, SiteName: s.Name, Error: fmt.Sprintf("签到异常(panic): %v", p)}
		}
	}()
	siteCtx, cancel := context.WithTimeout(ctx, checkinTimeout)
	defer cancel()

	res, e := c.Checkin(siteCtx, s, "manual")
	if e != nil {
		// ErrNotSupported（如 mteam 未实现 CheckinCapable）/ 网络错误均聚合到 Error，不阻断其它站。
		r.Error = errtext.Text(e)
		return r
	}
	if res != nil {
		r.Success = res.Success
		r.Message = res.Message
		r.Bonus = res.Bonus
	}
	return r
}

// CheckinAll 跨所有启用站点并发签到（每站独立超时；容错：单站失败不影响其它）。
// 各 goroutine 写入预分配 results 的不同下标，无需加锁；并发安全依据同 SearchSites。
//
// 「站点清单本身读不出来」必须回错误：从前这里 `return nil`，于是
// 一键签到回 `{"results": null}`、工作流节点播报「0/0 成功」、MCP 工具回空数组、
// 移动端与 Web 各自把「读不到」讲成「没有启用的站点」——四种说法都是同一种谎。
func (c *SiteChain) CheckinAll(ctx context.Context) ([]CheckinSiteResult, error) {
	sites, err := c.sites.ListActive()
	if err != nil {
		return nil, fmt.Errorf("读取启用站点清单失败: %w", err)
	}
	results := make([]CheckinSiteResult, len(sites))
	sem := make(chan struct{}, searchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		safego.Go("site:601", func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = c.checkinOneSite(ctx, &s)
		})
	}
	wg.Wait()
	return results, nil
}

// KeepAlive 保活站点 cookie（适配器需实现 site.KeepAliveCapable）。
// 做一次认证 GET 刷新 session，并返回轮换后的最新 cookie（若有）。
// 持有 per-site 锁，避免共享 cookie jar 并发污染；调用方无需再加锁。
func (c *SiteChain) KeepAlive(ctx context.Context, s *models.Site) (*site.KeepAliveResult, error) {
	lk := c.siteLock(s.ID)
	lk.Lock()
	defer lk.Unlock()
	return c.keepAliveLocked(ctx, s)
}

// keepAliveLocked 是 KeepAlive 的无锁内核（调用方已持有 siteLock）。
// 将适配器返回的 jar cookie 与原凭证合并，仅在语义变化时填 RefreshedCookie。
func (c *SiteChain) keepAliveLocked(ctx context.Context, s *models.Site) (*site.KeepAliveResult, error) {
	def := c.defFor(s)
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
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
	if err != nil || res == nil {
		return res, err
	}
	merged := site.MergeCookieHeader(token, res.RefreshedCookie)
	if merged == "" {
		merged = token
	}
	if site.CookieHeaderChanged(token, merged) {
		res.RefreshedCookie = merged
	} else {
		// 无变化：清空，调用方跳过落库
		res.RefreshedCookie = ""
	}
	return res, nil
}
