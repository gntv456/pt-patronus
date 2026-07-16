package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// newTransferTestDB 内存级测试库，仅迁移 TransferHistory 表。
func newTransferTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/transfer_test.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.TransferHistory{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestTransferHistoryDeleteMany 验证批量删除：存在的删掉、不存在的计入 failed、空入参安全。
func TestTransferHistoryDeleteMany(t *testing.T) {
	db := newTransferTestDB(t)
	op := NewTransferHistoryOper(db)
	for _, title := range []string{"A", "B", "C"} {
		if err := db.Create(&models.TransferHistory{Title: title, Status: "success"}).Error; err != nil {
			t.Fatalf("seed %s: %v", title, err)
		}
	}
	var hs []models.TransferHistory
	if err := db.Find(&hs).Error; err != nil || len(hs) != 3 {
		t.Fatalf("seed: %d err=%v", len(hs), err)
	}

	// 删前两个 + 一个不存在的 id
	ids := []uint{hs[0].ID, hs[1].ID, 9999}
	failed, err := op.DeleteMany(ids)
	if err != nil {
		t.Fatalf("DeleteMany: %v", err)
	}
	if len(failed) != 1 || failed[0] != 9999 {
		t.Errorf("failed = %v, want [9999]", failed)
	}

	var remain []models.TransferHistory
	if err := db.Find(&remain).Error; err != nil {
		t.Fatal(err)
	}
	if len(remain) != 1 || remain[0].Title != "C" {
		t.Errorf("remain = %+v, want only C", remain)
	}

	// 空入参安全（无错、nil failed）
	if f, err := op.DeleteMany(nil); err != nil || f != nil {
		t.Errorf("DeleteMany(nil) = %v %v, want nil nil", f, err)
	}
}

// TestTransferHistoryDeleteByStatus 验证按 status 批量删：只删 failed，success 保留；空 status 安全。
func TestTransferHistoryDeleteByStatus(t *testing.T) {
	db := newTransferTestDB(t)
	op := NewTransferHistoryOper(db)
	for _, h := range []models.TransferHistory{
		{Title: "ok1", Status: "success"},
		{Title: "fail1", Status: "failed"},
		{Title: "fail2", Status: "failed"},
	} {
		if err := db.Create(&h).Error; err != nil {
			t.Fatalf("seed %s: %v", h.Title, err)
		}
	}

	n, err := op.DeleteByStatus("failed")
	if err != nil {
		t.Fatalf("DeleteByStatus: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted = %d, want 2", n)
	}
	var remain []models.TransferHistory
	if err := db.Find(&remain).Error; err != nil {
		t.Fatal(err)
	}
	if len(remain) != 1 || remain[0].Title != "ok1" || remain[0].Status != "success" {
		t.Errorf("remain = %+v, want only success ok1", remain)
	}

	// 空 status 不删任何（防误清）
	if n, err := op.DeleteByStatus(""); err != nil || n != 0 {
		t.Errorf("DeleteByStatus(\"\") = %d %v, want 0 nil", n, err)
	}
}
