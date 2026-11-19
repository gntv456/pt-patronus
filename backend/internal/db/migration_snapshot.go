// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	preMigrationKeep = 5
	preMigrationDir  = "backups" + string(os.PathSeparator) + "pre-migration"
)

// snapshotBeforeMigration 在 AutoMigrate/RunMigrations 前为 SQLite 做一致性物理快照。
// 借鉴 MoviePilot V3「迁移前自动备份」：大版本升级或迁移失败可回滚，
// 避免 data 目录一旦损坏就丢失全部站点 Cookie / 凭据。
func snapshotBeforeMigration(gdb *gorm.DB, dataDir, dbPath string) error {
	if strings.TrimSpace(dbPath) == "" {
		return nil
	}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil // 新库首启，无数据可备
		}
		return fmt.Errorf("stat db for snapshot: %w", err)
	}
	// 全新空库（gorm.Open 已建文件但无任何表）无需快照。
	var cnt int64
	if err := gdb.Raw("SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&cnt).Error; err == nil && cnt == 0 {
		return nil
	}
	dir := filepath.Join(dataDir, preMigrationDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("mkdir snapshot dir: %w", err)
	}
	name := fmt.Sprintf("pre-migration-%s.db", time.Now().UTC().Format("20060102T150405Z"))
	target := filepath.Join(dir, name)
	// VACUUM INTO 生成事务一致性快照，不锁生产库；目标不接受绑定参数，转义单引号后拼字面量。
	escaped := strings.ReplaceAll(target, "'", "''")
	if err := gdb.Exec("VACUUM INTO '" + escaped + "'").Error; err != nil {
		return fmt.Errorf("vacuum into snapshot: %w", err)
	}
	return trimSnapshots(dir)
}

// trimSnapshots 仅保留最近若干份快照（命名含 UTC 时间戳，字典序即时间序）。
func trimSnapshots(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "pre-migration-") && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > preMigrationKeep {
		old := filepath.Join(dir, names[0])
		if err := os.Remove(old); err != nil {
			return fmt.Errorf("trim old snapshot %s: %w", old, err)
		}
		names = names[1:]
	}
	return nil
}
