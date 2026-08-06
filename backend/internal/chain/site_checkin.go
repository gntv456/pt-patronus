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
		safego.Go("site:601", func() {
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
		})
	}
	wg.Wait()
	return results
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
