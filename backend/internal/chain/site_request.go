// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/site"
)

// ---- 站点求种请求 ----

// RequestResult 单站求种列表抓取结果。
type RequestResult struct {
	SiteID   uint               `json:"site_id"`
	SiteName string             `json:"site_name"`
	Requests []site.SiteRequest `json:"requests"`
	Error    string             `json:"error,omitempty"`
}

// SiteOper 暴露站点 oper（handler 单站查询用；只读语义）。
func (c *SiteChain) SiteOper() *oper.SiteOper { return c.sites }

// Requests 抓取单站求种列表（适配器需实现 site.RequestCapable；NexusPHP 系已实现）。
// filled=true 取已满足段。
func (c *SiteChain) Requests(ctx context.Context, s *models.Site, filled bool) (*RequestResult, error) {
	def := c.defFor(s)
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
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
		out.Error = errtext.Text(err)
	}
	return out, err
}

// RequestsAll 跨站并发聚合求种列表（不含缓存——求种列表低频查看，现取现返回）。
// 站点清单读失败交回错误，不渲染成「全部站无求种」假绿。
func (c *SiteChain) RequestsAll(ctx context.Context, filled bool) ([]RequestResult, error) {
	sites, err := c.sites.ListActive()
	if err != nil {
		return nil, err
	}
	results := make([]RequestResult, len(sites))
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		safego.Go("site:requests", func() {
			defer wg.Done()
			// per-site 隔离：崩站交出它自己那一行（站名 + 空数组 + 原因）。零值槽是
			// site_id=0 + requests=nil，既匿名又会让前端 .length 崩。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("requests site panic", "site", s.Name, "err", r)
					results[i] = RequestResult{
						SiteID:   s.ID,
						SiteName: s.Name,
						Requests: []site.SiteRequest{},
						Error:    fmt.Sprintf("求种抓取异常: %v", r),
					}
				}
			}()
			sem <- struct{}{}
			defer func() { <-sem }()
			sctx, cancel := context.WithTimeout(ctx, infoFetchTimeout)
			defer cancel()
			r, e := c.Requests(sctx, &s, filled)
			if r != nil {
				results[i] = *r
			} else if e != nil {
				results[i] = RequestResult{SiteID: s.ID, SiteName: s.Name, Error: errtext.Text(e)}
			}
		})
	}
	wg.Wait()
	return results, nil
}
