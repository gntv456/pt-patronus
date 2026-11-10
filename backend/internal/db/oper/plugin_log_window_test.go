// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// TestPluginLogWindowAccount 插件日志的单页账面。
// 旧写法 `limit > 500` 被折回 100：要 9999 行的人只拿到 100 行，响应还长得跟「拿全了」一样。
func TestPluginLogWindowAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/pluglog.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.PluginLog{}); err != nil {
		t.Fatal(err)
	}
	o := NewPluginOper(db)
	add := func(pid string, n int) {
		for i := 0; i < n; i++ {
			if err := db.Create(&models.PluginLog{PluginID: pid}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	add("a", 130)
	add("b", 20)

	if got := NormalizePluginLogLimit(0); got != PluginLogListDefault {
		t.Errorf("留空应取默认 %d，got %d", PluginLogListDefault, got)
	}
	if got := NormalizePluginLogLimit(9999); got != PluginLogListLimit {
		t.Errorf("超上限应取上限 %d（不是退回默认 %d），got %d", PluginLogListLimit, PluginLogListDefault, got)
	}

	first, err := o.ListLogs("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != PluginLogListDefault {
		t.Errorf("首页应被窗口夹在 %d 条，got %d", PluginLogListDefault, len(first))
	}
	all, err := o.CountLogs("")
	if err != nil {
		t.Fatal(err)
	}
	if all != 150 {
		t.Errorf("总数应为 150，got %d", all)
	}
	if !(all > int64(len(first))) {
		t.Error("一页装不下时截断判断必须为真，否则被挤掉的老日志永远不上账")
	}

	big, err := o.ListLogs("", 9999)
	if err != nil {
		t.Fatal(err)
	}
	if len(big) != 150 {
		t.Errorf("limit=9999 应一次拿满 150 条（上限 500 没挡住），got %d", len(big))
	}
	if !(int64(len(big)) >= all) {
		t.Error("一次拿全时截断判断必须为假（恒真是另一种说谎）")
	}

	// 真库层：Count 与 List 同按 plugin_id 过滤，否则插件 A 的账面会被 B 的日志顶高。
	ca, err := o.CountLogs("a")
	if err != nil {
		t.Fatal(err)
	}
	if ca != 130 {
		t.Errorf("plugin a 总数应为 130（不是 150），got %d", ca)
	}
	la, err := o.ListLogs("a", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(la) != 100 {
		t.Errorf("plugin a 首页应 100 条，got %d", len(la))
	}
}
