// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package auth

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func newActivationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/act.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ActivationCode{}); err != nil {
		t.Fatal(err)
	}
	// 释放文件锁，避免 Windows 下 TempDir 清理失败（LIFO：在 TempDir 清理之前关闭）
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestActivationLifecycle(t *testing.T) {
	a := NewActivation(newActivationDB(t), "secret")

	code, err := a.Generate(3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Validate(code); err != nil {
		t.Errorf("validate ok: %v", err)
	}
	// 篡改应失败
	if err := a.Validate(code + "x"); err == nil {
		t.Error("tampered code should fail validation")
	}

	if a.AnyActivated() {
		t.Error("初始应为未激活状态")
	}
	lvl, err := a.Activate(code, "dev1")
	if err != nil || lvl != 3 {
		t.Fatalf("activate: lvl=%d err=%v", lvl, err)
	}
	if !a.AnyActivated() {
		t.Error("should be activated after activate")
	}
	if !a.IsActivated("dev1") {
		t.Error("device should be activated")
	}
	// 复用应失败
	if _, err := a.Activate(code, "dev1"); err == nil {
		t.Error("reuse should fail")
	}
}

func TestActivationExpired(t *testing.T) {
	a := NewActivation(newActivationDB(t), "secret")
	past := time.Now().Add(-time.Hour)
	code, _ := a.Generate(1, &past)
	if _, err := a.Activate(code, "d"); err == nil {
		t.Error("expired code should fail")
	}
}

func TestActivationRevoked(t *testing.T) {
	db := newActivationDB(t)
	a := NewActivation(db, "secret")
	code, _ := a.Generate(1, nil)
	db.Model(&models.ActivationCode{}).Where("code = ?", code).Update("revoked", true)
	if _, err := a.Activate(code, "d"); err == nil {
		t.Error("revoked code should fail")
	}
}
