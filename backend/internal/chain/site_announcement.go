// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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

// annSFResult 是公告单次回源的 singleflight 结果（ann + 展示用 errMsg）。
type annSFResult struct {
	ann    *site.SiteAnnouncement
	errMsg string
}

// annPanicResult 崩站的兜底行（JSON 面与流式面共用，防两处口径漂移）：Items 经 Normalize 归一为 []。
func annPanicResult(s models.Site, r any) AnnouncementResult {
	return AnnouncementResult{
		SiteID:    s.ID,
		SiteName:  s.Name,
		FetchedAt: time.Now(),
		Error:     fmt.Sprintf("公告抓取异常: %v", r),
	}.Normalize()
}

// AnnouncementsAll 跨站并发聚合公告（带 per-site 缓存；force=true 跳过缓存）。
// 等全部站完成后一次性返回（MCP 工具/cron 用）；前端页面要"加载一个显示一个"用 AnnouncementsStream。
// 站点清单读失败交回错误，不渲染成「全部站无公告」假绿。
func (c *SiteChain) AnnouncementsAll(ctx context.Context, force bool) ([]AnnouncementResult, error) {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("announcements list sites failed", "err", err)
		return nil, err
	}
	results := make([]AnnouncementResult, len(sites))
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		safego.Go("site_info:262", func() {
			defer wg.Done()
			// per-site 隔离：崩站交出它自己那一行（站名 + 空 items + 原因）。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("announcements site panic", "site", s.Name, "err", r)
					results[i] = annPanicResult(s, r)
				}
			}()
			results[i] = c.resolveAnnouncement(ctx, sem, s, force)
		})
	}
	wg.Wait()
	return results, nil
}

// resolveAnnouncement 解析单站公告结果（缓存命中/SWR/miss/force 同聚合逻辑），供聚合与流式复用。
func (c *SiteChain) resolveAnnouncement(ctx context.Context, sem chan struct{}, s models.Site, force bool) AnnouncementResult {
	if !force {
		c.annMu.Lock()
		e, ok := c.annCache[s.ID]
		c.annMu.Unlock()
		// SWR：缓存存在（含已过期）且有 ann → 立即返回旧值（秒回）；过期则后台异步刷新。
		if ok && e.ann != nil {
			if time.Since(e.at) > announcementCacheTTL {
				c.refreshAnnouncementsAsync(s)
			}
			return c.annRow(&s, e.ann, "")
		}
	}
	var res annSFResult
	if force {
		res = c.fetchAnnouncementOnce(ctx, sem, &s)
	} else {
		res, _ = sfDo(&c.annSF, strconv.Itoa(int(s.ID)), func() (annSFResult, error) {
			c.annMu.Lock()
			ce, ok := c.annCache[s.ID]
			c.annMu.Unlock()
			if ok && time.Since(ce.at) <= announcementCacheTTL && ce.ann != nil {
				return annSFResult{ann: ce.ann}, nil
			}
			return c.fetchAnnouncementOnce(ctx, sem, &s), nil
		})
	}
	// nil ann（失败/未接线）由 annRow 合成空行，两条出口同一口径。
	return c.annRow(&s, res.ann, res.errMsg)
}

// AnnouncementsStream 流式聚合：每站解析完立即通过 emit 推送（不等全部），供前端"加载一个显示一个"。
// 缓存命中的站秒级 emit，慢站（context deadline 15s）最后到达；缓存/SWR/负缓存逻辑同聚合。
// emit 由调用方保证并发安全（多 goroutine 同时回调）。
// 返回值：站点清单读失败的错误消息（""=正常），供端点在流末尾补 done 行留痕。
func (c *SiteChain) AnnouncementsStream(ctx context.Context, force bool, emit func(AnnouncementResult)) string {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("announcements stream list sites failed", "err", err)
		return err.Error()
	}
	sem := make(chan struct{}, infoFetchConcurrency)
	var wg sync.WaitGroup
	for _, s := range sites {
		wg.Add(1)
		safego.Go("site_info:319", func() {
			defer wg.Done()
			// per-site 隔离：崩站也必须 emit 一行，否则前端只看到「少一站」。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("announcements stream site panic", "site", s.Name, "err", r)
					emit(annPanicResult(s, r))
				}
			}()
			emit(c.resolveAnnouncement(ctx, sem, s, force))
		})
	}
	wg.Wait()
	return ""
}

