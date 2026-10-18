package oper

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// GORM 的 Save 在「主键非零但库里没这条」时是 INSERT 而不是失败。
// 刷流任务的编辑走全量 Save：从前 PUT /brushflow/999999 会凭空落一条任务，
// 随后 GET 就能读出这条全零记录（前端还会把它当成一条真任务列出来）。
func TestBrushflowUpdateTaskRejectsMissingRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "bf.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// Windows 上不关句柄会让 TempDir 清理失败（表现为整个用例红，与断言无关）。
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.BrushFlowTask{}, &models.BrushFlowSeed{}); err != nil {
		t.Fatal(err)
	}
	o := NewBrushFlowOper(db)

	err = o.UpdateTask(&models.BrushFlowTask{ID: 999999, Name: "幽灵任务"})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("编辑不存在的任务必须回 ErrRecordNotFound（供接口判 404），got %v", err)
	}

	var n int64
	if e := db.Model(&models.BrushFlowTask{}).Count(&n).Error; e != nil {
		t.Fatal(e)
	}
	if n != 0 {
		t.Fatalf("库里不得因这次失败编辑多出记录（防 Save 退化成 INSERT），count = %d", n)
	}

	// 正向对照：真实存在的任务仍能整字段保存（包括把关键词清空这种「零值也要写回」的编辑）。
	created := &models.BrushFlowTask{ID: 7, Name: "t7", Keyword: "1080p", MaxSeeds: 5}
	if e := o.CreateTask(created); e != nil {
		t.Fatal(e)
	}
	if e := o.UpdateTask(&models.BrushFlowTask{ID: 7, Name: "t7改", Keyword: "", MaxSeeds: 8}); e != nil {
		t.Fatalf("已存在任务的编辑不应失败：%v", e)
	}
	back, e := o.GetTask(7)
	if e != nil {
		t.Fatal(e)
	}
	if back.Name != "t7改" || back.Keyword != "" || back.MaxSeeds != 8 {
		t.Errorf("全字段保存丢失：name=%q keyword=%q max_seeds=%d", back.Name, back.Keyword, back.MaxSeeds)
	}
}
