package db

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// oldSchema 是迁移前落库的样子：唯一索引 uk_cross_seed 只有 (info_hash, candidate_site)。
// 手写 SQL 而不是 AutoMigrate，是为了真的把**旧**索引留在库里。
const oldSchema = `
CREATE TABLE cross_seed_tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	downloader_id INTEGER, site_id INTEGER, info_hash TEXT, source_site TEXT, source_name TEXT,
	candidate_site TEXT, candidate_url TEXT, save_path TEXT, status TEXT, match_score INTEGER,
	created_at DATETIME, resolved_at DATETIME
);
CREATE UNIQUE INDEX uk_cross_seed ON cross_seed_tasks(info_hash, candidate_site);
INSERT INTO cross_seed_tasks (downloader_id, info_hash, candidate_site, save_path, status)
	VALUES (1, 'H1', 'ourbits', '/nas1/seed', 'pending');
`

func indexCols(t *testing.T, gdb *gorm.DB, index string) []string {
	t.Helper()
	var cols []struct {
		Name string `gorm:"column:name"`
	}
	// pragma 表值函数的索引名不当参数传（SQLite 版本差异），这里 index 恒为代码内常量。
	if err := gdb.Raw(`SELECT name FROM pragma_index_info('` + index + `') ORDER BY seqno`).Scan(&cols).Error; err != nil {
		t.Fatalf("读索引 %s: %v", index, err)
	}
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		out = append(out, c.Name)
	}
	return out
}

// TestCrossSeedUniqueIndexMigration 老库跑完生产那套启动流程（AutoMigrate → RunMigrations）后，
// 唯一键必须是 (downloader_id, info_hash, candidate_site)，旧索引必须摘掉。
// 否则代码改了 Where，两台下载器做同一颗种子时第二台的 Create 仍撞旧索引，
// 而链层把冲突当「并发兜底」静默跳过 ⇒ 屏幕上照旧是「这台没有候选」。
func TestCrossSeedUniqueIndexMigration(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open(t.TempDir()+"/old.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := gdb.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := gdb.Exec(oldSchema).Error; err != nil {
		t.Fatalf("造旧库: %v", err)
	}
	// 夹具正控：旧约束下换一台下载器插同 (hash, site) 必须失败——不然下面那条「迁移后才通」是假绿。
	if e := gdb.Exec(`INSERT INTO cross_seed_tasks (downloader_id, info_hash, candidate_site, save_path, status)
		VALUES (2, 'H1', 'ourbits', '/nas2/seed', 'pending')`).Error; e == nil {
		t.Fatal("旧库上第二台竟然插入成功——夹具没复现出旧索引")
	}

	// 现网顺序：db.go:77 AutoMigrate 先建缺失的索引，:102 RunMigrations 再摘旧索引。
	if err := gdb.AutoMigrate(&models.CrossSeedTask{}, &models.Subscription{}, &models.SystemConfig{}, &models.Site{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if err := RunMigrations(gdb); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	if gdb.Migrator().HasIndex(&models.CrossSeedTask{}, "uk_cross_seed") {
		t.Error("旧唯一索引 uk_cross_seed 仍在：它会继续挡住第二台的候选")
	}
	got := indexCols(t, gdb, "uk_cross_seed_dl")
	want := map[string]bool{"downloader_id": false, "info_hash": false, "candidate_site": false}
	for _, c := range got {
		if _, ok := want[c]; ok {
			want[c] = true
		}
	}
	for c, seen := range want {
		if !seen {
			t.Errorf("新唯一索引应含列 %s，got %v", c, got)
		}
	}

	// 用户可见的那条链路：第二台能建自己的候选……
	if e := gdb.Exec(`INSERT INTO cross_seed_tasks (downloader_id, info_hash, candidate_site, save_path, status)
		VALUES (2, 'H1', 'ourbits', '/nas2/seed', 'pending')`).Error; e != nil {
		t.Fatalf("迁移后第二台仍被挡住: %v", e)
	}
	// ……但同一台的重复扫描仍必须撞唯一键（放宽成「无去重」是另一种坏）。
	if e := gdb.Exec(`INSERT INTO cross_seed_tasks (downloader_id, info_hash, candidate_site, save_path, status)
		VALUES (2, 'H1', 'ourbits', '/nas2/seed', 'pending')`).Error; e == nil {
		t.Error("同下载器同 hash 同候选站应仍被唯一索引挡住")
	}

	// 正控：老行不许被迁移弄丢。
	var n int64
	if err := gdb.Raw(`SELECT COUNT(*) FROM cross_seed_tasks`).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("应有 2 行（每台一条，老行保留），got %d", n)
	}

	// 幂等：再跑一次不许报错，也不许重复登记。
	if err := RunMigrations(gdb); err != nil {
		t.Fatalf("RunMigrations 第二次: %v", err)
	}
	var applied int64
	if err := gdb.Raw(`SELECT COUNT(*) FROM schema_migrations WHERE version = 5`).Scan(&applied).Error; err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Errorf("version 5 应只记录一次，got %d", applied)
	}
}

// TestCrossSeedStaleIndexIsDropped 承重证明：把「旧唯一键仍躺在库里」这个形态直接造出来
// （实测本仓的 SQLite + GORM 会让 AutoMigrate 自己回收多余索引，别的驱动不保证；而现网
// 也可能是用户手工建过同名索引），此时迁移必须把它摘掉。
// 这条用例是专门给「迁移函数改成空操作」准备的负控靶：没有它，摘索引那几行不承重。
func TestCrossSeedStaleIndexIsDropped(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open(t.TempDir()+"/stale.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := gdb.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := gdb.AutoMigrate(&models.CrossSeedTask{}, &models.Subscription{}, &models.SystemConfig{}, &models.Site{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if err := gdb.Exec(`CREATE UNIQUE INDEX uk_cross_seed ON cross_seed_tasks(info_hash, candidate_site)`).Error; err != nil {
		t.Fatalf("造残留旧索引: %v", err)
	}
	if err := gdb.Exec(`INSERT INTO cross_seed_tasks (downloader_id, info_hash, candidate_site, save_path, status)
		VALUES (1, 'H1', 'ourbits', '/nas1/seed', 'pending')`).Error; err != nil {
		t.Fatalf("造首台候选: %v", err)
	}
	// 夹具正控：残留索引下第二台必须插不进，否则这条用例证明不了任何事。
	if e := gdb.Exec(`INSERT INTO cross_seed_tasks (downloader_id, info_hash, candidate_site, save_path, status)
		VALUES (2, 'H1', 'ourbits', '/nas2/seed', 'pending')`).Error; e == nil {
		t.Fatal("残留旧索引竟然没挡住第二台——夹具无效")
	}

	if err := RunMigrations(gdb); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if gdb.Migrator().HasIndex(&models.CrossSeedTask{}, "uk_cross_seed") {
		t.Fatal("迁移没摘掉残留的旧唯一索引：第二台的 Create 会一直被挡，并被链层静默跳过")
	}
	if e := gdb.Exec(`INSERT INTO cross_seed_tasks (downloader_id, info_hash, candidate_site, save_path, status)
		VALUES (2, 'H1', 'ourbits', '/nas2/seed', 'pending')`).Error; e != nil {
		t.Fatalf("摘掉旧索引后第二台仍插不进: %v", e)
	}
}
