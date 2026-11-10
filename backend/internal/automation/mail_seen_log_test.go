// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package automation

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// TestSaveSeenIDsLogsWatermarkLoss 私信「已见水位」写失败必须留痕。
// 这条 Set 从前是 `_ = cfgOp.Set("mail:seen:"+siteName, …)`：写不进去时下一轮读到的还是旧水位，
// 于是**同一批邮件被当成新邮件反复提醒**（收件箱与外发渠道都会重复刷屏），而系统里没有任何一行
// 日志说明「水位没保存成功」。这里用真库真 oper，只把表删掉来造出确定性的写失败。
func TestSaveSeenIDsLogsWatermarkLoss(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"_seen?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	cfgOp := oper.NewSystemConfigOper(db)

	// 正控：表在的时候写得进去，不该出现失败日志。
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	saveSeenIDs(cfgOp, "hdchina", map[uint64]struct{}{11: {}, 22: {}})
	if strings.Contains(buf.String(), "私信已见水位保存失败") {
		t.Fatalf("正常保存不该报失败, got %q", buf.String())
	}

	// 缺陷形状：水位写不进去。
	if err := db.Migrator().DropTable(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	saveSeenIDs(cfgOp, "hdchina", map[uint64]struct{}{33: {}})
	logged := buf.String()
	for _, want := range []string{"私信已见水位保存失败", "重复提醒", "hdchina"} {
		if !strings.Contains(logged, want) {
			t.Errorf("日志里要点名 %q, got %q", want, logged)
		}
	}
}
