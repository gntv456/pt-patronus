package oper

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func TestWorkflowResetStateDoesNotIncrementRunCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/workflow.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&models.Workflow{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	wf := models.Workflow{Name: "test", State: "failed", RunCount: 7, LastResult: "boom", LastRunAt: &now}
	if err := db.Create(&wf).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewWorkflowOper(db).ResetState(wf.ID); err != nil {
		t.Fatal(err)
	}
	var got models.Workflow
	if err := db.First(&got, wf.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.State != "idle" || got.LastResult != "" {
		t.Fatalf("unexpected reset state: state=%q result=%q", got.State, got.LastResult)
	}
	if got.RunCount != 7 || got.LastRunAt == nil {
		t.Fatalf("reset changed run history: count=%d last=%v", got.RunCount, got.LastRunAt)
	}
}
