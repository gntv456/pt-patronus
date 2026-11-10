// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestBFMatchRuleAddedAtZero 复现缺陷：下载器返回零值 AddedAt（未知加入时间）时，
// now.Sub(time.Time{}) ≈ 56 年 → 做种时长规则立即命中 → 误删刚加入/未知加入时间的种子。
//
// 正确行为：AddedAt 零值时不应据做种时长删种（信息缺失不应触发删除）。
// 预期：未修复时 FAIL（误删），修复后 PASS。
func TestBFMatchRuleAddedAtZero(t *testing.T) {
	task := &models.BrushFlowTask{DeleteSeedHours: 1}
	tr := &downloader.Torrent{AddedAt: time.Time{}, Ratio: 0, Uploaded: 0}
	if r := bfMatchRule(task, tr, time.Now()); r != "" {
		t.Errorf("AddedAt 零值时做种时长规则不应误删, got %q — 缺陷: 刚加入/未知时间的种子被误判为做种 56 年", r)
	}
}

// TestBFMatchRuleUploadBoundary 上传量删除规则精确边界（字节级）。
func TestBFMatchRuleUploadBoundary(t *testing.T) {
	now := time.Now()
	task := &models.BrushFlowTask{DeleteUploadMB: 100}
	// 差 1 字节不删
	tr := &downloader.Torrent{AddedAt: now, Uploaded: 100<<20 - 1}
	if r := bfMatchRule(task, tr, now); r != "" {
		t.Errorf("上传 100MB-1B 不应删, got %q", r)
	}
	// 恰好删
	tr2 := &downloader.Torrent{AddedAt: now, Uploaded: 100 << 20}
	if r := bfMatchRule(task, tr2, now); r == "" {
		t.Error("上传恰好 100MB 应删")
	}
}

// TestBFMatchRuleSeedHoursBoundary 做种时长删除规则精确边界。
func TestBFMatchRuleSeedHoursBoundary(t *testing.T) {
	now := time.Now()
	task := &models.BrushFlowTask{DeleteSeedHours: 1}
	// 59min 不删
	if r := bfMatchRule(task, &downloader.Torrent{AddedAt: now.Add(-59 * time.Minute)}, now); r != "" {
		t.Errorf("59min 不应删, got %q", r)
	}
	// 61min 删
	if r := bfMatchRule(task, &downloader.Torrent{AddedAt: now.Add(-61 * time.Minute)}, now); r == "" {
		t.Error("61min 应删")
	}
}

// TestBFMatchRuleRatioBoundary 分享率删除规则精确边界（>= 即删）。
func TestBFMatchRuleRatioBoundary(t *testing.T) {
	now := time.Now()
	task := &models.BrushFlowTask{DeleteRatio: 1.5}
	if r := bfMatchRule(task, &downloader.Torrent{AddedAt: now, Ratio: 1.4999}, now); r != "" {
		t.Errorf("ratio 1.4999 不应删, got %q", r)
	}
	if r := bfMatchRule(task, &downloader.Torrent{AddedAt: now, Ratio: 1.5}, now); r == "" {
		t.Error("ratio 恰好 1.5 应删")
	}
}

// TestBFParseSizeEdgeCases 体积解析边界（含纯单位/无数字/超大值）。
func TestBFParseSizeEdgeCases(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"2 TB", 2 << 40},
		{"3.5GB", 3758096384},
		{"768 MB", 768 << 20},
		{"1024", 1024},
		{"GB", 0},  // 纯单位无数字
		{"abc", 0}, // 非法
		{"", 0},
	}
	for _, tc := range cases {
		got := bfParseSize(tc.in)
		if tc.want == 0 {
			if got != 0 {
				t.Errorf("bfParseSize(%q)=%d want 0", tc.in, got)
			}
			continue
		}
		diff := got - tc.want
		if diff < 0 {
			diff = -diff
		}
		if diff > tc.want/50 {
			t.Errorf("bfParseSize(%q)=%d want≈%d", tc.in, got, tc.want)
		}
	}
}

