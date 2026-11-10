// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/mediaserver"
)

// ---- 媒体健康中心 ----
//
// 对标 MSaber 媒体健康中心：库级巡检发现三类问题——
//   未匹配：条目无 TMDB/IMDb Provider ID（刮削失败）
//   缺集  ：剧集有 ParentId 但未挂任何 Episode（或集数 < 最小集数启发值 3）
//   缺字幕：媒体文件同目录无 .srt/.ass/.ssa
// 结果纯计算不落库（每次现算，库规模千级内秒级完成）。

// HealthIssue 单条健康问题。
type HealthIssue struct {
	ServerID uint   `json:"server_id"`
	Library  string `json:"library"`
	ItemID   string `json:"item_id"`
	Title    string `json:"title"`
	Type     string `json:"type"` // unmatched | missing_episodes | missing_subtitle
	Path     string `json:"path,omitempty"`
	Detail   string `json:"detail"`
}

// HealthReport 巡检报告。
type HealthReport struct {
	Issues      []HealthIssue `json:"issues"`
	Unmatched   int           `json:"unmatched"`
	MissingEps  int           `json:"missing_episodes"`
	MissingSub  int           `json:"missing_subtitle"`
	Upgradeable int           `json:"upgradeable"`
	CheckedAt   time.Time     `json:"checked_at"`
	// Errors 本轮未能覆盖到的部分（服务器连不上、库读不出来……），每条带服务器名/库名。
	// 必须显式回传：否则「一条问题都没有」既可能是库很干净，也可能是根本没扫成，
	// 前端会统一渲染成绿色健康，用户据此不再巡检。
	Errors []string `json:"errors,omitempty"`
}

// HealthCheck 对所有服务器全库巡检（best-effort：单服务器失败跳过，但失败原因记入
// report.Errors 供前端提示）。Emby/JF/Plex 均支持；Plex 缺集走 leafCount（EpisodeCount 已适配）。
// subtitleScan=true 时对条目路径做本地字幕扫描（后端需能访问媒体路径/挂载）。
// upgradeable=true 时对 TMDB 条目做版本升级检测（库内存在同 TMDB ID 的更高分辨率条目）。
func (c *MediaServerChain) HealthCheck(ctx context.Context, subtitleScan, upgradeable bool) *HealthReport {
	rep := &HealthReport{Issues: []HealthIssue{}, CheckedAt: time.Now()}
	servers, err := c.op.List()
	if err != nil {
		rep.Errors = append(rep.Errors, "媒体服务器列表读取失败："+err.Error())
		return rep
	}
	for _, sv := range servers {
		c.healthCheckServer(ctx, sv.ID, sv.Name, subtitleScan, upgradeable, rep)
	}
	rep.Unmatched = countIssues(rep.Issues, "unmatched")
	rep.MissingEps = countIssues(rep.Issues, "missing_episodes")
	rep.MissingSub = countIssues(rep.Issues, "missing_subtitle")
	rep.Upgradeable = countIssues(rep.Issues, "upgradeable")
	return rep
}

// healthCheckServer 单服务器巡检：series 用 LibraryItems + Episodes 子查询。
// 各失败出口都把原因写进 rep.Errors（跳过该服务器/该库，但不再静默）。
func (c *MediaServerChain) healthCheckServer(ctx context.Context, serverID uint, serverName string, subtitleScan, upgradeable bool, rep *HealthReport) {
	cli, _, err := c.client(serverID)
	if err != nil {
		slog.Warn("health: client build failed", "server", serverID, "err", err)
		rep.Errors = append(rep.Errors, fmt.Sprintf("%s：连接配置不可用（%v）", serverName, err))
		return
	}
	libs, err := cli.Libraries(ctx)
	if err != nil {
		slog.Warn("health: libraries failed", "server", serverID, "err", err)
		rep.Errors = append(rep.Errors, fmt.Sprintf("%s：库列表读取失败（%v）", serverName, err))
		return
	}
	ec, ok := cli.(episodeCounter)
	if !ok {
		rep.Errors = append(rep.Errors, fmt.Sprintf("%s：该服务器类型暂不支持健康巡检", serverName))
		return
	}
	for _, lib := range libs {
		if lib.Type != "tvshows" && lib.Type != "movies" && lib.Type != "mixed" {
			continue
		}
		items, err := cli.LibraryItems(ctx, lib.ID)
		if err != nil {
			slog.Debug("health: library items failed", "lib", lib.Name, "err", err)
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s：库「%s」条目读取失败（%v）", serverName, lib.Name, err))
			continue
		}
		if upgradeable {
			detectUpgradable(serverID, lib.Name, items, rep)
		}
		for _, it := range items {
			appendItemIssues(ctx, serverID, lib.Type, lib.Name, it, ec, subtitleScan, rep)
		}
	}
	sort.Slice(rep.Issues, func(i, j int) bool { return rep.Issues[i].Type < rep.Issues[j].Type })
}

