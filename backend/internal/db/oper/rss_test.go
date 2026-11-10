// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func newRssDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/rss.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RssSubscription{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// P0-5 回归：GORM default:true 会把 Enabled=false 当零值吞为 true
// （RssSubscription.Enabled 标了 default:true;index）。显式创建停用订阅，
// DB 必须存 false（修复后回写），且不出现在 ListEnabled（cron 不会错误轮询）。
func TestRssOperCreateDisabledNotSwallowed(t *testing.T) {
	op := NewRssOper(newRssDB(t))

	off := &models.RssSubscription{Name: "停用订阅", URL: "https://off.org/rss", Enabled: false}
	if err := op.Create(off); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := op.Get(off.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Errorf("停用订阅 Enabled=true：被 GORM default:true 吞了，修复后应保持 false")
	}

	on := &models.RssSubscription{Name: "启用订阅", URL: "https://on.org/rss", Enabled: true}
	if err := op.Create(on); err != nil {
		t.Fatal(err)
	}
	gotOn, _ := op.Get(on.ID)
	if !gotOn.Enabled {
		t.Errorf("启用订阅 Enabled=false，应保持 true")
	}

	enabled, _ := op.ListEnabled()
	for _, e := range enabled {
		if e.ID == off.ID {
			t.Errorf("停用订阅出现在 ListEnabled 里（cron 会错误轮询它）")
		}
	}
}
