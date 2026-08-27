package chain

import (
	"context"
	"sync"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// ---- 站点求种请求（对标 MSaber「站点请求」模块） ----

// RequestResult 单站求种列表抓取结果。
type RequestResult struct {
	SiteID   uint              `json:"site_id"`
	SiteName string            `json:"site_name"`
	Requests []site.SiteRequest `json:"requests"`
	Error    string            `json:"error,omitempty"`
}

// SiteOper 暴露站点 oper（handler 单站查询用；只读语义）。
func (c *SiteChain) SiteOper() *oper.SiteOper { return c.sites }

// Requests 抓取单站求种列表（适配器需实现 site.RequestCapable；NexusPHP 系已实现）。
// filled=true 取已满足段。
func (c *SiteChain) Requests(ctx context.Context, s *models.Site, filled bool) (*RequestResult, error) {
	def := c.defFor(s)
	token := c.credential(s, def)
	out := &RequestResult{SiteID: s.ID, SiteName: s.Name, Requests: []site.SiteRequest{}}
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		rc, ok := a.(site.RequestCapable)
		if !ok {
			return ErrNotSupported
		}
		reqs, e := rc.FetchRequests(ctx, def, s.BaseURL, token, filled)
		if e != nil {
			return e
		}
		out.Requests = reqs
		return nil
	})
	if err != nil {
		out.Error = err.Error()
	}
	return out, err
}

// RequestsAll 跨站并发聚合求种列表（不含缓存——求种列表低频查看，现取现返回）。
func (c *SiteChain) RequestsAll(ctx context.Context, filled bool) []RequestResult {
	sites, err := c.sites.ListActive()
	if err != nil {
		return nil
	}
	results := make([]RequestResult, len(sites))
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		safego.Go("site:requests", func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sctx, cancel := context.WithTimeout(ctx, infoFetchTimeout)
			defer cancel()
			r, e := c.Requests(sctx, &s, filled)
			if r != nil {
				results[i] = *r
			} else if e != nil {
				results[i] = RequestResult{SiteID: s.ID, SiteName: s.Name, Error: e.Error()}
			}
		})
	}
	wg.Wait()
	return results
}