// appendItemIssues 单条目的三类问题判定（拆出便于单测）。
func appendItemIssues(ctx context.Context, serverID uint, libType, libName string, it mediaserver.DetailedItem, ec episodeCounter, subtitleScan bool, rep *HealthReport) {
	// 未匹配：无 Provider ID（电影与剧集同判）
	if it.TMDBID == "" && it.IMDbID == "" {
		rep.Issues = append(rep.Issues, HealthIssue{
			ServerID: serverID, Library: libName, ItemID: it.ItemID,
			Title: it.Name, Type: "unmatched", Detail: "无 TMDB/IMDb 标识（刮削失败或未刮削）",
		})
	}
	// 缺集：剧集类目下集数过少（>0 且 <3 为启发阈值；0 完全无集）
	if libType == "tvshows" || libType == "mixed" {
		if it.Type == "series" {
			if n, err := ec.EpisodeCount(ctx, it.ItemID); err == nil && n < 3 {
				rep.Issues = append(rep.Issues, HealthIssue{
					ServerID: serverID, Library: libName, ItemID: it.ItemID,
					Title: it.Name, Type: "missing_episodes", Path: it.Path,
					Detail: fmt.Sprintf("仅 %d 集（疑似缺集）", n),
				})
			}
		}
	}
	// 缺字幕：本地同目录无字幕文件（需后端可访问路径）
	if subtitleScan && subtitleCheckablePath(it.Path) && !hasSubtitleNextTo(it.Path) {
		rep.Issues = append(rep.Issues, HealthIssue{
			ServerID: serverID, Library: libName, ItemID: it.ItemID,
			Title: it.Name, Type: "missing_subtitle", Path: it.Path,
			Detail: "同目录无 .srt/.ass 字幕",
		})
	}
}

// subtitleCheckablePath 仅当 Path 指向媒体文件本身时才可做同目录字幕判定。
// 剧集（Series）条目的 Path 是剧集目录：对其判定会读到父目录（库根）造成全库误报，
// 「一键补齐」还会把字幕写到库根（幻影文件），并反过来遮蔽后续全部检测（自遮蔽）。
func subtitleCheckablePath(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mkv", ".mp4", ".ts", ".m2ts", ".avi", ".mov", ".wmv", ".flv", ".m4v", ".iso", ".rmvb", ".rm", ".webm":
		return true
	}
	return false
}

// episodeCounter 剧集集数查询（embyJellyfinClient / plexClient 实现）。
type episodeCounter interface {
	EpisodeCount(ctx context.Context, seriesID string) (int, error)
}

// qualityRank 从文件名/路径提取分辨率等级（越高越好；识别不出=0，不参与升级判定）。
// 来源：DetailedItem.Path（库内条目路径，Plex LibraryItems 暂不带 Path 则跳过该项）。
func qualityRank(p string) int {
	s := strings.ToUpper(p)
	switch {
	case strings.Contains(s, "2160") || strings.Contains(s, "4K") || strings.Contains(s, "UHD"):
		return 4
	case strings.Contains(s, "1080"):
		return 3
	case strings.Contains(s, "720"):
		return 2
	case strings.Contains(s, "480") || strings.Contains(s, "SD"):
		return 1
	}
	return 0
}

// detectUpgradable 可升级检测：同 TMDB ID 的多个条目里，保留最高版本，
// 其余（同分低版本视为重复占位）中比最高版本低且低版本自身可识别的记为可升级。
// 洗版场景：库内同时存在 720p 与 2160p 同一部片 → 720p 条目记 upgradeable。
func detectUpgradable(serverID uint, library string, items []mediaserver.DetailedItem, rep *HealthReport) {
	type entry struct {
		it   mediaserver.DetailedItem
		rank int
	}
	byTMDB := map[string][]entry{}
	for _, it := range items {
		if it.TMDBID == "" || it.Path == "" {
			continue
		}
		r := qualityRank(it.Path)
		if r == 0 {
			continue // 无法判定版本的不参与
		}
		byTMDB[it.TMDBID] = append(byTMDB[it.TMDBID], entry{it: it, rank: r})
	}
	for _, group := range byTMDB {
		if len(group) < 2 {
			continue
		}
		best := 0
		for _, e := range group {
			if e.rank > best {
				best = e.rank
			}
		}
		for _, e := range group {
			if e.rank < best {
				rep.Issues = append(rep.Issues, HealthIssue{
					ServerID: serverID, Library: library, ItemID: e.it.ItemID,
					Title: e.it.Name, Type: "upgradeable", Path: e.it.Path,
					Detail: fmt.Sprintf("库内存在更高版本（当前 %d 级 vs 最高 %d 级），可洗版清理", e.rank, best),
				})
			}
		}
	}
}

// hasSubtitleNextTo 媒体文件同目录是否存在字幕（含 .strm 场景：跳过，云端字幕不做本地判定）。
func hasSubtitleNextTo(mediaPath string) bool {
	if strings.HasSuffix(strings.ToLower(mediaPath), ".strm") {
		return true // STRM 云端媒体：字幕在云盘/转码侧，不做本地判定
	}
	dir := filepath.Dir(mediaPath)
	entries, err := readDirNames(dir)
	if err != nil {
		return true // 目录不可访问（未挂载）：不误报
	}
	for _, e := range entries {
		switch strings.ToLower(filepath.Ext(e)) {
		case ".srt", ".ass", ".ssa", ".sub":
			return true
		}
	}
	return false
}

func countIssues(issues []HealthIssue, typ string) int {
	n := 0
	for _, i := range issues {
		if i.Type == typ {
			n++
		}
	}
	return n
}

// readDirNames 列目录文件名（os.ReadDir 薄封装便于测试替换）。
var readDirNames = defaultReadDirNames

func defaultReadDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}
