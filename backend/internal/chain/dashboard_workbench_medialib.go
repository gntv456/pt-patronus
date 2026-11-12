// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// 工作台「媒体库态势」的聚合与 TTL 缓存（从 dashboard_workbench.go 拆出：该文件
// 触及 400 行门禁；本域自成单元——缓存壳 + 真实抓取 + 测试复位口）。

// mediaLibCache 是 MediaLibraryOverview 的 5 分钟 TTL 缓存。工作台 60s 轮询 ×
// 媒体库计数（原本每轮全量拉条目）是首屏卡顿的大头；库条目数本就是慢变量，
// 5 分钟内重复请求直接回缓存。mu + exp 保护；缓存未命中才走真实抓取。
var (
	mediaLibMu    sync.Mutex
	mediaLibCache WorkbenchMediaLibrary
	mediaLibExp   time.Time
)

// mediaLibTTL 媒体库概览缓存时长。测试可通过缩短它验证过期行为。
var mediaLibTTL = 5 * time.Minute

// MediaLibraryOverview 枚举全部媒体服务器库计数（DashboardChain.Workbench 的
// msLibraries 回调实现；单服务器失败跳过不阻断）。5 分钟 TTL 缓存 + 计数端点
// （LibraryItemCount）：原实现每轮整库拉条目只为 ++，上万条的 Emby/Plex 库把
// 工作台首屏拖到数秒，且 60s 轮询每次重复付这个代价。
func (c *MediaServerChain) MediaLibraryOverview(ctx context.Context) WorkbenchMediaLibrary {
	mediaLibMu.Lock()
	if time.Now().Before(mediaLibExp) {
		cached := mediaLibCache
		mediaLibMu.Unlock()
		return cached
	}
	mediaLibMu.Unlock()

	out := c.fetchMediaLibraryOverview(ctx)

	// 出错不落缓存：列表读失败是瞬时状态（媒体服务器重启/网络抖动），缓存 5 分钟
	// 会把「这次没读到」钉成「5 分钟内都读不到」；成功结果才值得缓存。
	if out.Error == "" {
		mediaLibMu.Lock()
		mediaLibCache = out
		mediaLibExp = time.Now().Add(mediaLibTTL)
		mediaLibMu.Unlock()
	}
	return out
}

// resetMediaLibCache 清空媒体库概览缓存（测试隔离用；生产路径不调用）。
func resetMediaLibCache() {
	mediaLibMu.Lock()
	mediaLibCache = WorkbenchMediaLibrary{}
	mediaLibExp = time.Time{}
	mediaLibMu.Unlock()
}

// fetchMediaLibraryOverview 真实抓取：列表读失败与未配置必须区分（见调用处注释）。
func (c *MediaServerChain) fetchMediaLibraryOverview(ctx context.Context) WorkbenchMediaLibrary {
	out := WorkbenchMediaLibrary{Configured: false, Servers: []MediaLibraryStat{}}
	servers, err := c.List()
	if err != nil {
		// 读不出来 ≠ 没配置：不区分的话首屏会对一个配了 Emby/Plex 的库显示「未配置媒体库，去添加」，
		// 用户顺着去新建一遍，反而造成重复配置。
		slog.Warn("workbench: list media servers failed", "err", err)
		out.Error = "媒体服务器列表读取失败: " + err.Error()
		return out
	}
	if len(servers) == 0 {
		return out
	}
	out.Configured = true
	for _, s := range servers {
		stat := MediaLibraryStat{ServerID: s.ID, ServerName: s.Name, Type: s.Type, SyncedAt: time.Now()}
		cli, _, err := c.client(s.ID)
		if err != nil {
			slog.Debug("workbench: media server client failed", "server", s.Name, "err", err)
			out.Servers = append(out.Servers, stat)
			continue
		}
		libs, err := cli.Libraries(ctx)
		if err != nil {
			// 一台媒体的库列表读不出，界面上它是「0 个库」而不是「这次没读到」——
			// Debug 级在默认日志级别下等于没有留痕，必须 Warn。
			slog.Warn("workbench: media server libraries failed", "server", s.Name, "err", err)
			out.Servers = append(out.Servers, stat)
			continue
		}
		stat.LibraryCnt = len(libs)
		for _, lib := range libs {
			cnt, err := cli.LibraryItemCount(ctx, lib)
			if err != nil {
				// 单个库读不出就裸 continue ⇒ 总数被静默少算一个库，
				// 与「那个库真的是空的」在界面上同形（工作台是看全局的地方）。
				slog.Warn("workbench: library count failed, library skipped",
					"server", s.Name, "library", lib.ID, "name", lib.Name, "err", err)
				continue
			}
			stat.Movies += cnt.Movies
			stat.Series += cnt.Series
		}
		out.Servers = append(out.Servers, stat)
	}
	return out
}
