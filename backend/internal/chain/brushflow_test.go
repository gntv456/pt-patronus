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
		{ID: "2", Title: "b", DownloadURL: "u2", Size: "2 GB", Seeders: 5, Free: false}, // 非免费过滤
		{ID: "3", Title: "c", DownloadURL: "u3", Size: "3 GB", Seeders: 10, Free: true},
		{ID: "4", Title: "d", DownloadURL: "u4", Size: "4 GB", Seeders: 100, Free: true}, // seeders 超
		{ID: "5", Title: "e", DownloadURL: "u5", Size: "100 MB", Seeders: 8, Free: true}, // 体积过小
		{ID: "6", Title: "f", Size: "5 GB", Seeders: 3, Free: true},                      // 无下载链接
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
	// 全未配 → -1/-1（全局默认，不覆盖下载器设置）
	lim := bfShareLimits(&models.BrushFlowTask{})
	if lim.RatioLimit != -1 || lim.SeedingTimeLimit != -1 {
		t.Errorf("empty task want -1/-1, got %v/%v", lim.RatioLimit, lim.SeedingTimeLimit)
	}
	// 仅配 ratio
	lim = bfShareLimits(&models.BrushFlowTask{DeleteRatio: 2.5})
	if lim.RatioLimit != 2.5 || lim.SeedingTimeLimit != -1 {
		t.Errorf("ratio only want 2.5/-1, got %v/%v", lim.RatioLimit, lim.SeedingTimeLimit)
	}
	// 仅配做种时长（小时→分钟）
	lim = bfShareLimits(&models.BrushFlowTask{DeleteSeedHours: 1.5})
	if lim.RatioLimit != -1 || lim.SeedingTimeLimit != 90 {
		t.Errorf("hours 1.5 want -1/90, got %v/%v", lim.RatioLimit, lim.SeedingTimeLimit)
	}
	// 两者都配
	lim = bfShareLimits(&models.BrushFlowTask{DeleteRatio: 3, DeleteSeedHours: 2})
	if lim.RatioLimit != 3 || lim.SeedingTimeLimit != 120 {
		t.Errorf("both want 3/120, got %v/%v", lim.RatioLimit, lim.SeedingTimeLimit)
	}
}

func TestBFMatchRuleEnhanced(t *testing.T) {
	now := time.Now()
	base := downloader.Torrent{
		Ratio: 2.5, Uploaded: 10 << 20, UPRate: 100, Progress: 1,
		AddedAt: now.Add(-3 * time.Hour), CompletedAt: now.Add(-2 * time.Hour),
		LastActivity: now.Add(-90 * time.Minute), State: "uploading",
	}

	// or 默认：ratio 命中
	if r := bfMatchRule(&models.BrushFlowTask{DeleteRatio: 2}, &base, now); r == "" {
		t.Fatal("or ratio should hit")
	}
	// and：ratio 命中但 seed hours 未到 → 不删
	if r := bfMatchRule(&models.BrushFlowTask{DeleteRatio: 2, DeleteSeedHours: 10, DeleteLogic: "and"}, &base, now); r != "" {
		t.Fatalf("and should miss, got %q", r)
	}
	// and：两项都命中
	if r := bfMatchRule(&models.BrushFlowTask{DeleteRatio: 2, DeleteSeedHours: 1, DeleteLogic: "and"}, &base, now); r == "" {
		t.Fatal("and both should hit")
	}
	// 低上传速度
	slow := base
	slow.UPRate = 500 // 0.5 KB/s
	if r := bfMatchRule(&models.BrushFlowTask{DeleteMinUPRateKB: 2}, &slow, now); r == "" {
		t.Fatal("min uprate should hit")
	}
	// 下载中不因低速误杀
	dl := base
	dl.Progress = 0.5
	dl.CompletedAt = time.Time{}
	dl.State = "downloading"
	dl.UPRate = 0
	if r := bfMatchRule(&models.BrushFlowTask{DeleteMinUPRateKB: 2}, &dl, now); r != "" {
		t.Fatalf("downloading should not hit min uprate, got %q", r)
	}
	// 不活跃
	if r := bfMatchRule(&models.BrushFlowTask{DeleteInactiveHours: 1}, &base, now); r == "" {
		t.Fatal("inactive should hit")
	}
	// 表达式阶梯
	exprTask := &models.BrushFlowTask{DeleteRuleExpr: "(ratio>=1 && seed_hours>=24) || ratio>=2"}
	if r := bfMatchRule(exprTask, &base, now); r == "" {
		t.Fatal("expr ratio>=2 should hit")
	}
	low := base
	low.Ratio = 1.2
	low.AddedAt = now.Add(-2 * time.Hour)
	if r := bfMatchRule(exprTask, &low, now); r != "" {
		t.Fatalf("expr should miss for young low ratio, got %q", r)
	}
	old := base
	old.Ratio = 1.2
	old.AddedAt = now.Add(-30 * time.Hour)
	if r := bfMatchRule(exprTask, &old, now); r == "" {
		t.Fatal("expr ladder old+ratio>=1 should hit")
	}
}

func TestBFEvalExpr(t *testing.T) {
	now := time.Now()
	tr := &downloader.Torrent{Ratio: 1.5, Uploaded: 2048 << 20, UPRate: 3 * 1024, AddedAt: now.Add(-10 * time.Hour)}
	ok, _, err := bfEvalExpr("ratio>=1 && upload_mb>=1000", tr, now)
	if err != nil || !ok {
		t.Fatalf("want true, ok=%v err=%v", ok, err)
	}
	ok, _, err = bfEvalExpr("ratio>=3 || up_rate_kb<1", tr, now)
	if err != nil || ok {
		t.Fatalf("want false, ok=%v err=%v", ok, err)
	}
	if _, _, err := bfEvalExpr("ratio>=", tr, now); err == nil {
		t.Fatal("broken expr should error")
	}
}

func TestBfMatchRuleSwarm(t *testing.T) {
	now := time.Now()
	tr := &downloader.Torrent{
		Ratio: 0.1, Progress: 1, State: "uploading",
		NumComplete: 100, NumIncomplete: 0,
		AddedAt: now.Add(-2 * time.Hour),
	}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteMaxSwarmSeeders: 50}, tr, now); r == "" {
		t.Fatal("expected delete when swarm seeders high")
	}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteMinSwarmLeechers: 1}, tr, now); r == "" {
		t.Fatal("expected delete when swarm leechers low")
	}
	// expr
	if r := bfMatchRule(&models.BrushFlowTask{DeleteRuleExpr: "swarm_seeders>80"}, tr, now); r == "" {
		t.Fatal("expr swarm_seeders")
	}
	// swarm 数据全 0（下载器未提供）不得因 leechers 规则误删
	unknown := &downloader.Torrent{Progress: 1, State: "uploading", NumComplete: 0, NumIncomplete: 0}
	if r := bfMatchRule(&models.BrushFlowTask{DeleteMinSwarmLeechers: 1}, unknown, now); r != "" {
		t.Fatalf("unknown swarm should not delete, got %q", r)
	}
}
