package oper

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func newCloudTaskTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cloudtask_test.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.CloudTask{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestCloudTaskRetryOnlyAffectsFailedRow 「重试」按钮点下去必须知道到底重排了没有：
// 条件 UPDATE 对不存在的任务或已不是 failed 的任务影响 0 行且不报错，
// 不判 RowsAffected 就会把「什么都没做」报成「已重新排队」，而任务列表刷新后仍是失败态。
func TestCloudTaskRetryOnlyAffectsFailedRow(t *testing.T) {
	db := newCloudTaskTestDB(t)
	o := NewCloudTaskOper(db)

	failed := &models.CloudTask{DriveID: 1, Kind: "strm", Status: "failed", Error: "boom", Attempts: 3}
	if err := db.Create(failed).Error; err != nil {
		t.Fatalf("seed failed task: %v", err)
	}
	running := &models.CloudTask{DriveID: 1, Kind: "strm", Status: "running", Attempts: 1}
	if err := db.Create(running).Error; err != nil {
		t.Fatalf("seed running task: %v", err)
	}

	// 不存在的 id：修复前返回 nil（界面报「已重新排队」）
	if err := o.Retry(9999); !errors.Is(err, ErrTaskNotRetryable) {
		t.Fatalf("不存在的任务应报 ErrTaskNotRetryable，got %v", err)
	}
	// 非失败态：同样不能谎报成功
	if err := o.Retry(running.ID); !errors.Is(err, ErrTaskNotRetryable) {
		t.Fatalf("running 任务不该被重排，got %v", err)
	}
	// 真失败的任务：正常重排
	if err := o.Retry(failed.ID); err != nil {
		t.Fatalf("失败任务重排应成功，got %v", err)
	}
	var got models.CloudTask
	if err := db.First(&got, failed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != "pending" || got.Attempts != 0 || got.Error != "" {
		t.Fatalf("重排后应为 pending/0/空错，got %+v", got)
	}
	// 重排一次后再点：已不是 failed，必须再次报错而不是静默成功
	if err := o.Retry(failed.ID); !errors.Is(err, ErrTaskNotRetryable) {
		t.Fatalf("重复重排应报错，got %v", err)
	}
}
