// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// TestManagerLogFallsBackToProcessLog Manager.log 是全仓插件事件唯一的落库出口
// （插件页日志面板、#210 的设置读不出、#213 的映射刷新失败、#227 的卸载状态失败都只往这里写）。
// 从前 `_ = m.op.CreateLog(...)`：写失败时面板里什么都没有，连一行进程日志也不留，
// 于是「插件出过事」与「插件什么都没干」在任何一个界面上都长得一样。
func TestManagerLogFallsBackToProcessLog(t *testing.T) {
	m, db, _ := newZombieManager(t)
	if err := db.AutoMigrate(&models.PluginLog{}); err != nil {
		t.Fatal(err)
	}
	table := db.NamingStrategy.TableName("PluginLog")

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	// 正控：写得进插件日志表时，不该往进程日志里灌东西。
	m.log("demo", logError, "run.fail", "正常一次失败记录", nil)
	if strings.Contains(buf.String(), "插件日志落库失败") {
		t.Fatalf("落库成功不该有兜底日志, got %q", buf.String())
	}
	var cnt int64
	if err := db.Model(&models.PluginLog{}).Where("message = ?", "正常一次失败记录").Count(&cnt).Error; err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("正控前提不成立：插件日志表里应有 1 行, got %d", cnt)
	}

	// 缺陷形状：插件日志表写不进（磁盘满/表被锁/迁移没跑）。
	if err := db.Exec(`CREATE TRIGGER block_plugin_log BEFORE INSERT ON ` + table +
		` BEGIN SELECT RAISE(ABORT, '模拟插件日志写入失败'); END`).Error; err != nil {
		t.Fatalf("装触发器失败：%v", err)
	}
	buf.Reset()
	m.log("demo", logError, "run.fail", "面板里本应看到这条", map[string]any{"k": "v"})
	logged := buf.String()
	for _, want := range []string{"插件日志落库失败", "demo", "run.fail", "面板里本应看到这条", "模拟插件日志写入失败"} {
		if !strings.Contains(logged, want) {
			t.Errorf("兜底日志里要点名 %q, got %q", want, logged)
		}
	}
	// 原始级别要作为属性带出来（这行日志自己的级别是 Warn，别搞混——第一次断言我就写错过一次）
	if !strings.Contains(logged, "level=error") {
		t.Errorf("兜底至少要带原始级别，否则 error 与 info 分不开, got %q", logged)
	}
}
