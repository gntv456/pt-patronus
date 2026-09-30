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

	"github.com/hxpt/ptpatronus/internal/site"
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
	{
		Version: 2,
		Name:    "brushflow_seed_unique_task_source_v1",
		Fn:      migrateBrushFlowSeedUniqueV1,
	},
	{
		Version: 3,
		Name:    "site_dup_domain_unique_v1",
		Fn:      migrateSiteDupDomainUniqueV1,
	},
	{
		Version: 4,
		Name:    "site_registered_domain_backfill_v1",
		Fn:      migrateSiteRegisteredDomainBackfillV1,
	},
}

// migrateSiteDupDomainUniqueV1 站点注册域唯一索引（四轮 P2 修复：Create 的
// check-then-act 去重在 CookieCloud 自动建站 / 手工添加 / 备份导入并发时可同时
// 通过检查 → 同注册域双站落库 → 仪表盘聚合双计。注册域 eTLD+1 判定在应用层
//（dupDomain），DB 侧以 site_registered_domain 存储列 + 唯一索引兜底）。
// 旧行回填：按 base_url 计算注册域，同域多条保留 id 最小的一条。
func migrateSiteDupDomainUniqueV1(tx *gorm.DB) error {
	// 1) 加存储列（幂等：已存在时报错忽略）。
	if err := tx.Exec(`ALTER TABLE sites ADD COLUMN registered_domain TEXT NOT NULL DEFAULT ''`).Error; err != nil {
		slog.Warn("sites add registered_domain column skip", "err", err)
	}
	// 2) 历史空串行全部置 NULL（唯一索引不约束 NULL；eTLD+1 精确回填在 v4 应用层做）。
	if err := tx.Exec(`UPDATE sites SET registered_domain = NULL WHERE registered_domain = ''`).Error; err != nil {
		slog.Warn("sites empty domain null-out skip", "err", err)
	}
	// 3) 去重 + 唯一索引。
	if err := tx.Exec(`DELETE FROM sites WHERE registered_domain != '' AND id NOT IN (
		SELECT MIN(id) FROM sites WHERE registered_domain != '' GROUP BY registered_domain
	)`).Error; err != nil {
		slog.Warn("sites dup dedup skip", "err", err)
	}
	if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uk_sites_registered_domain
		ON sites(registered_domain)`).Error; err != nil {
		slog.Warn("sites registered_domain unique index create", "err", err)
	}
	return nil
}

// migrateBrushFlowSeedUniqueV1 刷流种子历史 (task_id, source_key) 唯一索引：
// 旧库可能已有重复行（并发窗口产生），先按保留最旧一条去重，再建唯一索引。
// AutoMigrate 只做加列/建表，约束语义变更走版本化迁移；新库 AutoMigrate 已带
// uniqueIndex 标签，CREATE UNIQUE INDEX IF NOT EXISTS 幂等不冲突。
func migrateBrushFlowSeedUniqueV1(tx *gorm.DB) error {
	// 1) 去重：同 (task_id, source_key) 保留 id 最小的一条。
	if err := tx.Exec(`DELETE FROM brush_flow_seeds WHERE id NOT IN (
		SELECT MIN(id) FROM brush_flow_seeds GROUP BY task_id, source_key
	)`).Error; err != nil {
		// 表尚不存在（新库 AutoMigrate 后必存在）或方言差异：跳过，唯一索引兜底新数据。
		slog.Warn("brushflow seed dedup skip", "err", err)
	}
	// 2) 建唯一索引（IF NOT EXISTS 幂等；MySQL<8.0 无该子句时报重复名视为已存在）。
	if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uk_brush_seed_task_source
		ON brush_flow_seeds(task_id, source_key)`).Error; err != nil {
		slog.Warn("brushflow seed unique index create", "err", err)
	}
	return nil
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


// migrateSiteRegisteredDomainBackfillV1 用应用层 eTLD+1（publicsuffix）回填
// sites.registered_domain：SQL 端做不了 eTLD+1 判定。同注册域多行（历史并发窗口
// 产生的重复站）保留 id 最小的一条，其余删除——与应用层 Create 去重口径一致。
func migrateSiteRegisteredDomainBackfillV1(tx *gorm.DB) error {
	var rows []struct {
		ID      uint
		BaseURL string
	}
	if err := tx.Raw("SELECT id, base_url FROM sites WHERE registered_domain IS NULL OR registered_domain = ''").Scan(&rows).Error; err != nil {
		slog.Warn("sites backfill select skip", "err", err)
		return nil
	}
	best := map[string]uint{} // domain -> 保留的最小 id
	for _, r := range rows {
		dom := site.RegisteredDomain(r.BaseURL)
		if dom == "" {
			continue // 解析不出域名的行不参与约束（保持 NULL）
		}
		if prev, ok := best[dom]; !ok || r.ID < prev {
			best[dom] = r.ID
		}
	}
	for _, r := range rows {
		dom := site.RegisteredDomain(r.BaseURL)
		if dom == "" {
			continue
		}
		if keep, ok := best[dom]; ok && keep == r.ID {
			if err := tx.Exec("UPDATE sites SET registered_domain = ? WHERE id = ?", dom, r.ID).Error; err != nil {
				return err
			}
			continue
		}
		// 重复站：删除较大 id 的行（与应用层去重一致：同域只留最先建的）。
		if err := tx.Exec("DELETE FROM sites WHERE id = ?", r.ID).Error; err != nil {
			return err
		}
	}
	return nil
}
