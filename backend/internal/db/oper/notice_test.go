package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// newNoticeTestDB 内存级测试库，仅迁移 Notice 表。
func newNoticeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/notice_test.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestNoticeClearAll 验证一键清空：删除全部通知（含未读），返回正确条数，空表安全。
func TestNoticeClearAll(t *testing.T) {
	op := NewNoticeOper(newNoticeTestDB(t))

	for _, n := range []*models.Notice{
		{Source: "new_mail", Title: "m1", Level: "info"},
		{Source: "dl_done", Title: "m2", Level: "success", Read: true},
		{Source: "announce", Title: "m3", Level: "warning"},
	} {
		if err := op.Create(n); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	n, err := op.ClearAll()
	if err != nil {
		t.Fatalf("clearall: %v", err)
	}
	if n != 3 {
		t.Fatalf("want 3 affected, got %d", n)
	}
	if got, _ := op.List(false, 0); len(got) != 0 {
		t.Fatalf("want empty after clear, got %d", len(got))
	}

	// 空表再清一次应安全（无错、0 条）。
	if n, err := op.ClearAll(); err != nil || n != 0 {
		t.Fatalf("clearall empty: n=%d err=%v", n, err)
	}
}
