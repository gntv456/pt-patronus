// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// 「按条件改一行」的 GORM 语义是 err==nil + RowsAffected==0，
// 不判行数的话，对不存在的对象执行动作一律变成 200 成功。
// 这里钉住两处最要紧的：轮转密钥（会往外发明文密钥）、重置状态（用户以为卡住的状态被清掉了）。

func openGuardDB(t *testing.T, name string, models_ ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// Windows：句柄不关会让 TempDir 清理失败，表现为整条用例红。
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(models_...); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAgentResetSecretRejectsMissingAgent(t *testing.T) {
	db := openGuardDB(t, "agent.db", &models.Agent{})
	o := NewAgentOper(db)

	secret, err := o.ResetSecret(999999)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("轮转不存在的被控端必须回 ErrRecordNotFound（供接口判 404），got %v", err)
	}
	// 关键：失败时绝不能把明文密钥交出去——否则 404 响应里还带一把可用的凭据。
	if secret != "" {
		t.Errorf("失败时不得返回明文密钥，got %q…", secret)
	}

	// 正向对照：真实存在的被控端照样能轮转出新密钥。
	if err := db.Create(&models.Agent{ID: 3, Name: "nas-agent"}).Error; err != nil {
		t.Fatal(err)
	}
	secret2, err := o.ResetSecret(3)
	if err != nil {
		t.Fatalf("已存在的被控端轮转不应失败：%v", err)
	}
	if secret2 == "" {
		t.Error("轮转应回一条明文密钥供用户复制")
	}
}

func TestAlertAndNoticeTouchGuards(t *testing.T) {
	db := openGuardDB(t, "alert.db", &models.MonitorAlert{}, &models.Notice{})

	cases := []struct {
		name string
		err  error
	}{
		{"告警标已读", NewMonitorOper(db, security.NewCrypto("guard-test-seed")).MarkAlertRead(999997)},
		{"告警确认", NewMonitorOper(db, security.NewCrypto("guard-test-seed")).AckAlert(999997, "admin")},
		{"告警静默", NewMonitorOper(db, security.NewCrypto("guard-test-seed")).SilenceAlert(999997, time.Now().Add(time.Hour), "admin")},
		{"通知标已读", NewNoticeOper(db).MarkRead(999997)},
	}
	for _, tc := range cases {
		if !errors.Is(tc.err, gorm.ErrRecordNotFound) {
			t.Errorf("%s：对不存在的记录必须回 ErrRecordNotFound（供接口判 404），got %v", tc.name, tc.err)
		}
	}

	// 正向对照：真实存在的告警/通知照旧能标记，且反复标记同一行也算命中。
	if err := db.Create(&models.MonitorAlert{ID: 9}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Notice{ID: 9}).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewMonitorOper(db, security.NewCrypto("guard-test-seed")).AckAlert(9, "admin"); err != nil {
		t.Fatalf("已存在的告警确认不应失败：%v", err)
	}
	if err := NewMonitorOper(db, security.NewCrypto("guard-test-seed")).AckAlert(9, "admin2"); err != nil {
		t.Fatalf("重复确认（值会变）不应被判成记录不存在：%v", err)
	}
	if err := NewNoticeOper(db).MarkRead(9); err != nil {
		t.Fatalf("已存在的通知标已读不应失败：%v", err)
	}
	if err := NewNoticeOper(db).MarkRead(9); err != nil {
		t.Fatalf("重复标已读（幂等）不应被判成记录不存在：%v", err)
	}
}

func TestWorkflowResetStateRejectsMissingRow(t *testing.T) {
	db := openGuardDB(t, "wf.db", &models.Workflow{})
	o := NewWorkflowOper(db)

	if err := o.ResetState(999998); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("重置不存在的工作流必须回 ErrRecordNotFound，got %v", err)
	}
	if err := db.Create(&models.Workflow{ID: 4, Name: "wf4", State: "running"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := o.ResetState(4); err != nil {
		t.Fatalf("已存在的工作流重置不应失败：%v", err)
	}
	var got models.Workflow
	if err := db.First(&got, 4).Error; err != nil {
		t.Fatal(err)
	}
	if got.State != "idle" {
		t.Errorf("重置要把 state 归位，got %q", got.State)
	}
}