// TestBFFilterAndSortSizeBounds 体积上下限过滤精确边界（MB 级）。
func TestBFFilterAndSortSizeBounds(t *testing.T) {
	task := &models.BrushFlowTask{MinSizeMB: 100, MaxSizeMB: 1000}
	in := []site.Torrent{
		{ID: "1", DownloadURL: "u1", Size: "99 MB", Seeders: 5},   // 低于下限
		{ID: "2", DownloadURL: "u2", Size: "100 MB", Seeders: 5},  // 恰好下限
		{ID: "3", DownloadURL: "u3", Size: "1000 MB", Seeders: 6}, // 恰好上限
		{ID: "4", DownloadURL: "u4", Size: "1001 MB", Seeders: 7}, // 超上限
	}
	out := bfFilterAndSort(in, task)
	if len(out) != 2 {
		t.Fatalf("want 2 (边界含等), got %d: %+v", len(out), out)
	}
}

// TestBFExprVarsDataMissing [P1] 数据缺失守卫：表达式变量在数据不可靠时置 NaN，
// bfExprParser 用 Go 原生 float 比较（NaN 与任何数比较均 false），恰好"不命中删除"。
func TestBFExprVarsDataMissing(t *testing.T) {
	now := time.Now()
	// 1) 下载中（!bfIsSeedingDone）的 up_rate 不可靠（瞬态低速）→ 表达式不命中
	dl := &downloader.Torrent{Progress: 0.5, State: "downloading", UPRate: 0, NumComplete: 5, AddedAt: now}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteRuleExpr: "up_rate_kb<10"}, dl, now); r != "" {
		t.Errorf("下载中 up_rate 规则不应命中, got %q", r)
	}
	// 2) 已做种但 swarm 缺失（NumComplete<=0）→ swarm_seeders/swarm_leechers 不命中
	seeding := &downloader.Torrent{Progress: 1, State: "uploading", NumComplete: 0, NumIncomplete: 0, AddedAt: now}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteRuleExpr: "swarm_seeders>10"}, seeding, now); r != "" {
		t.Errorf("swarm 缺失时 swarm_seeders 规则不应命中, got %q", r)
	}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteRuleExpr: "swarm_leechers<5"}, seeding, now); r != "" {
		t.Errorf("swarm 缺失时 swarm_leechers 规则不应命中, got %q", r)
	}
	// 3) 已做种 + swarm 已知 + 速度低 → 应命中（NaN 未介入）
	slow := &downloader.Torrent{
		Progress: 1, State: "uploading", UPRate: 0, NumComplete: 20, NumIncomplete: 0,
		AddedAt: now.Add(-time.Hour), CompletedAt: now.Add(-time.Hour),
	}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteRuleExpr: "up_rate_kb<10"}, slow, now); r == "" {
		t.Error("已做种低速应命中 up_rate 规则")
	}
}

// TestBFMatchRuleMinSeedTime [P1] 最小做种时长兜底：
// DeleteMinSeedMinutes>0 时，刚完成做种（now-CompletedAt 不足该分钟数）的种子，
// 即使瞬时速度极低/swarm 极差，也不应因 DeleteMinUPRateKB/DeleteMinSwarmLeechers 误删。
func TestBFMatchRuleMinSeedTime(t *testing.T) {
	now := time.Now()
	young := &downloader.Torrent{
		Progress: 1, State: "uploading",
		UPRate: 0, NumComplete: 100, NumIncomplete: 0,
		CompletedAt: now.Add(-1 * time.Minute), AddedAt: now.Add(-1 * time.Hour),
	}
	// 1) 未过最小做种时长 → 瞬时规则全部不命中
	task := &models.BrushFlowTask{DeleteMinSeedMinutes: 10, DeleteMinUPRateKB: 2, DeleteMinSwarmLeechers: 5}
	if r := bfMatchRule(task, young, now); r != "" {
		t.Errorf("最小做种时长未满足不应删, got %q", r)
	}
	// 2) 已过最小做种时长 → 瞬时低速/swarm 差应命中（DeleteLogic 默认 or）
	old := *young
	old.CompletedAt = now.Add(-11 * time.Minute)
	if r := bfMatchRule(task, &old, now); r == "" {
		t.Error("已过最小做种时长且瞬时条件命中应删")
	}
	// 3) DeleteMinSeedMinutes=0（未启用兜底）→ 刚完成做种也应按瞬时规则删
	if r := bfMatchRule(&models.BrushFlowTask{DeleteMinUPRateKB: 2}, young, now); r == "" {
		t.Error("未启用最小做种时长兜底时应按瞬时规则删")
	}
	// 4) CompletedAt 零值（未完成/未知）即使 State=uploading → 视为未满足最小做种时长
	unknown := downloader.Torrent{
		Progress: 1, State: "uploading", UPRate: 0, NumComplete: 100, NumIncomplete: 0, AddedAt: now,
	}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteMinSeedMinutes: 10, DeleteMinUPRateKB: 2}, &unknown, now); r != "" {
		t.Errorf("CompletedAt 零值时最小做种时长视为未满足不应删, got %q", r)
	}
}

