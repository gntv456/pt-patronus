package db

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func TestMigrateSubscriptionNullableBooleansRunsOnlyOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.SystemConfig{}, &models.Subscription{}); err != nil {
		t.Fatal(err)
	}

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

	if err := migrateSubscriptionNullableBooleans(db); err != nil {
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
	if err := migrateSubscriptionNullableBooleans(db); err != nil {
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
