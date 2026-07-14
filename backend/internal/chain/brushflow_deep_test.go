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