// TestBFMatchRuleMaxSwarmSeedersDownloading [P2] 下载中种子即使 swarm 做种多
// 也不应因 DeleteMaxSwarmSeeders 删（与 DeleteMinSwarmLeechers 守卫对称）。
func TestBFMatchRuleMaxSwarmSeedersDownloading(t *testing.T) {
	now := time.Now()
	dl := &downloader.Torrent{
		Progress: 0.5, State: "downloading",
		NumComplete: 200, NumIncomplete: 10, AddedAt: now,
	}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteMaxSwarmSeeders: 50}, dl, now); r != "" {
		t.Errorf("下载中不应因 swarm 做种多被删, got %q", r)
	}
	// 已完成做种且 swarm 做种多 → 应删
	seeding := *dl
	seeding.Progress = 1
	seeding.State = "uploading"
	if r := bfMatchRule(&models.BrushFlowTask{DeleteMaxSwarmSeeders: 50}, &seeding, now); r == "" {
		t.Error("已做种且 swarm 做种多应删")
	}
}

// TestBFEvictionLessLastActiveZero [P2] 体积淘汰排序：
// bfLastActive 零值（数据缺失）的种子应排最后（不优先淘汰）。
func TestBFEvictionLessLastActiveZero(t *testing.T) {
	now := time.Now()
	// normal: 有 LastActivity；zero: 所有时间字段零值（bfLastActive 返回零值）
	normal := downloader.Torrent{Hash: "normal", UPRate: 100, LastActivity: now.Add(-time.Hour)}
	zero := downloader.Torrent{Hash: "zero", UPRate: 100} // LastActivity/CompletedAt/AddedAt 全零
	// 同速度下，零值应排后（即 bfEvictionLess(normal, zero)=true）
	if !bfEvictionLess(normal, zero) {
		t.Error("normal 应排在 zero 之前（零值排末尾），got bfEvictionLess(normal,zero)=false")
	}
	if bfEvictionLess(zero, normal) {
		t.Error("零值不应排在 normal 之前，got bfEvictionLess(zero,normal)=true")
	}
	// 同为零值时保持稳定（互不为先）
	zero2 := downloader.Torrent{Hash: "zero2", UPRate: 100}
	if bfEvictionLess(zero, zero2) || bfEvictionLess(zero2, zero) {
		t.Error("同为零值时应保持稳定，不应互为先")
	}
}

// TestBFLastActiveFallback 验证 bfLastActive 的字段优先级与零值行为（缺陷5 的基础）。
func TestBFLastActiveFallback(t *testing.T) {
	// 全零值 → 返回零值
	if last := bfLastActive(downloader.Torrent{}); !last.IsZero() {
		t.Errorf("全零值应返回零值, got %v", last)
	}
	addedAt := time.Now().Add(-3 * time.Hour)
	completedAt := time.Now().Add(-2 * time.Hour)
	lastAct := time.Now().Add(-30 * time.Minute)
	// 仅 AddedAt → AddedAt
	if last := bfLastActive(downloader.Torrent{AddedAt: addedAt}); !last.Equal(addedAt) {
		t.Errorf("仅 AddedAt 应返回 AddedAt, got %v", last)
	}
	// CompletedAt 优先于 AddedAt
	if last := bfLastActive(downloader.Torrent{AddedAt: addedAt, CompletedAt: completedAt}); !last.Equal(completedAt) {
		t.Errorf("CompletedAt 应优先, got %v", last)
	}
	// LastActivity 优先于 CompletedAt
	if last := bfLastActive(downloader.Torrent{AddedAt: addedAt, CompletedAt: completedAt, LastActivity: lastAct}); !last.Equal(lastAct) {
		t.Errorf("LastActivity 应最优先, got %v", last)
	}
}
