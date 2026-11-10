// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 外部测试包：oper 依赖链上含 notice（oper→monitor→notice），包内测试无法反向引入 oper。
package notice_test

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"gorm.io/gorm"
)

// TestOneOffNoticesNotDroppedByUniqueIndex：空 Key 的一次性通知（reseed_done/backup_done 等）
// 每条都必须落库。缺陷复现：唯一索引 uk_notice_src_key(source,key) 把 (source,"") 视为同一键，
// NoticeOper.Create 的 OnConflict{DoNothing} 从第二条起静默丢弃（且不返回错误）——
// 用户通知中心永远只见第一条「辅种完成」。
func TestOneOffNoticesNotDroppedByUniqueIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/notice.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, e := db.DB(); e == nil {
			_ = s.Close()
		}
	})

	svc := notice.NewService(oper.NewNoticeOper(db), nil, nil)
	for _, title := range []string{"辅种完成：A", "辅种完成：B"} {
		if err := svc.Notice(context.Background(), notice.Input{
			Source: notice.SourceReseedDone, Title: title, Level: "success",
		}, false); err != nil {
			t.Fatalf("Notice(%s): %v", title, err)
		}
	}
	var n int64
	if err := db.Model(&models.Notice{}).Where("source = ?", notice.SourceReseedDone).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("空 Key 一次性通知必须每条都落库（got %d 条，唯一索引静默吞掉 %d 条）", n, 2-n)
	}

	// 正控：带 Key + dedup=true 当天去重仍然生效（修复合成的唯一键不得破坏周期事件去重）。
	for i := 0; i < 2; i++ {
		if err := svc.Notice(context.Background(), notice.Input{
			Source: notice.SourceSiteAnniv, Key: "anniv:2026-10-10", Title: "今日站庆",
		}, true); err != nil {
			t.Fatalf("Notice dedup: %v", err)
		}
	}
	var m int64
	if err := db.Model(&models.Notice{}).Where("source = ?", notice.SourceSiteAnniv).Count(&m).Error; err != nil {
		t.Fatal(err)
	}
	if m != 1 {
		t.Errorf("带 Key 周期事件当天二次发送应被去重（got %d 条）", m)
	}
}
