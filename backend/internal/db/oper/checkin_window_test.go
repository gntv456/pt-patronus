// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// TestCheckinRecordWindowAccount 签到历史的分页账面。
// 这条接口同时有单页上限和 offset：只回被切过的一页时，界面分不清
// 「已经翻到头」和「这页到此为止、后面还有」，失败的那几次可能整段看不见。
func TestCheckinRecordWindowAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/checkin.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.CheckinRecord{}); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	add := func(siteID uint, n int) {
		for i := 0; i < n; i++ {
			rec := &models.CheckinRecord{SiteID: siteID, CreatedAt: base}
			if err := db.Create(rec).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	op := NewCheckinRecordOper(db)
	add(1, 120)
	add(2, 7)

	if got := NormalizeCheckinLimit(0); got != CheckinListDefault {
		t.Errorf("留空应取默认 %d，got %d", CheckinListDefault, got)
	}
	if got := NormalizeCheckinLimit(99999); got != CheckinListLimit {
		t.Errorf("超上限应取上限 %d（不是退回默认 %d），got %d", CheckinListLimit, CheckinListDefault, got)
	}

	first, err := op.List(1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != CheckinListDefault {
		t.Errorf("首页应被窗口夹在 %d 条，got %d", CheckinListDefault, len(first))
	}
	total, err := op.Count(1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 120 {
		t.Errorf("site 1 总数应为 120，got %d", total)
	}
	if int64(0+len(first)) >= total {
		t.Error("首页装不下全部时按「offset+本页长度 < 总数」必须判为未翻完")
	}

	// 末页语义：offset 100 + 20 条 == 总数 ⇒ 翻到头了，不许再报「还有更多」。
	last, err := op.List(1, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(last) != 20 {
		t.Errorf("第二页应剩 20 条，got %d", len(last))
	}
	if int64(100+len(last)) < total {
		t.Error("已翻完却报未翻完：恒真是另一种说谎")
	}

	// 真库层：Count 与 List 同按 site_id 过滤，否则站 A 的历史会被站 B 的条数顶高。
	other, err := op.Count(2)
	if err != nil {
		t.Fatal(err)
	}
	if other != 7 {
		t.Errorf("site 2 总数应为 7（不是 127），got %d", other)
	}
	all, err := op.Count(0)
	if err != nil {
		t.Fatal(err)
	}
	if all != 127 {
		t.Errorf("siteID=0 应不过滤，总数 127，got %d", all)
	}
}
