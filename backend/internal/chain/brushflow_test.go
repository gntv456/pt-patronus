package chain

import (
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestBFParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1 GB", 1 << 30},
		{"1.5 GB", 1610612736},
		{"500 MB", 500 << 20 / 1000 * 1000}, // 524288000
		{"500MB", 524288000},
		{"1024 KB", 1 << 20},
		{"1 TB", 1 << 40},
		{"1.5T", 1610612736 << 10},
		{"12345", 12345},
		{"", 0},
		{"N/A", 0},
	}
	for _, tc := range cases {
		got := bfParseSize(tc.in)
		// 允许 1% 浮点误差（500 MB 用 want 近似）
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

func TestBFFilterAndSort(t *testing.T) {
	task := &models.BrushFlowTask{MinSeeders: 1, MaxSeeders: 50, FreeOnly: true, MinSizeMB: 500}
	in := []site.Torrent{
		{ID: "1", Title: "a", DownloadURL: "u1", Size: "1 GB", Seeders: 30, Free: true},
		{ID: "2", Title: "b", DownloadURL: "u2", Size: "2 GB", Seeders: 5, Free: false},  // 非免费过滤
		{ID: "3", Title: "c", DownloadURL: "u3", Size: "3 GB", Seeders: 10, Free: true},
		{ID: "4", Title: "d", DownloadURL: "u4", Size: "4 GB", Seeders: 100, Free: true}, // seeders 超
		{ID: "5", Title: "e", DownloadURL: "u5", Size: "100 MB", Seeders: 8, Free: true}, // 体积过小
		{ID: "6", Title: "f", Size: "5 GB", Seeders: 3, Free: true},                     // 无下载链接
	}
	out := bfFilterAndSort(in, task)
	if len(out) != 2 {
		t.Fatalf("want 2 candidates, got %d: %+v", len(out), out)
	}
	// seeders 升序：c(10) 在 a(30) 前
	if out[0].ID != "3" || out[1].ID != "1" {
		t.Errorf("expect asc by seeders [3,1], got [%s,%s]", out[0].ID, out[1].ID)
	}
}

func TestBFMatchRule(t *testing.T) {
	now := time.Now()
	// 分享率命中
	tr := downloader.Torrent{Ratio: 2.0, AddedAt: now, Uploaded: 0}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteRatio: 1.5}, &tr, now); r == "" {
		t.Error("ratio rule should match")
	}
	// 做种时长命中
	old := now.Add(-2 * time.Hour)
	tr2 := downloader.Torrent{Ratio: 0.1, AddedAt: old, Uploaded: 0}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteSeedHours: 1}, &tr2, now); r == "" {
		t.Error("seed-hours rule should match")
	}
	// 上传量命中
	tr3 := downloader.Torrent{Ratio: 0.1, AddedAt: now, Uploaded: 200 << 20}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteUploadMB: 100}, &tr3, now); r == "" {
		t.Error("upload rule should match")
	}
	// 全不命中
	tr4 := downloader.Torrent{Ratio: 0.1, AddedAt: now, Uploaded: 0}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteRatio: 1.5, DeleteSeedHours: 1, DeleteUploadMB: 100}, &tr4, now); r != "" {
		t.Errorf("no rule should match, got %q", r)
	}
	// 全 0 规则 = 永不删
	if r := bfMatchRule(&models.BrushFlowTask{}, &tr, now); r != "" {
		t.Errorf("zero rules should never delete, got %q", r)
	}
}

func TestBFHasTag(t *testing.T) {
	if !bfHasTag([]string{"a", "brushflow:1"}, "brushflow:1") {
		t.Error("should find tag")
	}
	if bfHasTag([]string{"a"}, "brushflow:1") {
		t.Error("should not find tag")
	}
}

func TestBFShareLimits(t *testing.T) {
	// 全未配 → -2/-2（不限制）
	lim := bfShareLimits(&models.BrushFlowTask{})
	if lim.RatioLimit != -2 || lim.SeedingTimeLimit != -2 {
		t.Errorf("empty task want -2/-2, got %v/%v", lim.RatioLimit, lim.SeedingTimeLimit)
	}
	// 仅配 ratio
	lim = bfShareLimits(&models.BrushFlowTask{DeleteRatio: 2.5})
	if lim.RatioLimit != 2.5 || lim.SeedingTimeLimit != -2 {
		t.Errorf("ratio only want 2.5/-2, got %v/%v", lim.RatioLimit, lim.SeedingTimeLimit)
	}
	// 仅配做种时长（小时→分钟）
	lim = bfShareLimits(&models.BrushFlowTask{DeleteSeedHours: 1.5})
	if lim.RatioLimit != -2 || lim.SeedingTimeLimit != 90 {
		t.Errorf("hours 1.5 want -2/90, got %v/%v", lim.RatioLimit, lim.SeedingTimeLimit)
	}
	// 两者都配
	lim = bfShareLimits(&models.BrushFlowTask{DeleteRatio: 3, DeleteSeedHours: 2})
	if lim.RatioLimit != 3 || lim.SeedingTimeLimit != 120 {
		t.Errorf("both want 3/120, got %v/%v", lim.RatioLimit, lim.SeedingTimeLimit)
	}
}
