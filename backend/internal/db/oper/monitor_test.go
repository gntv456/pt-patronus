// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// newMonitorDB 内存级测试库，迁移 MonitorTarget + MetricSnapshot + MonitorAlert 三张表。
func newMonitorDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/monitor.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.MonitorTarget{}, &models.MetricSnapshot{}, &models.MonitorAlert{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestMonitorDeleteCascadesSnapshotsAlerts 验证 MonitorOper.Delete 把 target 与其快照/告警
// 在单事务里全部删除（修复回归：原实现忽略 tx.Where(...).Delete 的错误，级联失败时会
// 残留 snapshot/alert 孤儿行且事务仍提交）。
//
// 复现路径：插入 target + 关联 snapshot/alert，调用 Delete，确认三表均无残留。
func TestMonitorDeleteCascadesSnapshotsAlerts(t *testing.T) {
	db := newMonitorDB(t)
	op := NewMonitorOper(db, security.NewCrypto("seed"))

	tgt, err := op.Create(TargetInput{
		Name: "host1", Type: "host", Protocol: "local", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	// 直接造关联行（绕过 RecordSnapshot，避免采集依赖）。
	if err := db.Create(&models.MetricSnapshot{TargetID: tgt.ID, SnapAt: db.NowFunc()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.MonitorAlert{TargetID: tgt.ID, Level: "info", Metric: "cpu"}).Error; err != nil {
		t.Fatal(err)
	}

	if err := op.Delete(tgt.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var nT, nS, nA int64
	db.Model(&models.MonitorTarget{}).Count(&nT)
	db.Model(&models.MetricSnapshot{}).Count(&nS)
	db.Model(&models.MonitorAlert{}).Count(&nA)
	if nT != 0 || nS != 0 || nA != 0 {
		t.Errorf("cascade delete left orphan rows: target=%d snapshot=%d alert=%d (all want 0)", nT, nS, nA)
	}
}

// TestMonitorDeleteNonexistentTargetSucceeds 删除不存在的 target 应不报错（幂等）。
func TestMonitorDeleteNonexistentTargetSucceeds(t *testing.T) {
	op := NewMonitorOper(newMonitorDB(t), security.NewCrypto("seed"))
	if err := op.Delete(9999); err != nil {
		t.Errorf("delete nonexistent target: %v", err)
	}
}
