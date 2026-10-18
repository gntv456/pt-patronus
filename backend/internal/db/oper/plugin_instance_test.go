package oper

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func newPluginMarketDB(t *testing.T) (*gorm.DB, *PluginMarketOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/plugin_market.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.PluginInstance{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	return db, NewPluginMarketOper(db)
}

// TestCreateInstanceUniqueAndOrdering 基线：同名必须拒；新建排到末尾（max+1）。
func TestCreateInstanceUniqueAndOrdering(t *testing.T) {
	db, op := newPluginMarketDB(t)
	first, err := op.CreateInstance("p1", "default", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if first.SortOrder != 1 {
		t.Fatalf("首个实例排序位应为 1，got %d", first.SortOrder)
	}
	second, err := op.CreateInstance("p1", "alt", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if second.SortOrder != 2 {
		t.Fatalf("第二个应排到末尾（max+1=2），got %d", second.SortOrder)
	}
	if _, err := op.CreateInstance("p1", "default", `{}`); err == nil ||
		!strings.Contains(err.Error(), "已存在") {
		t.Fatalf("同名实例应被拒绝，got %v", err)
	}
	// 另一个插件的同名实例不受影响
	if _, err := op.CreateInstance("p2", "default", `{}`); err != nil {
		t.Fatalf("不同插件可同名实例: %v", err)
	}
	var n int64
	if err := db.Model(&models.PluginInstance{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("应有 3 个实例，got %d", n)
	}
}

// TestCreateInstanceAbortsWhenDedupeQueryFails 查重查询失败时必须中止创建。
// 回归：此前 Count 的错误被丢弃，n 停在 0 → 「实例名已存在」这道唯一的门形同不存在，
// 而 (plugin_id, instance_key) 在库里只是普通索引（idx_plugin_inst，非唯一），
// 没有任何东西兜住这条不变量：一旦落下同名实例，之后按 (plugin_id, instance_key)
// 取配置会随机命中其一（用户改的是 A，生效的是 B）。
func TestCreateInstanceAbortsWhenDedupeQueryFails(t *testing.T) {
	db, op := newPluginMarketDB(t)
	if _, err := op.CreateInstance("p1", "default", `{}`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&models.PluginInstance{}); err != nil {
		t.Fatal(err)
	}
	_, err := op.CreateInstance("p1", "second", `{}`)
	if err == nil {
		t.Fatal("表都读不到时不得声称创建成功")
	}
	if !strings.Contains(err.Error(), "查重失败") {
		t.Fatalf("应在查重这一步就中止（而不是继续往下撞到 Create），got %v", err)
	}
}
