// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package db

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openSnapTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path + "?_pragma=journal_mode(WAL)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	return db
}

// 新库（db 文件不存在）不应生成快照。
func TestSnapshotSkipsMissingDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "none.db")
	gdb := openSnapTestDB(t, dbPath)
	if err := snapshotBeforeMigration(gdb, dir, dbPath); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, preMigrationDir)); !os.IsNotExist(err) {
		t.Fatalf("snapshot dir should not exist for missing db, err=%v", err)
	}
}

// 已有库须生成快照，且快照可读、含迁移前数据。
func TestSnapshotCapturesPreMigrationState(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	gdb := openSnapTestDB(t, dbPath)
	type row struct { ID uint `gorm:"primaryKey"`; Val string }
	if err := gdb.AutoMigrate(&row{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := gdb.Create(&row{ID: 1, Val: "before"}).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := snapshotBeforeMigration(gdb, dir, dbPath); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, preMigrationDir))
	if err != nil || len(entries) != 1 {
		t.Fatalf("want 1 snapshot, got %d err=%v", len(entries), err)
	}
	snap := openSnapTestDB(t, filepath.Join(dir, preMigrationDir, entries[0].Name()))
	var got row
	if err := snap.First(&got, 1).Error; err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if got.Val != "before" {
		t.Fatalf("snapshot val=%q, want before", got.Val)
	}
	// 生产库后续变更不应污染快照
	if err := gdb.Model(&row{}).Where("id = 1").Update("val", "after").Error; err != nil {
		t.Fatalf("update: %v", err)
	}
	var snapVal row
	_ = snap.First(&snapVal, 1)
	if snapVal.Val != "before" {
		t.Fatalf("snapshot mutated after prod change: %q", snapVal.Val)
	}
}

// trim 仅保留最近 5 份。
func TestTrimSnapshotsKeepsLatestFive(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("pre-migration-2026010%d.db", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := trimSnapshots(dir); err != nil {
		t.Fatalf("trim: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != preMigrationKeep {
		t.Fatalf("kept %d, want %d", len(entries), preMigrationKeep)
	}
}
