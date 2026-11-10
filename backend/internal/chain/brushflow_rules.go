// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/site"
)

func bfMatchRule(task *models.BrushFlowTask, tr *downloader.Torrent, now time.Time) string {
	if expr := strings.TrimSpace(task.DeleteRuleExpr); expr != "" {
		ok, reason, err := bfEvalExpr(expr, tr, now)
		if err != nil {
			slog.Warn("brushflow delete expr error", "expr", expr, "err", err)
			return ""
		}
		if ok {
			return reason
		}
		return ""
	}

	type cond struct {
		active bool
		hit    bool
		reason string
	}
	var cs []cond

	// ratio
	if task.DeleteRatio > 0 {
		hit := tr.Ratio >= task.DeleteRatio
		cs = append(cs, cond{true, hit, fmt.Sprintf("分享率 %.2f≥%.2f", tr.Ratio, task.DeleteRatio)})
	}
	// seed hours（加入至今）
	if task.DeleteSeedHours > 0 {
		hit := false
		reason := ""
		if !tr.AddedAt.IsZero() {
			h := now.Sub(tr.AddedAt).Hours()
			hit = h >= task.DeleteSeedHours
			reason = fmt.Sprintf("做种 %.1fh≥%.1fh", h, task.DeleteSeedHours)
		}
		cs = append(cs, cond{true, hit, reason})
	}
	// upload volume
	if task.DeleteUploadMB > 0 {
		hit := tr.Uploaded >= task.DeleteUploadMB<<20
		cs = append(cs, cond{true, hit, fmt.Sprintf("上传 %s≥%dMB", bfFormatBytes(tr.Uploaded), task.DeleteUploadMB)})
	}
	// min upload speed（仅对已完成/做种中的种子；下载中不因低速误杀）
	if task.DeleteMinUPRateKB > 0 {
		hit := false
		reason := ""
		// 已完成做种 + 满足最小做种时长兜底（DeleteMinSeedMinutes，防刚完成瞬时低速误杀）才判速度
		if bfIsSeedingDone(tr) && bfMeetsMinSeedTime(task, tr, now) {
			thr := task.DeleteMinUPRateKB << 10
			hit = tr.UPRate < thr
			reason = fmt.Sprintf("上传速度 %dB/s<%dKB/s", tr.UPRate, task.DeleteMinUPRateKB)
		}
		cs = append(cs, cond{true, hit, reason})
	}
	// inactive
	if task.DeleteInactiveHours > 0 {
		hit := false
		reason := ""
		if last := bfLastActive(*tr); !last.IsZero() {
			h := now.Sub(last).Hours()
			hit = h >= task.DeleteInactiveHours
			reason = fmt.Sprintf("不活跃 %.1fh≥%.1fh", h, task.DeleteInactiveHours)
		}
		cs = append(cs, cond{true, hit, reason})
	}
	// swarm 做种过多（NumComplete；下载中/swarm 数据缺失不删，对称于 DeleteMinSwarmLeechers）
	if task.DeleteMaxSwarmSeeders > 0 {
		hit := false
		reason := ""
		if bfIsSeedingDone(tr) && tr.NumComplete > 0 {
			hit = tr.NumComplete > task.DeleteMaxSwarmSeeders
			reason = fmt.Sprintf("swarm做种 %d>%d", tr.NumComplete, task.DeleteMaxSwarmSeeders)
		}
		cs = append(cs, cond{true, hit, reason})
	}
	// swarm 下载过少（已完成做种时才判；须 NumComplete>0 表示 swarm 数据可用，避免 TR/未填字段全 0 误杀）
	if task.DeleteMinSwarmLeechers > 0 {
		hit := false
		reason := ""
		if bfIsSeedingDone(tr) && bfMeetsMinSeedTime(task, tr, now) && tr.NumComplete > 0 {
			hit = tr.NumIncomplete < task.DeleteMinSwarmLeechers
			reason = fmt.Sprintf("swarm下载 %d<%d", tr.NumIncomplete, task.DeleteMinSwarmLeechers)
		}
		cs = append(cs, cond{true, hit, reason})
	}

	if len(cs) == 0 {
		return ""
	}
	logic := strings.ToLower(strings.TrimSpace(task.DeleteLogic))
	if logic == "and" {
		var reasons []string
		for _, c := range cs {
			if !c.hit {
				return ""
			}
			if c.reason != "" {
				reasons = append(reasons, c.reason)
			}
		}
		return strings.Join(reasons, " & ")
	}
	// default: or
	for _, c := range cs {
		if c.hit && c.reason != "" {
			return c.reason
		}
	}
	return ""
}

