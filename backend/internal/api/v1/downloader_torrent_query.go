// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// 下载器种子列表的查询侧辅助（从 downloader.go 拆出以满足单文件 400 行门禁）：
// query 钳位、state 归并分组、内存过滤与排序。逻辑零变化，纯搬运。

import (
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// clampQueryInt 解析 query 整数，越界钳到 [lo,hi]。
func clampQueryInt(c *gin.Context, key string, def, lo, hi int) int {
	v := atoiDefault(c.Query(key), def)
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// torrentStateGroup 把各下载器原始 state 归并为前端分组（对齐前端 torrentTableFilters.stateGroup）。
// qB 的做种族 uploading/stalledUP/queuedUP/forcedUP 以 UP 后缀识别（stalledUP 里无 upload/seed 字面量，
// 旧实现归到 other → 下载器页做种筛选漏掉绝大多数做种种子）；DL 后缀同理归下载族。
// checking（校验中）/moving（移动中）先排除再匹配后缀；仅字面 pause|stop 判暂停。
func torrentStateGroup(s string) string {
	low := strings.ToLower(s)
	switch {
	case strings.Contains(low, "error") || strings.Contains(low, "missing"):
		return "error"
	case strings.Contains(low, "pause") || strings.Contains(low, "stop"):
		return "paused"
	case strings.Contains(low, "check") || strings.Contains(low, "moving"):
		return "other"
	case strings.HasSuffix(low, "up") || strings.Contains(low, "upload") || strings.Contains(low, "seed"):
		return "seeding"
	case strings.HasSuffix(low, "dl") || strings.Contains(low, "download") || strings.Contains(low, "leech") || strings.Contains(low, "meta"):
		return "downloading"
	default:
		return "other"
	}
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// filterTorrents 按 category/state/tag/q 内存过滤（种子量级千以内可接受）。
func filterTorrents(ts []downloader.Torrent, c *gin.Context) []downloader.Torrent {
	cat := c.Query("category")
	st := c.Query("state")
	tag := c.Query("tag")
	q := strings.ToLower(c.Query("q"))
	if cat == "" && (st == "" || st == "all") && tag == "" && q == "" {
		return ts
	}
	out := make([]downloader.Torrent, 0, len(ts))
	for _, t := range ts {
		if cat != "" && t.Category != cat {
			continue
		}
		if st != "" && st != "all" && torrentStateGroup(t.State) != st {
			continue
		}
		if tag != "" && !containsString(t.Tags, tag) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.Name), q) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func cmpInt64(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func cmpFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// sortTorrents 按 sort/order 原地排序（默认 added_at desc）；用 cmp int 避免违反严格弱序。
func sortTorrents(ts []downloader.Torrent, c *gin.Context) {
	key := c.DefaultQuery("sort", "added_at")
	desc := c.DefaultQuery("order", "desc") != "asc"
	cmp := func(a, b downloader.Torrent) int {
		switch key {
		case "name":
			return strings.Compare(a.Name, b.Name)
		case "size":
			return cmpInt64(a.Size, b.Size)
		case "progress":
			return cmpFloat(a.Progress, b.Progress)
		case "ratio":
			return cmpFloat(a.Ratio, b.Ratio)
		case "dl_rate":
			return cmpInt64(a.DLRate, b.DLRate)
		case "up_rate":
			return cmpInt64(a.UPRate, b.UPRate)
		default:
			return cmpInt64(a.AddedAt.UnixNano(), b.AddedAt.UnixNano())
		}
	}
	sort.Slice(ts, func(i, j int) bool {
		r := cmp(ts[i], ts[j])
		if desc {
			return r > 0
		}
		return r < 0
	})
}
