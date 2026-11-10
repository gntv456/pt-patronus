// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package db

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// TestMigrationV3FreshDBNoSpuriousWarn 钉住新库首启体验：db.Open 的 AutoMigrate 已按模型标签
// 建好 registered_domain 列，v3 不能无条件 ALTER——修复前每个全新安装都打一条
// 「duplicate column name: registered_domain」WARN，而升级说明让用户「看日志 WARN 计数」
// 判断重复站点去重情况，首启噪声会淹掉这条真信号。
func TestMigrationV3FreshDBNoSpuriousWarn(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(&models.Site{}, &models.Subscription{}, &models.BrushFlowSeed{},
		&models.SystemConfig{}); err != nil {
		t.Fatalf("AutoMigrate fresh schema: %v", err)
	}

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	if err := RunMigrations(gdb); err != nil {
		t.Fatalf("RunMigrations on fresh schema: %v", err)
	}
	logged := buf.String()
	if strings.Contains(logged, "level=WARN") || strings.Contains(logged, "duplicate column") {
		t.Errorf("全新库首启不该出现迁移 WARN：%s", logged)
	}
	// 正控：迁移确实跑了（v3/v4 登记 + 唯一索引在位），不是「什么都没做」导致的干净日志
	var applied int64
	gdb.Table("schema_migrations").Where("version IN (3,4)").Count(&applied)
	if applied != 2 {
		t.Errorf("schema_migrations v3/v4 rows = %d, want 2", applied)
	}
	if !gdb.Migrator().HasIndex(&models.Site{}, "uk_sites_registered_domain") {
		t.Error("registered_domain 唯一索引缺失")
	}
}

// TestMigrationV3V4OldDBUpgrade 模拟老库升级：手工造旧 schema sites 表（无 registered_domain），
// 塞两行同注册域 + 一行不同域，跑 RunMigrations 验证 v3 加列/去重 + v4 eTLD+1 回填。
func TestMigrationV3V4OldDBUpgrade(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 旧 schema：无 registered_domain 列（v3 的 ALTER 负责加）。
	if err := gdb.Exec(`CREATE TABLE sites (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT, platform TEXT, definition TEXT, base_url TEXT,
		cookie_enc TEXT, api_key_enc TEXT, passkey_enc TEXT,
		username TEXT, password_enc TEXT,
		is_active NUMERIC DEFAULT 1, limit_interval INTEGER DEFAULT 0,
		priority INTEGER DEFAULT 0,
		created_at DATETIME, updated_at DATETIME)`).Error; err != nil {
		t.Fatal(err)
	}
	// 同注册域两行（并发窗口产生）：id 1 保留、id 2 应被 v4 删除。
	seed := []struct {
		id   int
		name string
		url  string
	}{
		{1, "HDsky", "https://hdsky.me"},
		{2, "HDsky-dup", "https://www.hdsky.me"},
		{3, "PTer", "https://pter.club"},
	}
	for _, s := range seed {
		if err := gdb.Exec(`INSERT INTO sites (id, name, base_url, is_active, created_at, updated_at)
			VALUES (?, ?, ?, 1, datetime('now'), datetime('now'))`, s.id, s.name, s.url).Error; err != nil {
			t.Fatal(err)
		}
	}
	// v1/v2 与本测试无关（subscriptions/brush_flow_seeds 表不在旧 schema 里），
	// 预登记为已应用，聚焦 v3/v4 的升级路径。
	if err := gdb.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT, applied_at DATETIME)`).Error; err != nil {
		t.Fatal(err)
	}
	for _, vn := range [][2]any{{1, "pre"}, {2, "pre"}} {
		if err := gdb.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, vn[0], vn[1]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := RunMigrations(gdb); err != nil {
		t.Fatalf("RunMigrations on legacy schema: %v", err)
	}
	// v3/v4 应已登记。
	var cnt int64
	gdb.Table("schema_migrations").Where("version IN (3,4)").Count(&cnt)
	if cnt != 2 {
		t.Errorf("schema_migrations v3/v4 rows = %d, want 2", cnt)
	}
	type row struct {
		ID               int
		Name             string
		RegisteredDomain *string
	}
	var rows []row
	if err := gdb.Raw("SELECT id, name, registered_domain FROM sites ORDER BY id").Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows after dedup = %d, want 2 (dup removed)", len(rows))
	}
	if rows[0].ID != 1 || rows[0].RegisteredDomain == nil || *rows[0].RegisteredDomain != "hdsky.me" {
		t.Errorf("row1: %+v, want id=1 domain=hdsky.me", rows[0])
	}
	if rows[1].ID != 3 || rows[1].RegisteredDomain == nil || *rows[1].RegisteredDomain != "pter.club" {
		t.Errorf("row2: %+v, want id=3 domain=pter.club", rows[1])
	}
	// 唯一索引必须真的建成：修复前 ALTER 用 NOT NULL 建列 → 紧随的「历史空串行置 NULL」撞约束
	// 失败 → 全表停在 '' → CREATE UNIQUE INDEX 又撞 '' 重复 → 只 WARN 不报错，本迁移宣称的
	// 「DB 侧同域兜底」对升级用户实际没装上（并发建同域站仍会双计）。
	if !gdb.Migrator().HasIndex(&models.Site{}, "uk_sites_registered_domain") {
		t.Error("升级路径缺 registered_domain 唯一索引（同域并发建站不会被 DB 拦住）")
	}
}