// bfIsSeedingDone 是否视为「已完成下载、进入做种阶段」（低上传速度规则仅在此时生效）。
func bfIsSeedingDone(tr *downloader.Torrent) bool {
	if tr.Progress >= 0.999 {
		return true
	}
	if !tr.CompletedAt.IsZero() {
		return true
	}
	switch strings.ToLower(tr.State) {
	case "uploading", "stalledup", "forcedup", "queuedup", "pausedup", "stoppedup", "seeding", "seed":
		return true
	}
	return false
}

// bfMeetsMinSeedTime 最小做种时长兜底（DeleteMinSeedMinutes，防刚完成做种瞬时速度/swarm 为 0 即被误删）。
// 返回 true=允许瞬时规则（低速/swarm）生效，false=跳过（不命中删除）。
//   - DeleteMinSeedMinutes<=0：不启用此守卫，直接放行（true）。
//   - CompletedAt 零值（未完成/未知完成时间）：视为不满足（false），与 bfIsSeedingDone 协同防御。
//   - 否则要求 now-CompletedAt ≥ DeleteMinSeedMinutes 分钟。
func bfMeetsMinSeedTime(task *models.BrushFlowTask, tr *downloader.Torrent, now time.Time) bool {
	if task.DeleteMinSeedMinutes <= 0 {
		return true
	}
	if tr.CompletedAt.IsZero() {
		return false
	}
	return now.Sub(tr.CompletedAt) >= time.Duration(task.DeleteMinSeedMinutes)*time.Minute
}

// bfLastActive 最后活跃时间：优先 LastActivity → CompletedAt → AddedAt。
func bfLastActive(tr downloader.Torrent) time.Time {
	if !tr.LastActivity.IsZero() {
		return tr.LastActivity
	}
	if !tr.CompletedAt.IsZero() {
		return tr.CompletedAt
	}
	return tr.AddedAt
}

// bfEvictionLess 体积淘汰排序比较：上传速度慢优先 → 最久未活跃优先。
// 数据缺失守卫：bfLastActive 零值（所有时间字段都未知）的种子排最后，不优先淘汰
// （避免信息不全的种子被 0001 年时间戳误判为"最久未活跃"而优先删）。
func bfEvictionLess(i, j downloader.Torrent) bool {
	if i.UPRate != j.UPRate {
		return i.UPRate < j.UPRate
	}
	li, lj := bfLastActive(i), bfLastActive(j)
	liZero, ljZero := li.IsZero(), lj.IsZero()
	if liZero || ljZero {
		// 任一零值时：零值排后（i 零值返 false，j 零值返 true；都零值保持稳定）
		return ljZero && !liZero
	}
	return li.Before(lj)
}

// bfEvictBySize 体积淘汰：总量超 limit 时按 bfEvictionLess 顺序挑出要删的种子，
// 返回互不相交的 (evict, remain)。remain 必须不含已淘汰项——调用方拿它去设原生限流，
// 混进刚删掉的哈希会让 TR/Deluge 的 torrent-set 整批报错，真留下的种子反而设不上限。
// 未超限返回 (nil, 原样 kept)。不修改传入切片的顺序之外的内容。
func bfEvictBySize(kept []downloader.Torrent, limit int64) (evict, remain []downloader.Torrent) {
	var total int64
	for _, tr := range kept {
		total += tr.Size
	}
	if total <= limit {
		return nil, kept
	}
	sorted := append([]downloader.Torrent(nil), kept...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return bfEvictionLess(sorted[i], sorted[j])
	})
	evictHashes := make(map[string]bool)
	for _, tr := range sorted {
		if total <= limit {
			break
		}
		evict = append(evict, tr)
		evictHashes[tr.Hash] = true
		total -= tr.Size
	}
	remain = make([]downloader.Torrent, 0, len(kept)-len(evict))
	for _, tr := range kept {
		if !evictHashes[tr.Hash] {
			remain = append(remain, tr)
		}
	}
	return evict, remain
}