// fetchAnnouncementOnce 抓单站公告（占一个 sem 槽、独立超时），成功时写缓存。
// 失败/未登录不钉缓存（announcementCacheTTL 短，下次轮询重试）。
func (c *SiteChain) fetchAnnouncementOnce(ctx context.Context, sem chan struct{}, s *models.Site) annSFResult {
	sem <- struct{}{}
	defer func() { <-sem }()
	siteCtx, cancel := context.WithTimeout(ctx, infoFetchTimeout)
	defer cancel()
	ann, e := c.Announcements(siteCtx, s)
	var errMsg string
	if e != nil {
		// 未登录同样作为错误暴露（与站免路径 FreeleechAll 一致）：否则 cookie 过期会被前端
		// 误显为「暂无公告」，用户无从知晓该站需刷新登录。ErrNotLoggedIn 文本 "not logged in"
		// 不含 "login" 子串，前端 friendlySiteError 匹配不到关键词会落到兜底「该站点获取失败」，
		// 故此处补 "cookie"/"login" 关键词，使其译为「站点登录已失效，请在「站点」页重新配置 Cookie」。
		if errors.Is(e, site.ErrNotLoggedIn) {
			errMsg = "站点登录已失效，请在「站点」页重新配置 Cookie"
		} else {
			errMsg = errtext.Text(e)
			slog.Debug("announcement fetch failed", "site", s.Name, "err", e)
		}
	}
	if e == nil {
		c.annMu.Lock()
		c.annCache[s.ID] = annCacheEntry{ann: ann, at: time.Now()}
		c.annMu.Unlock()
	}
	return annSFResult{ann: ann, errMsg: errMsg}
}

// refreshAnnouncementsAsync SWR 后台刷新：公告缓存过期时立即返回旧值后，异步拉最新写缓存。
// 独立 ctx（不绑请求）+ sfDo 合并同 siteID（防刷新风暴）+ recover 防 panic。容量 1 的 sem
// 仅占槽不阻塞（真实限流靠 sfDo 合并 + 站点有界），与消息路径 refreshMessagesAsync 等效。
func (c *SiteChain) refreshAnnouncementsAsync(s models.Site) {
	go func() {
		defer func() { _ = recover() }()
		_, _ = sfDo(&c.annSF, strconv.Itoa(int(s.ID)), func() (annSFResult, error) {
			c.annMu.Lock()
			ce, ok := c.annCache[s.ID]
			c.annMu.Unlock()
			if ok && ce.ann != nil && time.Since(ce.at) <= announcementCacheTTL { // double-check
				return annSFResult{ann: ce.ann}, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), infoFetchTimeout)
			defer cancel()
			return c.fetchAnnouncementOnce(ctx, make(chan struct{}, 1), &s), nil
		})
	}()
}

func annResult(id uint, name string, ann *site.SiteAnnouncement, errMsg string) AnnouncementResult {
	items := ann.Items
	if items == nil {
		items = []site.AnnouncementItem{} // nil → [] 防 JSON null 致前端 .length 崩
	}
	r := AnnouncementResult{
		SiteID: id, SiteName: name,
		RawText:   ann.RawText,
		Items:     items,
		FetchedAt: ann.FetchedAt,
		Error:     errMsg,
	}
	return r
}

// annRow 组装单站公告行（聚合两条出口 + 单站同步共用，防口径漂移）：在 annResult 之上
// 补「这一站没接线」判定，并把回源失败留下的 nil ann 合成空行。
func (c *SiteChain) annRow(s *models.Site, ann *site.SiteAnnouncement, errMsg string) AnnouncementResult {
	if ann == nil {
		ann = &site.SiteAnnouncement{SiteID: s.ID, SiteName: s.Name, FetchedAt: time.Now()}
	}
	r := annResult(s.ID, s.Name, ann, errMsg)
	r.Unsupported = !c.AnnouncementWired(s)
	return r
}

// SyncAnnouncement 单站强制刷新公告（/announcements/sync/:id）：与聚合面共用 annRow 口径，
// 并回写缓存——handler 自行拼结果时，「没接线」判定与中文原因两处都会掉。
func (c *SiteChain) SyncAnnouncement(ctx context.Context, s *models.Site) AnnouncementResult {
	res := c.fetchAnnouncementOnce(ctx, make(chan struct{}, 1), s)
	return c.annRow(s, res.ann, res.errMsg)
}
