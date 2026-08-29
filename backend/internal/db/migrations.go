// 版本化 schema 迁移 runner（数据审计/运维审计项：此前 AutoMigrate 单点 + 一次性
// 迁移散落）。
//
// 设计：
//   - AutoMigrate 仍负责「建表/加列」这类增量 DDL（GORM diff，幂等）；
//   - AutoMigrate 做不了/做不对的变更（数据回填、列改语义、破坏性重整）写进
//     Migrations 有序列表，由 RunMigrations 在启动时按序执行、恰好一次；
//   - 执行记录存 schema_migrations 表（版本号 + 幂等），事务内跑（SQLite/PG 均支持），
//     失败即启动失败——绝不带病运行半迁移状态。
//
// 新增迁移：在 Migrations 切片尾部追加 {Version, Name, Fn}；版本号必须严格递增
// （单测钉死），Fn 内只操作传入 tx，不要引用全局 db。
package db

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// Migration 是一个版本化迁移步骤。
type Migration struct {
	Version int64   // 严格递增（追加时取 Max+1）
	Name    string  // 人读名（日志/排障用）
	Fn      func(tx *gorm.DB) error
}

// Migrations 全部版本化迁移，按 Version 升序执行。
// 历史迁移 migrateSubscriptionNullableBooleans（原 SystemConfig key 标记）已收编为 0001，
// 并保留旧 key 的兼容判定——老库不会重复执行。
var Migrations = []Migration{
	{
		Version: 1,
		Name:    "subscription_nullable_booleans_v1",
		Fn:      migrateSubscriptionNullableBooleansV1,
	},
}

type schemaMigrationRow struct {
	Version int64  `gorm:"column:version"`
	Name    string `gorm:"column:name"`
}

func (schemaMigrationRow) TableName() string { return "schema_migrations" }

// RunMigrations 建表执行记录表后按序执行未应用的迁移。幂等：已应用的跳过。
func RunMigrations(db *gorm.DB) error {
	if err := db.AutoMigrate(&schemaMigrationRow{}); err != nil {
		return fmt.Errorf("migrate schema_migrations table: %w", err)
	}
	var applied []schemaMigrationRow
	if err := db.Find(&applied).Error; err != nil {
		return fmt.Errorf("load schema_migrations: %w", err)
	}
	done := make(map[int64]bool, len(applied))
	for _, r := range applied {
		done[r.Version] = true
	}
	for _, m := range Migrations {
		if done[m.Version] {
			continue
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := m.Fn(tx); err != nil {
				return err
			}
			return tx.Create(&schemaMigrationRow{Version: m.Version, Name: m.Name}).Error
		}); err != nil {
			return fmt.Errorf("migration %04d_%s: %w", m.Version, m.Name, err)
		}
		slog.Info("schema migration applied", "version", m.Version, "name", m.Name)
	}
	return nil
}

// migrateSubscriptionNullableBooleansV1 converts historical false values to the new
// "inherit global default" state. 兼容旧 SystemConfig key 标记：老库已执行过则跳过
// 数据回填（schema_migrations 记录照写，保证新框架幂等）。
func migrateSubscriptionNullableBooleansV1(tx *gorm.DB) error {
	var count int64
	if err := tx.Table("system_configs").
		Where("key = ?", "MigrationSubscriptionNullableBooleansV1").
		Count(&count).Error; err != nil {
		// 老库 system_configs 表一定存在（AutoMigrate 已跑）；查不到按未迁移处理
		count = 0
	}
	if count > 0 {
		return nil
	}
	for _, col := range []string{"enable_rating_guard", "best_version"} {
		if err := tx.Table("subscriptions").
			Where(col+" = ?", false).
			Update(col, nil).Error; err != nil {
			return fmt.Errorf("migrate subscriptions.%s nullable: %w", col, err)
		}
	}
	return nil
}
