// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func newRegDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/reg.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RegistrationWatch{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestRegistrationOperCRUD(t *testing.T) {
	op := NewRegistrationOper(newRegDB(t))

	w, err := op.Create(RegistrationInput{Name: "好学", URL: "https://hxpt.org"})
	if err != nil {
		t.Fatal(err)
	}
	if w.SignupPath != "signup.php" {
		t.Errorf("default signup path = %q, want signup.php", w.SignupPath)
	}
	if w.LastOpen != nil {
		t.Errorf("LastOpen should be nil before first check, got %v", *w.LastOpen)
	}

	list, err := op.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %d err=%v", len(list), err)
	}

	// 首次检测：开放 → prev 应为 nil（从未检测），回写后 LastOpen=true
	prev, err := op.UpdateStatus(w.ID, true)
	if err != nil || prev != nil {
		t.Errorf("first UpdateStatus prev=%v err=%v (want nil)", prev, err)
	}
	got, _ := op.Get(w.ID)
	if got.LastOpen == nil || !*got.LastOpen {
		t.Errorf("after open, LastOpen should be true")
	}

	// 第二次检测：仍开放 → prev 应为 &true（无转换）
	prev, _ = op.UpdateStatus(w.ID, true)
	if prev == nil || !*prev {
		t.Errorf("second UpdateStatus prev=%v (want &true)", prev)
	}

	if err := op.Delete(w.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = op.List()
	if len(list) != 0 {
		t.Errorf("after delete, list len = %d", len(list))
	}
}

// P0-5 回归：GORM default:true 会把 IsActive=false 当零值吞为 true。
// 显式创建停用目标，DB 必须存 false（修复后回写），且不出现在 ListActive 中。
func TestRegistrationOperCreateInactiveNotSwallowed(t *testing.T) {
	op := NewRegistrationOper(newRegDB(t))

	off, err := op.Create(RegistrationInput{Name: "停用站", URL: "https://off.org", IsActive: false})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := op.Get(off.ID)
	if got.IsActive {
		t.Errorf("停用站 IsActive=true：被 GORM default:true 吞了，修复后应保持 false")
	}

	on, _ := op.Create(RegistrationInput{Name: "启用站", URL: "https://on.org", IsActive: true})
	gotOn, _ := op.Get(on.ID)
	if !gotOn.IsActive {
		t.Errorf("启用站 IsActive=false，应保持 true")
	}

	active, _ := op.ListActive()
	foundOn := false
	for _, a := range active {
		if a.ID == off.ID {
			t.Errorf("停用站出现在 ListActive 里（cron 会错误轮询它）")
		}
		if a.ID == on.ID {
			foundOn = true
		}
	}
	if !foundOn {
		t.Errorf("启用站未出现在 ListActive 里（应在）")
	}
}
