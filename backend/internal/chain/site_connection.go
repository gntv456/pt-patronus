// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/site"
)

// connPanicResult 崩站的连通结果（JSON 面与流式面共用，防两处口径漂移）。
// 前端连通性只有 connected/disconnected/disabled 三态（api/site.ts SiteConnectivityState），
// 零值槽的 Status="" 既不算连上也算不上失败 ⇒ 这一站在矩阵里静默消失；按失败路径同口径记 disconnected。
func connPanicResult(s models.Site, r any) SiteConnectionResult {
	return SiteConnectionResult{
		SiteID:    s.ID,
		SiteName:  s.Name,
		Status:    "disconnected",
		Error:     fmt.Sprintf("连通检测异常: %v", r),
		CheckedAt: time.Now(),
	}
}

// TestConnections 并发检查全部站点的网络连接与登录态。
// 停用站点不发起网络请求，直接返回 disabled；启用站点各自有独立超时，单站失败不影响其它站点。
func (c *SiteChain) TestConnections(ctx context.Context) ([]SiteConnectionResult, error) {
	sites, err := c.sites.List()
	if err != nil {
		return nil, err
	}
	results := make([]SiteConnectionResult, len(sites))
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		if !s.IsActive {
			results[i] = SiteConnectionResult{SiteID: s.ID, SiteName: s.Name, Status: "disabled", Error: "站点已停用"}
			continue
		}
		wg.Add(1)
		safego.Go("site:400", func() {
			defer wg.Done()
			// per-site 隔离：崩站记 disconnected + 原因，并刷新连通缓存（否则 TTL 内下次进
			// 页面还会被推成上次那条 green）。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("connectivity site panic", "site", s.Name, "err", r)
					res := connPanicResult(s, r)
					_ = c.sites.UpsertConnectivity(s.ID, res.Status, "", res.Error, 0)
					results[i] = res
				}
			}()
			sem <- struct{}{}
			defer func() { <-sem }()

			siteCtx, cancel := context.WithTimeout(ctx, infoFetchTimeout)
			defer cancel()
			level, e := c.TestConnection(siteCtx, &s)
			result := SiteConnectionResult{SiteID: s.ID, SiteName: s.Name, Level: level}
			if e != nil {
				result.Status = "disconnected"
				result.Error = errtext.Text(e)
			} else {
				result.Status = "connected"
			}
			results[i] = result
		})
	}
	wg.Wait()
	return results, nil
}

// TestConnectionsStream 流式并发检查全部站点：每站检测完即回调 emit（由 SSE handler 推给前端，
// 实现「检测到一个更新一个」）。并发骨架与 TestConnections 一致——infoFetchConcurrency 上限、
// 每站 infoFetchTimeout 独立超时、停用站直接 disabled（不发网络请求）。连通结果由 TestConnection
// 顺带 upsert 进 site_connectivity 缓存；停用站在此处单独 upsert。emit 无返回值——客户端断开时
// 链路靠 request ctx 取消自然收敛（各站 siteCtx 随之取消），与 SearchSitesStream 同款容错。
func (c *SiteChain) TestConnectionsStream(ctx context.Context, emit func(SiteConnectionResult)) string {
	sites, err := c.sites.List()
	if err != nil {
		slog.Warn("TestConnectionsStream list sites failed", "err", err)
		return err.Error()
	}
	// 新鲜度节流：进页面时若某站最近 connectivityFreshTTL 内已检测过（connected/disconnected），
	// 直接复用 site_connectivity 缓存结果推送，跳过网络探测——兑现「进页面显示上次结果 + 后台仅刷新过期站」
	// 的设计意图，避免每次打开页面都全量重测所有站（28s→近瞬时）。从未检测或已过期的站才走真实并发探测。
	// 注意：仅 SSE 流式路径节流；手动 TestConnections / 单站 Test 仍强制实测。
	fresh := make(map[uint]models.SiteConnectivity, len(sites))
	if cache, err := c.sites.ListConnectivity(); err == nil {
		for _, sc := range cache {
			fresh[sc.SiteID] = sc
		}
	}
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for i := range sites {
		s := sites[i]
		if !s.IsActive {
			_ = c.sites.UpsertConnectivity(s.ID, "disabled", "", "站点已停用", 0)
			emit(SiteConnectionResult{SiteID: s.ID, SiteName: s.Name, Status: "disabled", Error: "站点已停用", CheckedAt: time.Now()})
			continue
		}
		// 新鲜缓存直接推送，跳过网络探测（缓存状态为 disabled 表示上次检测时站点停用，现已激活须重测）
		if sc, ok := fresh[s.ID]; ok && sc.Status != "" && sc.Status != "disabled" && time.Since(sc.CheckedAt) < connectivityFreshTTL {
			emit(SiteConnectionResult{SiteID: s.ID, SiteName: s.Name, Status: sc.Status, Level: sc.Level, Error: sc.ErrorMsg, CheckedAt: sc.CheckedAt})
			continue
		}
		wg.Add(1)
		safego.Go("site:458", func() {
			defer wg.Done()
			// per-site 隔离：崩站必须 emit 一帧（前端「检测到一个更新一个」，少一帧就是少一站）。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("connectivity stream site panic", "site", s.Name, "err", r)
					res := connPanicResult(s, r)
					_ = c.sites.UpsertConnectivity(s.ID, res.Status, "", res.Error, 0)
					emit(res)
				}
			}()
			sem <- struct{}{}
			defer func() { <-sem }()
			siteCtx, cancel := context.WithTimeout(ctx, infoFetchTimeout)
			defer cancel()
			level, e := c.TestConnection(siteCtx, &s) // 内部已 upsert 连通缓存
			r := SiteConnectionResult{SiteID: s.ID, SiteName: s.Name, Level: level, CheckedAt: time.Now()}
			if e != nil {
				r.Status = "disconnected"
				r.Error = errtext.Text(e)
			} else {
				r.Status = "connected"
			}
			emit(r)
		})
	}
	wg.Wait()
	return ""
}

