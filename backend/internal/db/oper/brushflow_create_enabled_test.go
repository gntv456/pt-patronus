package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// TestCreateTaskWritesExplicitDisabled 回归（实测踩出来的 GORM 语义）：
// 字段带 `gorm:"default:true"` 时，结构体里零值 false 会被当成「未赋值」而跳过该列，
// 由 DB 默认值补成 true，连内存里的结构体都被回填成 true。
// 后果：用户在「新建刷流任务时就关掉启用」，任务仍是启用状态，
// 直接进 10 分钟选种 cron 往下载器里加种子——而他以为它是停的。
func TestCreateTaskWritesExplicitDisabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/bf.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.BrushFlowTask{}); err != nil {
		t.Fatal(err)
	}
	op := NewBrushFlowOper(db)

	off := &models.BrushFlowTask{Name: "off", Enabled: false, SiteID: 1, DownloaderID: 1}
	if err := op.CreateTask(off); err != nil {
		t.Fatal(err)
	}
	var got models.BrushFlowTask
	if err := db.First(&got, off.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatalf("用户选了不启用，库里却是 enabled=true（新建即被 cron 启用）")
	}
	// 回执也不能骗人：CreateTask 之后结构体本身必须还是 false，否则 API 会回一个「已启用」。
	if off.Enabled {
		t.Errorf("结构体被回填成 true，接口回执会跟着说谎")
	}

	// 正控：显式启用照常写入。
	on := &models.BrushFlowTask{Name: "on", Enabled: true, SiteID: 1, DownloaderID: 1}
	if err := op.CreateTask(on); err != nil {
		t.Fatal(err)
	}
	var gotOn models.BrushFlowTask
	if err := db.Where("id = ?", on.ID).First(&gotOn).Error; err != nil {
		t.Fatal(err)
	}
	if !gotOn.Enabled {
		t.Fatalf("显式启用必须写入 true，got false")
	}
}
