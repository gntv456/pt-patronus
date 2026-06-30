package chain

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// newTrendDB 建一个仅迁移 site_stats 的内存文件库（Trend 的 SQL 只读该表）。
// 关连接释放文件锁，避免 Windows TempDir 清理失败（对齐 newChainDB）。
func newTrendDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/trend.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SiteStat{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestDashboardTrend(t *testing.T) {
	db := newTrendDB(t)
	chain := &DashboardChain{db: db}

	// 3 天（-3 最旧 → -1 最新，均落在 7 天窗口内、不含今日以保证恰好 3 天）。
	// site1 的最旧日插入两条同日快照（uploaded 100 / 200）以验证「MAX-再-SUM」：
	// 若直接 SUM 原始行，site1 当日会算成 300（错），正确聚合应取 MAX=200。
	day := func(offset int) time.Time {
		// 固定到当日 12:00，避免跨日边界与「今日」重叠。
		return time.Now().AddDate(0, 0, offset)
	}
	// 注：snap_at 精度到秒即可，按 (site,day) 组合插入。
	rows := []models.SiteStat{
		// day -3（最旧，points[0]）：site1 双快照
		{SiteID: 1, Uploaded: 100, Downloaded: 50, Seeding: 8, SeedingSize: 800, Bonus: 60, SnapAt: day(-3)},
		{SiteID: 1, Uploaded: 200, Downloaded: 50, Seeding: 10, SeedingSize: 1000, Bonus: 100, SnapAt: day(-3).Add(time.Hour)},
		{SiteID: 2, Uploaded: 50, Downloaded: 20, Seeding: 5, SeedingSize: 500, Bonus: 50, SnapAt: day(-3)},
		// day -2（points[1]）
		{SiteID: 1, Uploaded: 300, Downloaded: 80, Seeding: 12, SeedingSize: 1500, Bonus: 120, SnapAt: day(-2)},
		{SiteID: 2, Uploaded: 100, Downloaded: 40, Seeding: 6, SeedingSize: 700, Bonus: 70, SnapAt: day(-2)},
		// day -1（最新，points[2]）
		{SiteID: 1, Uploaded: 500, Downloaded: 100, Seeding: 15, SeedingSize: 2000, Bonus: 150, SnapAt: day(-1)},
		{SiteID: 2, Uploaded: 200, Downloaded: 60, Seeding: 7, SeedingSize: 900, Bonus: 90, SnapAt: day(-1)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	got, err := chain.Trend(context.Background(), 7)
	if err != nil {
		t.Fatalf("Trend: %v", err)
	}
	if len(got.Points) != 3 {
		t.Fatalf("want 3 day points, got %d (%+v)", len(got.Points), got.Points)
	}

	// points 按日升序：[0]=day-3, [1]=day-2, [2]=day-1。
	p0, p2 := got.Points[0], got.Points[2]

	// day -3：site1 MAX(uploaded)=200（不是 300）+ site2=50 → 250。
	if p0.Uploaded != 250 {
		t.Errorf("day-3 uploaded: want 250 (MAX-then-SUM), got %d", p0.Uploaded)
	}
	if p0.Downloaded != 70 { // 50 + 20
		t.Errorf("day-3 downloaded: want 70, got %d", p0.Downloaded)
	}
	if p0.Seeding != 15 { // 10 + 5
		t.Errorf("day-3 seeding: want 15, got %d", p0.Seeding)
	}
	if p0.SeedingSize != 1500 { // 1000 + 500
		t.Errorf("day-3 seeding_size: want 1500, got %d", p0.SeedingSize)
	}
	if p0.Bonus != 150 { // 100 + 50
		t.Errorf("day-3 bonus: want 150, got %v", p0.Bonus)
	}
	// ratio = uploaded/downloaded，当日跨站聚合总分享率。
	wantRatio := float64(250) / float64(70)
	if p0.Ratio < wantRatio-1e-6 || p0.Ratio > wantRatio+1e-6 {
		t.Errorf("day-3 ratio: want %v, got %v", wantRatio, p0.Ratio)
	}

	// day -1：500+200=700，ratio=700/160。
	if p2.Uploaded != 700 {
		t.Errorf("day-1 uploaded: want 700, got %d", p2.Uploaded)
	}
	wantRatio2 := float64(700) / float64(160)
	if p2.Ratio < wantRatio2-1e-6 || p2.Ratio > wantRatio2+1e-6 {
		t.Errorf("day-1 ratio: want %v, got %v", wantRatio2, p2.Ratio)
	}
}
