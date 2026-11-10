// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/mediaserver"
)

// PlaybackReport 观影报告：聚合某服务器近 N 天的播放数据（用户列表 + 最近播放 + 热门条目）。
type PlaybackReport struct {
	Server      string                 `json:"server"`
	GeneratedAt time.Time              `json:"generated_at"`
	Days        int                    `json:"days"`
	Users       []mediaserver.User     `json:"users"`
	RecentPlays []mediaserver.PlayItem `json:"recent_plays"`
	TopItems    []mediaserver.PlayItem `json:"top_items"`
	TotalPlays  int                    `json:"total_plays"`
}

// reportRecentLimit 单用户最近播放抓取条数（观影报告默认覆盖面；过大拖慢）。
const reportRecentLimit = 100

// PlaybackReport 生成观影报告：枚举用户 → 各取最近 reportRecentLimit 条已播放 → 汇总最近/热门。
//
// days<=0 取 30 天。last-played 早于窗口或解析失败的条目被过滤。单用户失败不阻断其余
// （best-effort，与 OnDownloadCompleted 同范式）。Plex 历史需 Plex Pass， PlayedItems 仅返回
// 活跃会话近似（详见 plexClient.PlayedItems 注释，标 TODO 联调）。
func (c *MediaServerChain) PlaybackReport(ctx context.Context, id uint, days int) (*PlaybackReport, error) {
	if days <= 0 {
		days = 30
	}
	cli, m, err := c.client(id)
	if err != nil {
		return nil, err
	}
	users, err := cli.Users(ctx)
	if err != nil {
		return nil, err
	}
	if users == nil {
		users = []mediaserver.User{}
	}
	cutoff := time.Now().AddDate(0, 0, -days)

	var allPlays []mediaserver.PlayItem
	for i := range users {
		u := users[i]
		items, err := cli.PlayedItems(ctx, u.ID, reportRecentLimit)
		if err != nil {
			// 单用户失败（如受限账户）不阻断整体报告。
			continue
		}
		for _, it := range items {
			it.UserName = u.Name
			if it.UserID == "" {
				it.UserID = u.ID
			}
			u.PlayCount += it.PlayCount
			if t, terr := time.Parse(time.RFC3339, it.LastPlayed); terr == nil && t.Before(cutoff) {
				continue // 窗口外
			}
			allPlays = append(allPlays, it)
		}
		users[i] = u
	}

	// 最近播放：按 LastPlayed 降序（解析失败排末尾）。
	sort.SliceStable(allPlays, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, allPlays[i].LastPlayed)
		tj, _ := time.Parse(time.RFC3339, allPlays[j].LastPlayed)
		return ti.After(tj)
	})
	recent := allPlays
	if len(recent) > 50 {
		recent = recent[:50]
	}

	// 热门条目：按 ItemID 聚合 PlayCount，取前 10。
	top := topPlayed(allPlays, 10)

	total := 0
	for _, p := range allPlays {
		total += p.PlayCount
	}
	return &PlaybackReport{
		Server: m.Name, GeneratedAt: time.Now(), Days: days,
		Users: users, RecentPlays: recent, TopItems: top, TotalPlays: total,
	}, nil
}

// topPlayed 按 ItemID 聚合播放次数，返回 Top N（同名条目聚合为一条，PlayCount 取聚合值）。
func topPlayed(plays []mediaserver.PlayItem, limit int) []mediaserver.PlayItem {
	if limit <= 0 {
		limit = 10
	}
	type agg struct {
		item  mediaserver.PlayItem
		count int
	}
	bucket := map[string]*agg{}
	order := []string{}
	for _, p := range plays {
		key := p.ItemID
		if key == "" {
			key = strings.ToLower(p.Name)
		}
		if a, ok := bucket[key]; ok {
			a.count += p.PlayCount
			continue
		}
		bucket[key] = &agg{item: p, count: p.PlayCount}
		order = append(order, key)
	}
	out := make([]mediaserver.PlayItem, 0, len(order))
	for _, k := range order {
		it := bucket[k].item
		it.PlayCount = bucket[k].count
		out = append(out, it)
	}
	// 按聚合播放次数降序（稳定排序保序）。
	sort.SliceStable(out, func(i, j int) bool {
		keyOf := func(p mediaserver.PlayItem) string {
			if p.ItemID != "" {
				return p.ItemID
			}
			return strings.ToLower(p.Name)
		}
		return bucket[keyOf(out[i])].count > bucket[keyOf(out[j])].count
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// DetectDuplicates 重复媒体检测：枚举某库条目 → 按 TMDB/IMDb ID（优先）或归一化标题+年份归并 →
// 返回 Count>=2 的组。空库返回空切片（非 nil）。
func (c *MediaServerChain) DetectDuplicates(ctx context.Context, id uint, libraryID string) ([]mediaserver.DuplicateGroup, error) {
	cli, _, err := c.client(id)
	if err != nil {
		return nil, err
	}
	items, err := cli.LibraryItems(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	groups := mediaserver.DetectDuplicates(items)
	if groups == nil {
		groups = []mediaserver.DuplicateGroup{}
	}
	return groups, nil
}

// DirectLink 302 直链改写：读取服务器配置，若开启直链且 DirectLinkBase 非空，
// 返回改写后的目标 URL（{base}/{path}）与服务器名（日志用）。未开启返回 errDirectLinkDisabled。
//
// 仅做静态改写（不泄露服务器 Token），让播放器直连高带宽 CDN。path 为 Emby 文件相对路径。
// path 先经 path.Clean 归一（.. 不能越过根），防公开端点上构造 ../.. 被 CDN 归一后
// 逃出 DirectLinkBase 的目录前缀、访问同主机其它路径。
func (c *MediaServerChain) DirectLink(_ context.Context, id uint, relPath string) (target, serverName string, err error) {
	m, err := c.op.Get(id)
	if err != nil {
		return "", "", err
	}
	if !m.DirectLinkEnabled || m.DirectLinkBase == "" {
		return "", "", errDirectLinkDisabled
	}
	base := strings.TrimRight(m.DirectLinkBase, "/")
	target = base + path.Clean("/"+strings.TrimLeft(relPath, "/"))
	if !directLinkSafe(target) {
		return "", m.Name, errDirectLinkBadBase
	}
	return target, m.Name, nil
}

// errDirectLinkDisabled 直链未开启（handler 返 404，非错误日志）。
var errDirectLinkDisabled = fmt.Errorf("direct link not enabled")

// errDirectLinkBadBase DirectLinkBase 配置非法（非 http(s) URL），改写出的目标不可用。
var errDirectLinkBadBase = fmt.Errorf("direct link base is not a valid http(s) URL")

// directLinkSafe 校验改写后的 URL 合法（scheme/host 存在），防 base 配错产生相对 URL。
func directLinkSafe(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
