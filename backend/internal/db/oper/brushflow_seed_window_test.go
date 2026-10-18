package oper

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// TestBrushFlowSeedWindowAccount 种子历史的单页窗口：长期任务加种超过 500 条后，
// 老记录被窗口挤出去——清单只回 500 条时若不带 total/truncated，
// 界面就把它画成「这个任务只加过这些种」，更早加入、可能仍在做种的那些在界面上凭空消失。
func TestBrushFlowSeedWindowAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/bfseeds.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.BrushFlowTask{}, &models.BrushFlowSeed{}); err != nil {
		t.Fatal(err)
	}
	o := NewBrushFlowOper(db)
	for i := 0; i < SeedListLimit+7; i++ {
		if err := o.AddSeed(&models.BrushFlowSeed{
			TaskID: 1, SourceKey: fmt.Sprintf("s-%d", i), Title: fmt.Sprintf("t-%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := o.AddSeed(&models.BrushFlowSeed{
			TaskID: 2, SourceKey: fmt.Sprintf("u-%d", i), Title: fmt.Sprintf("u-t-%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	seeds, err := o.ListSeeds(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != SeedListLimit {
		t.Errorf("清单应被窗口夹在 %d 条，got %d", SeedListLimit, len(seeds))
	}
	total, err := o.CountSeeds(1)
	if err != nil {
		t.Fatal(err)
	}
	if total != int64(SeedListLimit+7) {
		t.Errorf("总数应为 %d（窗口外的 7 条也要上账），got %d", SeedListLimit+7, total)
	}
	if !(total > int64(len(seeds))) {
		t.Error("总数大于清单长度时截断判断必须为真，否则被挤掉的老种子永远不上账")
	}

	// 真库层：Count 与 List 同按 task_id 过滤，否则 A 任务的窗口会被 B 任务的条数顶歪。
	t2, err := o.CountSeeds(2)
	if err != nil {
		t.Fatal(err)
	}
	if t2 != 3 {
		t.Errorf("任务 2 的总数应为 3（不是 510），got %d", t2)
	}
	seeds2, err := o.ListSeeds(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds2) != 3 {
		t.Errorf("任务 2 的清单应 3 条，got %d", len(seeds2))
	}
	if t2 > int64(len(seeds2)) {
		t.Errorf("一次拿全时截断判断必须为假（恒真是另一种说谎），total=%d len=%d", t2, len(seeds2))
	}
}
