// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package notice_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAlertStateMachineEscalatesAndRecovers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/alert.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	store := oper.NewNoticeOper(db)
	svc := notice.NewService(store, nil, nil)
	ctx := context.Background()
	in := notice.Input{Source: notice.SourceSiteOffline, Key: "offline:1", Title: "站点 A 掉线", Level: "danger"}

	if err := svc.Notice(ctx, in, true); err != nil {
		t.Fatal(err)
	}
	// 当天重复触发应被去重
	if err := svc.Notice(ctx, in, true); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&models.Notice{}).Count(&count)
	if count != 1 {
		t.Fatalf("dedup expected 1 notice, got %d", count)
	}

	// 把首条告警改成 3 小时前 → 再触发应产生「持续」升级通知
	old := time.Now().Add(-3 * time.Hour)
	db.Model(&models.Notice{}).Where("source = ?", notice.SourceSiteOffline).Update("created_at", old)
	if err := svc.Notice(ctx, in, true); err != nil {
		t.Fatal(err)
	}
	var escalated models.Notice
	if err := db.Where("key LIKE ?", "%:persist:%").First(&escalated).Error; err != nil {
		t.Fatalf("escalation notice missing: %v", err)
	}
	if !strings.Contains(escalated.Title, "持续") {
		t.Fatalf("escalation title missing 持续: %s", escalated.Title)
	}

	// 恢复通知
	if err := svc.Recover(ctx, in); err != nil {
		t.Fatal(err)
	}
	var recovered models.Notice
	if err := db.Where("key = ?", "recover:offline:1").First(&recovered).Error; err != nil {
		t.Fatalf("recovery notice missing: %v", err)
	}
	if recovered.Level != "success" || !strings.Contains(recovered.Title, "已恢复") {
		t.Fatalf("bad recovery notice: %+v", recovered)
	}
}
