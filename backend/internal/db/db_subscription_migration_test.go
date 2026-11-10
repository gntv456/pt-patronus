// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package db

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.SystemConfig{}, &models.Subscription{}, &schemaMigrationRow{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// TestMigrateSubscriptionNullableBooleansRunsOnlyOnce 收编进 runner 后语义不变：
// 历史存量 false → NULL 一次性回填；回填后显式 false 不再被后续启动触碰。
func TestMigrateSubscriptionNullableBooleansRunsOnlyOnce(t *testing.T) {
	db := openTestDB(t)

	falseValue, trueValue := false, true
	historicalFalse := &models.Subscription{
		Source: "tmdb", SourceID: "movie:false", Title: "Historical false",
		EnableRatingGuard: &falseValue, BestVersion: &falseValue,
	}
	historicalTrue := &models.Subscription{
		Source: "tmdb", SourceID: "movie:true", Title: "Historical true",
		EnableRatingGuard: &trueValue, BestVersion: &trueValue,
	}
	if err := db.Create(historicalFalse).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(historicalTrue).Error; err != nil {
		t.Fatal(err)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	var migratedFalse, migratedTrue models.Subscription
	if err := db.First(&migratedFalse, historicalFalse.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&migratedTrue, historicalTrue.ID).Error; err != nil {
		t.Fatal(err)
	}
	if migratedFalse.EnableRatingGuard != nil || migratedFalse.BestVersion != nil {
		t.Fatalf("historical false values were not converted: %+v", migratedFalse)
	}
	if migratedTrue.EnableRatingGuard == nil || !*migratedTrue.EnableRatingGuard ||
		migratedTrue.BestVersion == nil || !*migratedTrue.BestVersion {
		t.Fatalf("historical true values changed: %+v", migratedTrue)
	}

	// This represents an explicit opt-out saved after the first application start.
	if err := db.Model(&models.Subscription{}).Where("id = ?", historicalFalse.ID).
		Updates(map[string]any{"enable_rating_guard": false, "best_version": false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err != nil { // 第二次启动：已应用必须跳过
		t.Fatal(err)
	}
	var explicitFalse models.Subscription
	if err := db.First(&explicitFalse, historicalFalse.ID).Error; err != nil {
		t.Fatal(err)
	}
	if explicitFalse.EnableRatingGuard == nil || *explicitFalse.EnableRatingGuard ||
		explicitFalse.BestVersion == nil || *explicitFalse.BestVersion {
		t.Fatalf("explicit false values were changed on a later startup: %+v", explicitFalse)
	}
}

// TestRunMigrations_OldSystemConfigMarkerCompat 老库用 SystemConfig key 标记过的
// 迁移不再重复回填（升级到版本化框架后的兼容路径）。
func TestRunMigrations_OldSystemConfigMarkerCompat(t *testing.T) {
	db := openTestDB(t)
	if err := db.Create(&models.SystemConfig{
		Key: "MigrationSubscriptionNullableBooleansV1", Value: "done",
	}).Error; err != nil {
		t.Fatal(err)
	}
	falseValue := false
	if err := db.Create(&models.Subscription{
		Source: "tmdb", SourceID: "m1", Title: "explicit false",
		EnableRatingGuard: &falseValue,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	var s models.Subscription
	if err := db.First(&s, "source_id = ?", "m1").Error; err != nil {
		t.Fatal(err)
	}
	if s.EnableRatingGuard == nil || *s.EnableRatingGuard {
		t.Fatal("old-marker library must not be backfilled again")
	}
	// schema_migrations 记录必须补写（新框架此后接管幂等）
	var n int64
	db.Model(&schemaMigrationRow{}).Where("version = ?", 1).Count(&n)
	if n != 1 {
		t.Fatalf("schema_migrations rows for v1 = %d, want 1", n)
	}
}

// TestMigrations_VersionsStrictlyIncreasing 版本号严格递增且从 1 开始——
// 防止追加迁移时用错版本号导致跳过/重复执行。
func TestMigrations_VersionsStrictlyIncreasing(t *testing.T) {
	if len(Migrations) == 0 {
		t.Fatal("Migrations must not be empty")
	}
	for i, m := range Migrations {
		if m.Version <= 0 {
			t.Fatalf("migration[%d] version must be > 0, got %d", i, m.Version)
		}
		if m.Name == "" {
			t.Fatalf("migration[%d] missing name", i)
		}
		if m.Fn == nil {
			t.Fatalf("migration[%d] missing Fn", i)
		}
		if i > 0 && m.Version <= Migrations[i-1].Version {
			t.Fatalf("migration versions must strictly increase: [%d]=%d <= [%d]=%d",
				i, m.Version, i-1, Migrations[i-1].Version)
		}
	}
}

// TestRunMigrations_FailureAtomicity 迁移失败不写执行记录（事务回滚）——
// 下次启动重跑同一迁移，绝不留下半迁移状态。
func TestRunMigrations_FailureAtomicity(t *testing.T) {
	db := openTestDB(t)
	saved := Migrations
	t.Cleanup(func() { Migrations = saved })

	Migrations = append(Migrations, Migration{
		Version: saved[len(saved)-1].Version + 1,
		Name:    "boom_migration",
		Fn: func(tx *gorm.DB) error {
			// 先写点东西再失败——回滚后必须全部消失
			if err := tx.Create(&models.SystemConfig{Key: "boom_side_effect", Value: "x"}).Error; err != nil {
				return err
			}
			return errors.New("intentional failure")
		},
	})
	if err := RunMigrations(db); err == nil {
		t.Fatal("expected migration failure to propagate")
	}
	var n int64
	db.Model(&schemaMigrationRow{}).Where("name = ?", "boom_migration").Count(&n)
	if n != 0 {
		t.Fatal("failed migration must not be recorded as applied")
	}
	var sideEffect int64
	db.Model(&models.SystemConfig{}).Where("key = ?", "boom_side_effect").Count(&sideEffect)
	if sideEffect != 0 {
		t.Fatal("failed migration side effects must be rolled back")
	}
}
