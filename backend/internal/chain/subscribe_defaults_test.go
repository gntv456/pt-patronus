// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// 订阅显式值覆盖全局默认（含 *bool 显式 false 覆盖全局 true）。
func TestResolveEffectiveSubOverridesGlobal(t *testing.T) {
	falseV, trueV := false, true
	sub := &models.Subscription{
		Resolution:        "2160p",
		PromoTypes:        models.StringSlice{"free"},
		SiteIDs:           models.IntSlice{7},
		EnableRatingGuard: &falseV, // 显式关，必须覆盖全局开
		BestVersion:       &trueV,
	}
	gd := GlobalDefaults{
		Resolution: "1080p", PromoTypes: []string{"2x"}, SiteIDs: []uint{1, 2},
		EnableRatingGuard: true, BestVersion: false,
	}
	eff := resolveEffective(sub, gd)
	if eff.Resolution != "2160p" {
		t.Fatalf("resolution = %q, want sub 2160p", eff.Resolution)
	}
	if len(eff.PromoTypes) != 1 || eff.PromoTypes[0] != "free" {
		t.Fatalf("promo_types = %v, want sub [free]", eff.PromoTypes)
	}
	if len(eff.SiteIDs) != 1 || eff.SiteIDs[0] != 7 {
		t.Fatalf("site_ids = %v, want sub [7]", eff.SiteIDs)
	}
	if eff.EnableRatingGuard { // 显式 false 必须胜出
		t.Fatalf("enable_rating_guard = %v, want false (sub explicit override)", eff.EnableRatingGuard)
	}
	if !eff.BestVersion {
		t.Fatalf("best_version = %v, want true (sub override)", eff.BestVersion)
	}
}

// 订阅空/nil 时回退全局默认。
func TestResolveEffectiveFallsBackToGlobal(t *testing.T) {
	sub := &models.Subscription{} // 全空 / bool nil
	gd := GlobalDefaults{
		Resolution: "1080p", PromoTypes: []string{"free", "2xfree"}, SiteIDs: []uint{1, 2, 3},
		EnableRatingGuard: true, BestVersion: true,
	}
	eff := resolveEffective(sub, gd)
	if eff.Resolution != "1080p" {
		t.Fatalf("resolution = %q, want global 1080p", eff.Resolution)
	}
	if len(eff.PromoTypes) != 2 || eff.PromoTypes[1] != "2xfree" {
		t.Fatalf("promo_types = %v, want global", eff.PromoTypes)
	}
	if len(eff.SiteIDs) != 3 || eff.SiteIDs[2] != 3 {
		t.Fatalf("site_ids = %v, want global", eff.SiteIDs)
	}
	if !eff.EnableRatingGuard || !eff.BestVersion {
		t.Fatalf("nil bool should fall back to global: guard=%v best=%v", eff.EnableRatingGuard, eff.BestVersion)
	}
}

// 订阅与全局都空 → 零值（不限/全部站/关）。
func TestResolveEffectiveEmptyBoth(t *testing.T) {
	eff := resolveEffective(&models.Subscription{}, GlobalDefaults{})
	if eff.Resolution != "" || len(eff.PromoTypes) != 0 || len(eff.SiteIDs) != 0 {
		t.Fatalf("empty both should stay zero: %+v", eff)
	}
	if eff.EnableRatingGuard || eff.BestVersion {
		t.Fatalf("nil bool + empty global should resolve false")
	}
}

// 全局默认 KV 存取往返（SystemConfig 持久化 + JSON 序列化）。
func TestGlobalDefaultsRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "gd.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	cfgOp := oper.NewSystemConfigOper(db)

	// 初始未配置 → 零值。
	if gd := loadGlobalDefaults(cfgOp); gd.Resolution != "" || gd.EnableRatingGuard || gd.BestVersion {
		t.Fatalf("initial should be zero: %+v", gd)
	}

	want := GlobalDefaults{
		Resolution: "1080p", PromoTypes: []string{"free"}, SiteIDs: []uint{1},
		EnableRatingGuard: true, BestVersion: false,
	}
	if err := saveGlobalDefaults(cfgOp, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got := loadGlobalDefaults(cfgOp)
	if got.Resolution != "1080p" || len(got.PromoTypes) != 1 || got.PromoTypes[0] != "free" ||
		len(got.SiteIDs) != 1 || !got.EnableRatingGuard || got.BestVersion {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

// cfgOp 为 nil（测试/未装配）时 load 返回零值，不 panic。
func TestLoadGlobalDefaultsNilCfgOp(t *testing.T) {
	gd := loadGlobalDefaults(nil)
	if gd.Resolution != "" || gd.EnableRatingGuard {
		t.Fatalf("nil cfgOp should yield zero: %+v", gd)
	}
	// 切片须非 nil：API 序列化为 [] 而非 null，否则前端读 .length 崩溃（go-nil-slice-json-null）。
	if gd.PromoTypes == nil || gd.SiteIDs == nil {
		t.Fatalf("nil cfgOp slices must be empty-non-nil, got promo=%v site=%v", gd.PromoTypes, gd.SiteIDs)
	}
}
