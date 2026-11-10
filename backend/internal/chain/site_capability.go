// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// HRWired 该站的 H&R 检测是否接了线（站点定义显式 hr.enabled: true）。
// HR 默认关闭（多数内置定义没开），所以调用方必须把「没接线」与「抓到 0 条」分开记账。
func (c *SiteChain) HRWired(s *models.Site) bool { return c.defFor(s).HR.Enabled }

// HR 抓取站点 H&R 列表（适配器需实现 site.HRCapable）。
// 站点定义未显式 hr.enabled: true 时直接返回空，避免对未开启 H&R 的站发请求。
func (c *SiteChain) HR(ctx context.Context, s *models.Site) ([]site.HRTorrent, error) {
	def := c.defFor(s)
	if !def.HR.Enabled {
		return nil, nil
	}
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
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

// SeedingStats 抓取逐条做种列表并按官种维度聚合（适配器需实现 site.SeedingListCapable）。
// 保种契约 OfficialOnly 维度用；不支持逐行解析的站（返回 ErrNotSupported /
// site.ErrSeedingListUnsupported）时 Supported=false，调用方应回退 UserInfo 汇总统计。
func (c *SiteChain) SeedingStats(ctx context.Context, s *models.Site) (*site.SeedingStats, error) {
	def := c.defFor(s)
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
	out := &site.SeedingStats{}
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		lc, ok := a.(site.SeedingListCapable)
		if !ok {
			return ErrNotSupported
		}
		items, e := lc.FetchSeedingList(ctx, def, s.BaseURL, token)
		if e != nil {
			return e
		}
		out.Supported = true
		for _, it := range items {
			out.Total.Count++
			out.Total.Size += it.Size
			if site.IsOfficial(it.Title, it.Labels) {
				out.Official.Count++
				out.Official.Size += it.Size
			}
		}
		return nil
	})
	return out, err
}

// detailCacheTTL 详情页缓存有效期（详情较重，且很少变动；过短则失缓存意义）。
const detailCacheTTL = 10 * time.Minute

// TorrentDetail 抓取并解析单条种子详情页（capability dispatch + 短缓存）。
// 先做 sameHost 守卫：detailURL 必须落在站点 baseURL 的同注册域下，防止用 A 站 cookie
// 探测 B 站（socket 层 ssrf=true 已拦私网 IP，此为授权层）。
// 缓存 miss 时走 singleflight 合并同 key 并发抓取（详情页较重，防击穿）。
func (c *SiteChain) TorrentDetail(ctx context.Context, s *models.Site, detailURL string) (*site.TorrentDetail, error) {
	if !sameHost(s.BaseURL, detailURL) {
		return nil, errors.New("详情页地址与站点首页不同域，已拒绝该请求（防止把本站 cookie 发给别站）")
	}
	key := fmt.Sprintf("%d|%s", s.ID, detailURL)
	// 快速路径：命中缓存直接返回（不经 singleflight）。
	c.detailMu.Lock()
	if e, ok := c.detailCache[key]; ok && time.Since(e.at) <= detailCacheTTL {
		c.detailMu.Unlock()
		return e.detail, nil
	}
	c.detailMu.Unlock()
	// miss → singleflight 合并同 key 并发抓取。
	return sfDo(&c.detailSF, key, func() (*site.TorrentDetail, error) {
		// double-check：leader 完成后紧随的 caller 重跑 fn，缓存可能已满。
		c.detailMu.Lock()
		if e, ok := c.detailCache[key]; ok && time.Since(e.at) <= detailCacheTTL {
			c.detailMu.Unlock()
			return e.detail, nil
		}
		c.detailMu.Unlock()
		return c.fetchTorrentDetail(ctx, s, detailURL, key)
	})
}

// fetchTorrentDetail 调度适配器抓取并解析详情页，成功时写短缓存并清理过期条目。
func (c *SiteChain) fetchTorrentDetail(ctx context.Context, s *models.Site, detailURL, key string) (*site.TorrentDetail, error) {
	def := c.defFor(s)
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
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
	// 同注册域视为同站（site.RegisteredDomain 对无公共后缀的 IP/单标签主机回退小写裸 host，
	// 与原 EqualFold 精确主机比对等价）。
	return site.RegisteredDomain(b.Hostname()) == site.RegisteredDomain(c.Hostname())
}
