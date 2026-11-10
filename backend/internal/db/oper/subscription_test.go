// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// newSubDB 建一个已 AutoMigrate 的内存级 sqlite 订阅库。
func newSubDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sub.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Subscription{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// UpdateEditableFields 仅写用户可编辑字段，不覆盖 cron 维护的
// last_checked_at/current_priority/episode_priority（缺陷 4 修复）。
func TestUpdateEditableFieldsPreservesCronFields(t *testing.T) {
	db := newSubDB(t)
	op := NewSubscriptionOper(db)

	// 建一条订阅，并模拟 cron 已写入 last_checked_at/current_priority/episode_priority。
	now := time.Now()
	truth := true
	s := &models.Subscription{
		Source: "tmdb", SourceID: "tv:1", Title: "Show", Type: "tv", Category: "tv",
		Status: "subscribed", BestVersion: &truth,
	}
	if err := op.Create(s); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.Subscription{}).Where("id = ?", s.ID).Updates(map[string]any{
		"last_checked_at":  now,
		"current_priority": 80,
		"episode_priority": `{"S01E01":80}`,
	}).Error; err != nil {
		t.Fatal(err)
	}

	// 用户编辑（经 handler Update → SubscribeChain.Update → UpdateEditableFields）：
	// 改 resolution/status，但 Get 出的 s 仍带旧的 cron 字段快照（模拟并发场景下 handler 读到的旧值）。
	loaded, err := op.Get(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Resolution = "2160p"
	loaded.Status = "paused"
	// 关键：loaded.LastCheckedAt 是 now（Get 读出的快照），但即便 handler 把它改成零值，
	// UpdateEditableFields 也不应写入该列——此处显式置零验证不被持久化。
	loaded.LastCheckedAt = nil
	loaded.CurrentPriority = 0
	loaded.EpisodePriority = ""
	if err := op.UpdateEditableFields(loaded); err != nil {
		t.Fatal(err)
	}

	// 重新读，验证可编辑字段已更新、cron 字段保持原值（未被零值覆盖）。
	got, err := op.Get(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resolution != "2160p" || got.Status != "paused" {
		t.Fatalf("editable fields not updated: resolution=%q status=%q", got.Resolution, got.Status)
	}
	// 关键断言：cron 维护字段未被覆盖。
	if got.CurrentPriority != 80 {
		t.Errorf("current_priority overwritten: got %d want 80 (cron field must be preserved)", got.CurrentPriority)
	}
	if got.EpisodePriority != `{"S01E01":80}` {
		t.Errorf("episode_priority overwritten: got %q want {\"S01E01\":80}", got.EpisodePriority)
	}
	if got.LastCheckedAt == nil || got.LastCheckedAt.IsZero() {
		t.Errorf("last_checked_at cleared: got %v want non-nil %v", got.LastCheckedAt, now)
	}
}

// UpdateEditableFields 能把 *bool 置 nil（ResetFields 场景：回退全局默认）。
func TestUpdateEditableFieldsNullBool(t *testing.T) {
	db := newSubDB(t)
	op := NewSubscriptionOper(db)
	truth := true
	s := &models.Subscription{Source: "tmdb", SourceID: "tv:2", Title: "X", Type: "tv",
		Category: "tv", BestVersion: &truth}
	if err := op.Create(s); err != nil {
		t.Fatal(err)
	}
	loaded, _ := op.Get(s.ID)
	loaded.BestVersion = nil // ResetFields 把 *bool 置 nil
	if err := op.UpdateEditableFields(loaded); err != nil {
		t.Fatal(err)
	}
	got, _ := op.Get(s.ID)
	if got.BestVersion != nil {
		t.Errorf("best_version should be NULL, got %v", *got.BestVersion)
	}
}

// HasAnyHit 按订阅维度判断「是否已推过种子」（电影洗版无画质信息时的基线判据）：
// 必须只看本订阅，别把别的订阅的命中算进来。
func TestHasAnyHitPerSubscription(t *testing.T) {
	db := newSubDB(t)
	if err := db.AutoMigrate(&models.SubscriptionHit{}); err != nil {
		t.Fatal(err)
	}
	op := NewSubscriptionOper(db)
	truth := true
	s1 := &models.Subscription{Source: "tmdb", SourceID: "m:1", Title: "Movie A", Type: "movie",
		Status: "subscribed", BestVersion: &truth}
	s2 := &models.Subscription{Source: "tmdb", SourceID: "m:2", Title: "Movie B", Type: "movie",
		Status: "subscribed", BestVersion: &truth}
	for _, s := range []*models.Subscription{s1, s2} {
		if err := op.Create(s); err != nil {
			t.Fatal(err)
		}
	}

	for _, id := range []uint{s1.ID, s2.ID} {
		if ok, err := op.HasAnyHit(id); err != nil || ok {
			t.Fatalf("fresh subscription sub=%d: HasAnyHit=%v err=%v, want false/nil", id, ok, err)
		}
	}
	if err := op.AddHit(&models.SubscriptionHit{SubscriptionID: s1.ID, TorrentKey: "k1", Title: "T"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := op.HasAnyHit(s1.ID); err != nil || !ok {
		t.Errorf("after hit: HasAnyHit=%v err=%v, want true/nil", ok, err)
	}
	if ok, err := op.HasAnyHit(s2.ID); err != nil || ok {
		t.Errorf("other subscription must stay false: HasAnyHit=%v err=%v", ok, err)
	}
}
