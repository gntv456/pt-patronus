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

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// Migration 是一个版本化迁移步骤。
type Migration struct {
	Version int64  // 严格递增（追加时取 Max+1）
	Name    string // 人读名（日志/排障用）
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
	{
		Version: 5,
		Name:    "cross_seed_unique_per_downloader_v1",
		Fn:      migrateCrossSeedUniquePerDownloaderV1,
	},
	{
		Version: 6,
		Name:    "rss_non_positive_interval_floor_v1",
		Fn:      migrateRssNonPositiveIntervalV1,
	},
}

// migrateCrossSeedUniquePerDownloaderV1 把辅种候选的唯一键从
// (info_hash, candidate_site) 改成 (downloader_id, info_hash, candidate_site)。
//
// 旧键不含下载器 ⇒ 两台下载器做着同一颗种子时，第二台扫描撞上第一台的行就被当成
// 「进行中，去重」跳过 ⇒ 那台永远不出现候选；而自动辅种是**按下载器逐台**跑的
// （automation 里 ListAutoReseedSettings → for → Reseed(st.DownloaderID)），
// 用户看到的是「开关已开、这台却永远没有任务」。辅种要复用的是**本机**保存路径，
// 各台路径本来就不同，一条全局候选也存不下两套。
//
// 老行不改写：新键比旧键更宽（去掉一条全局唯一约束），既有数据不可能违反它。
//
// 实测（SQLite + 本仓 GORM）：AutoMigrate 自己就会摘掉模型里已不存在的 `uk_cross_seed`
// 并建出 `uk_cross_seed_dl`——所以现网升级的第一道修复其实是 schema 同步本身。
// 这条迁移是**方言兜底**：GORM 的 AutoMigrate 在别的驱动上不保证回收多余索引，
// 残留的旧唯一键会继续挡住第二台的 Create，而链层把冲突当「并发兜底」静默跳过
// ⇒ 屏幕上照旧是「这台没有候选」。见同文件 TestCrossSeedStaleIndexIsDropped。
func migrateCrossSeedUniquePerDownloaderV1(tx *gorm.DB) error {
	// 表都没有 = 全新库或从未用过辅种：没有旧索引可删，别留 WARN（同 v2/v3/v4 的宽容语义）。
	if !tx.Migrator().HasTable(&models.CrossSeedTask{}) {
		return nil
	}
	if !tx.Migrator().HasIndex(&models.CrossSeedTask{}, "uk_cross_seed") {
		return nil // 已经清理过（幂等）
	}
	if err := tx.Migrator().DropIndex(&models.CrossSeedTask{}, "uk_cross_seed"); err != nil {
		slog.Warn("cross seed old unique index drop", "err", err)
	}
	return nil
}

// migrateSiteDupDomainUniqueV1 站点注册域唯一索引（四轮 P2 修复：Create 的
// check-then-act 去重在 CookieCloud 自动建站 / 手工添加 / 备份导入并发时可同时
// 通过检查 → 同注册域双站落库 → 仪表盘聚合双计。注册域 eTLD+1 判定在应用层
// （dupDomain），DB 侧以 site_registered_domain 存储列 + 唯一索引兜底）。
// 旧行回填：按 base_url 计算注册域，同域多条保留 id 最小的一条。
func migrateSiteDupDomainUniqueV1(tx *gorm.DB) error {
	// 没有 sites 表时按 v2 的宽容语义跳过（迁移单测与部分库只建了自己关心的表），
	// 别把「表不存在」升级成启动失败。
	if !tx.Migrator().HasTable(&models.Site{}) {
		slog.Info("sites table absent, skip dup-domain unique migration")
		return nil
	}
	// 1) 加存储列——必须先探测再加：新库由 db.Open 的 AutoMigrate 按模型标签建列，
	//    无条件 ALTER 会让每个新库首启都刷一条「duplicate column name」WARN，
	//    而升级说明让用户「看日志 WARN 计数」判断去重情况，噪声会淹掉真信号。
	if !tx.Migrator().HasColumn(&models.Site{}, "RegisteredDomain") {
		// 列必须可空：模型侧是 *string，且下一步要把历史空串行置 NULL 绕过唯一约束。
		// 写成 NOT NULL 会让那条 UPDATE 直接撞约束（实测 SQLite "NOT NULL constraint failed"），
		// 于是所有历史行停留在 ''→ 建唯一索引又撞 '' 重复 → 索引静默缺失（只 WARN），
		// 本迁移宣称的「DB 侧同域去重兜底」对升级用户等于没装。
		if err := tx.Exec(`ALTER TABLE sites ADD COLUMN registered_domain TEXT DEFAULT ''`).Error; err != nil {
			return fmt.Errorf("sites add registered_domain column: %w", err)
		}
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
	removed := 0
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
		res := tx.Exec("DELETE FROM sites WHERE id = ?", r.ID)
		if res.Error != nil {
			return res.Error
		}
		removed += int(res.RowsAffected)
	}
	// 让用户可见丢了哪些站（五轮 P3：静默硬删不可接受，至少留下日志痕迹）。
	if removed > 0 {
		slog.Warn("site dedup: removed duplicate sites during migration", "count", removed,
			"note", "同注册域只保留最先创建的一条，被删站点名见历史库")
	}
	return nil
}

// migrateRssNonPositiveIntervalV1 把历史上写进库的 interval_min<=0 的 RSS 订阅一次修成 30 分钟。
// 出口层的归一此前只在 Create 做一次（Update 走同一个 applyRssReq 却没有它），于是编辑过的订阅
// 会存 0；抓取节奏有读侧兜底不会坏，但 Web Rss.vue 与移动 rss_page.dart 把这一行原样回显成
// 「间隔 0 分钟」——账面与生效不一致，这里对齐账面（0 行也照常提交，幂等）。
func migrateRssNonPositiveIntervalV1(tx *gorm.DB) error {
	// 迁移必须能在「还没有这张表」的老库/新库上跑（internal/db 的用例就是手工铺 legacy schema 后调
	// RunMigrations）：生产顺序是 AutoMigrate 建表再 RunMigrations，这里只做存在性兜底。
	if !tx.Migrator().HasTable(&models.RssSubscription{}) {
		return nil
	}
	res := tx.Model(&models.RssSubscription{}).Where("interval_min <= ?", 0).Update("interval_min", 30)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		slog.Info("rss interval_min backfilled to 30", "rows", res.RowsAffected)
	}
	return nil
}
