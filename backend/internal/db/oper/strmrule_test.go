// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func newStrmRuleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/strm.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.StrmRule{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// StrmRule 的 Recursive/KeepExt/Enabled 均带 gorm:"default:true"。
// 显式创建全 false 的规则，DB 必须存 false（map Create 绕开 default 机制），否则会被吞为 true。
func TestStrmRuleCreateBooleansNotSwallowed(t *testing.T) {
	op := &StrmRuleOper{db: newStrmRuleDB(t)}
	fv := false
	m, err := op.Create(StrmRuleInput{
		Name: "禁用规则", DriveID: 1, CloudPath: "/c", OutputDir: "/o",
		Enabled: &fv, Recursive: &fv, KeepExt: &fv,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := op.Get(m.ID)
	if got.Enabled {
		t.Errorf("Enabled=true：被 gorm default:true 吞了，应保持 false")
	}
	if got.Recursive {
		t.Errorf("Recursive=true：应保持 false")
	}
	if got.KeepExt {
		t.Errorf("KeepExt=true：应保持 false")
	}
	// 默认值路径：不传 bool → 三者应为 true
	m2, err := op.Create(StrmRuleInput{Name: "默认", DriveID: 2, CloudPath: "/c2", OutputDir: "/o2"})
	if err != nil {
		t.Fatal(err)
	}
	got2, _ := op.Get(m2.ID)
	if !got2.Enabled || !got2.Recursive || !got2.KeepExt {
		t.Errorf("默认值 got E=%v R=%v K=%v，应全 true", got2.Enabled, got2.Recursive, got2.KeepExt)
	}
	// 仅启用列表不应包含禁用规则
	enabled, _ := op.ListEnabled()
	for _, e := range enabled {
		if e.ID == m.ID {
			t.Errorf("禁用规则出现在 ListEnabled 里")
		}
	}
}
