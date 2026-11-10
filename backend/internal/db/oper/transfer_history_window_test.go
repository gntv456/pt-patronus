// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// TestTransferHistoryWindowAccount 整理历史的取数账面。
// 这条列表单页封顶 500、默认 200：只回被切过的一页时，界面就把「最近 200 次整理」
// 画成这个下载器的全部整理记录，被挤掉的失败记录在界面上不存在，
// 「已经没有待处理了」这类结论也跟着失真。
func TestTransferHistoryWindowAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/tfhist.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.TransferHistory{}); err != nil {
		t.Fatal(err)
	}
	o := NewTransferHistoryOper(db)
	add := func(status string, n int) {
		for i := 0; i < n; i++ {
			if err := db.Create(&models.TransferHistory{Status: status}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	add("failed", 260)
	add("success", 30)

	first, err := o.List("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 200 {
		t.Errorf("留空应被默认窗口夹在 200 条，got %d", len(first))
	}
	all, err := o.CountByStatus("")
	if err != nil {
		t.Fatal(err)
	}
	if all != 290 {
		t.Errorf("总数应为 290，got %d", all)
	}
	if !(all > int64(len(first))) {
		t.Error("一页装不下时截断判断必须为真，否则被挤掉的失败记录永远不上账")
	}

	big, err := o.List("", 9999)
	if err != nil {
		t.Fatal(err)
	}
	if len(big) != 290 {
		t.Errorf("limit=9999 应一次拿满 290 条（上限 500 没挡住），got %d", len(big))
	}

	// 真库层：Count 与 List 同按 status 过滤，否则 failed 页的账面会被 success 顶高。
	failed, err := o.CountByStatus("failed")
	if err != nil {
		t.Fatal(err)
	}
	if failed != 260 {
		t.Errorf("status=failed 总数应为 260（不是 290），got %d", failed)
	}
	fs, err := o.List("failed", 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 260 {
		t.Errorf("status=failed 清单应 260 条，got %d", len(fs))
	}

	// 超限不许折回默认值：要 400 条就该拿 400 条上限内的量。
	capped, err := o.List("", 501)
	if err != nil {
		t.Fatal(err)
	}
	if len(capped) != 290 {
		t.Errorf("limit=501 应取上限 500 内的全部 290 条，got %d", len(capped))
	}
}