// TestConnection 测试站点连接与登录态，返回用户等级；同时把结果 upsert 进 site_connectivity 缓存，
// 供站点运营矩阵/Dashboard 进页面瞬时显示「上次检测结果」（connected/disconnected 三态，含失败）。
// 因此 TestConnections、流式 TestConnectionsStream、手动 Test、workflow 节点、CookieCloud 同步
// 任一路径检测后，缓存都会被刷新——下次进页面即看到本次结果。计时计入 AvgSeconds（成功才用）。
func (c *SiteChain) TestConnection(ctx context.Context, s *models.Site) (string, error) {
	start := time.Now()
	ui, err := c.UserInfo(ctx, s)
	secs := time.Since(start).Seconds()
	if err != nil {
		_ = c.sites.UpsertConnectivity(s.ID, "disconnected", "", err.Error(), secs)
		return "", err
	}
	if ui == nil {
		_ = c.sites.UpsertConnectivity(s.ID, "disconnected", "", "站点未返回用户信息", secs)
		return "", errors.New("站点未返回用户信息")
	}
	_ = c.sites.UpsertConnectivity(s.ID, "connected", ui.Level, "", secs)
	return ui.Level, nil
}

// UserInfo 抓取并返回站点用户资料（自动化统计同步用）。
func (c *SiteChain) UserInfo(ctx context.Context, s *models.Site) (*site.UserInfo, error) {
	def := c.defFor(s)
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
	var ui *site.UserInfo
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		u, e := a.FetchUserInfo(ctx, def, s.BaseURL, token)
		if e != nil {
			return e
		}
		ui = u
		return nil
	})
	if err == nil {
		c.maybeAdoptPasskey(s, def, ui)
	}
	return ui, err
}

// maybeAdoptPasskey 在适配器从站点端点发现 passkey（如 FluxTorrent ptppUserInfo）且
// 站点尚未存有 passkey 时，自动加密回写——用户首次只需填 API Token，下载链即可用。
// 站点已存 passkey 时不覆盖（尊重用户手填值）；本次抓取未发现 passkey 则跳过。
// discovered 只能来自本次调用的返回值：适配器实例跨站点共享，任何实例级中转都会在并发
// TestConnections 时将 A 站的 passkey 写进 B 站（实测串站）。
func (c *SiteChain) maybeAdoptPasskey(s *models.Site, def *site.SiteMetadata, ui *site.UserInfo) {
	if s == nil || def == nil || ui == nil {
		return
	}
	pk := ui.DiscoveredPasskey
	if pk == "" {
		return
	}
	if existing := c.sites.DecryptedPasskey(s); existing != "" {
		return
	}
	if len(pk) < 16 { // FluxTorrent passkey 为 32 位；过短视为无效不落库
		return
	}
	if err := c.sites.UpdatePasskey(s.ID, pk); err != nil {
		slog.Warn("adopt site passkey failed", "site", s.Name, "err", err)
		return
	}
	slog.Info("adopted site passkey from adapter", "site", s.Name, "schema", def.Schema)
}