// bfShareLimits 把任务的删除阈值（DeleteRatio/DeleteSeedHours）映射为下载器原生 ShareLimits。
// 未配维度（值 0）→ -1（全局默认，不覆盖下载器设置）。
// DeleteUploadMB 无原生对应（qb share limit 仅 ratio/time），仅应用层删种。
//
// 注意：跨下载器 SeedingTimeLimit 语义不一致（已知差异，不改语义，仅注释说明）：
//   - qBittorrent：总做种时长（达到后暂停，活跃也算）
//   - Transmission：映射为 seedIdleLimit 空闲时长（无 peer 上传一段时间后暂停）
//
// 因此 DeleteSeedHours 在 TR 上效果偏宽松（高活跃种子不会被暂停）；前端/任务编辑 UI 需注明此差异。
func bfShareLimits(task *models.BrushFlowTask) downloader.ShareLimits {
	// 未配维度 = -1（全局默认，不覆盖下载器设置），区别于 -2（无限）。
	lim := downloader.ShareLimits{RatioLimit: -1, SeedingTimeLimit: -1}
	if task.DeleteRatio > 0 {
		lim.RatioLimit = task.DeleteRatio
	}
	if task.DeleteSeedHours > 0 {
		lim.SeedingTimeLimit = int(task.DeleteSeedHours * 60) // 小时 → 分钟
	}
	return lim
}

// filterAndSort 选种过滤排序：任务配了 FilterGroups 则用规则引擎过滤（剔除不匹配的种子），
// 再按 seeders 升序排（刷流本质：少做种优先快涨分享率；引擎 pri 优先级排序不适用刷流）；
// 未配 FilterGroups 则回退 bfFilterAndSort（体积/做种/Free 硬过滤）。
func (c *BrushFlowChain) filterAndSort(ctx context.Context, results []site.Torrent, task *models.BrushFlowTask) []site.Torrent {
	groups := parsePipeList(task.FilterGroups)
	if c.filterEng != nil && len(groups) > 0 {
		media := &filter.MediaInfo{Category: task.Category}
		ranked, err := c.filterEng.Filter(ctx, groups, results, media)
		if err == nil {
			out := make([]site.Torrent, 0, len(ranked))
			for _, mt := range ranked {
				out = append(out, mt.Torrent)
			}
			sort.SliceStable(out, func(i, j int) bool { return out[i].Seeders < out[j].Seeders })
			return out
		}
	}
	return bfFilterAndSort(results, task)
}

// bfFilterAndSort 按选种规则过滤并按 seeders 升序排（seeders 少的优先快速涨分享率）。
func bfFilterAndSort(results []site.Torrent, task *models.BrushFlowTask) []site.Torrent {
	out := make([]site.Torrent, 0, len(results))
	for _, t := range results {
		if t.DownloadURL == "" {
			continue
		}
		if task.FreeOnly && !t.Free {
			continue
		}
		if task.MaxSeeders > 0 && t.Seeders > task.MaxSeeders {
			continue
		}
		if t.Seeders < task.MinSeeders {
			continue
		}
		sizeMB := bfParseSize(t.Size) >> 20
		if task.MinSizeMB > 0 && sizeMB < task.MinSizeMB {
			continue
		}
		if task.MaxSizeMB > 0 && sizeMB > task.MaxSizeMB {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seeders < out[j].Seeders })
	return out
}

func bfHasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func bfSeedKey(t site.Torrent) string {
	if t.DownloadURL != "" {
		return t.DownloadURL
	}
	return t.ID
}

// bfParseSize 解析 "1.2 GB"/"500MB"/"1024"/"1.5T" 等为字节；无法解析返回 0。
func bfParseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// 去掉中英文逗号
	s = strings.NewReplacer(",", "", "，", "").Replace(s)
	var num float64
	var unit string
	if _, err := fmt.Sscanf(s, "%f%s", &num, &unit); err != nil || num == 0 {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
		return 0
	}
	var mul int64
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "KB", "KIB", "K":
		mul = 1 << 10
	case "MB", "MIB", "M":
		mul = 1 << 20
	case "GB", "GIB", "G":
		mul = 1 << 30
	case "TB", "TIB", "T":
		mul = 1 << 40
	default:
		mul = 1
	}
	return int64(num * float64(mul))
}

func bfFormatBytes(n int64) string {
	switch {
	case n >= 1<<40:
		return fmt.Sprintf("%.2f TB", float64(n)/float64(1<<40))
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ---- 布尔表达式（阶梯策略）----
// 语法：ratio / seed_hours / upload_mb / up_rate_kb / inactive_hours 与数字比较，支持 && || () >= <= > < ==
// 例：(ratio>=1 && seed_hours>=24) || ratio>=3 || upload_mb>=5000
