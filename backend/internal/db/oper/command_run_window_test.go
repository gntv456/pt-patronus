// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// TestCommandRunWindowAccount 运维命令审计台账的取数账面。
// 旧写法 `limit <= 0 || limit > 100` 都把 limit 折回 30：要 500 条的人只拿到 30 条，
// 而响应与清单同形，界面上「这一页就是全部」的谎没人能拆穿。
func TestCommandRunWindowAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cmdrun.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.OpsCommandRun{}); err != nil {
		t.Fatal(err)
	}
	o := NewCommandRunOper(db)
	for i := 0; i < 90; i++ {
		if err := db.Create(&models.OpsCommandRun{Source: "web", Status: "success"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 30; i++ {
		if err := db.Create(&models.OpsCommandRun{Source: "cron", Status: "failed"}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if got := NormalizeCommandRunLimit(0); got != CommandRunListDefault {
		t.Errorf("留空应取默认 %d，got %d", CommandRunListDefault, got)
	}
	if got := NormalizeCommandRunLimit(500); got != CommandRunListLimit {
		t.Errorf("超上限应取上限 %d 而不是退回默认 %d，got %d", CommandRunListLimit, CommandRunListDefault, got)
	}

	all, err := o.List("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != CommandRunListDefault {
		t.Errorf("清单应被窗口夹在 %d 条，got %d", CommandRunListDefault, len(all))
	}
	total, err := o.Count("")
	if err != nil {
		t.Fatal(err)
	}
	if total != 120 {
		t.Errorf("总数应为 120，got %d", total)
	}
	if !(total > int64(len(all))) {
		t.Error("一页装不下时截断判断必须为真，否则翻不到的失败批次等于不存在")
	}

	// 取上限那一档：真拿到 100 条，而不是被悄悄折回 30。
	big, err := o.List("", 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(big) != CommandRunListLimit {
		t.Errorf("limit=500 应回 %d 条，got %d", CommandRunListLimit, len(big))
	}

	// 真库层：Count 与 List 同按 source 过滤，否则 web 页的账面会被 cron 批次顶高。
	webTotal, err := o.Count("web")
	if err != nil {
		t.Fatal(err)
	}
	if webTotal != 90 {
		t.Errorf("source=web 应为 90（不是 120），got %d", webTotal)
	}
	cron, err := o.List("cron", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(cron) != 30 {
		t.Errorf("source=cron 应 30 条，got %d", len(cron))
	}
	cronTotal, err := o.Count("cron")
	if err != nil {
		t.Fatal(err)
	}
	if cronTotal != int64(len(cron)) {
		t.Errorf("cron 一次拿全时 total 应等于清单长度，got %d vs %d", cronTotal, len(cron))
	}
	if cronTotal > int64(len(cron)) {
		t.Error("一次拿全时截断判断必须为假（恒真是另一种说谎）")
	}

	// 空筛选集：不是截断，也不报错。
	none, err := o.Count("nope")
	if err != nil {
		t.Fatal(err)
	}
	if none != 0 {
		t.Errorf("不存在的 source 总数应为 0，got %d", none)
	}
}
