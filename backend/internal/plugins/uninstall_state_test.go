package plugins

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func newZombieManager(t *testing.T) (*Manager, *gorm.DB, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/zombie.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.PluginInstance{}, &models.PluginState{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(root)
	return m, db, root
}

// TestUninstallFailsWhenStateWriteIsLost 卸载是「删目录 + 把状态行改成未安装」两步。
// 第二步从前是 `_ = m.op.SaveState(st)`：目录已经删了，状态行却还标着 installed=true/enabled=true，
// 函数照样回成功——面板刷新后插件「复活」成已安装且启用（指向一个不存在的目录），
// 而再调用它又报「插件未安装」，两份读数互相矛盾。现在这步失败必须让整个卸载失败。
func TestUninstallFailsWhenStateWriteIsLost(t *testing.T) {
	m, db, root := newZombieManager(t)

	if err := os.MkdirAll(filepath.Join(root, "zombie"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := &models.PluginState{ID: "zombie", Installed: true, Enabled: true}
	if err := db.Create(st).Error; err != nil {
		t.Fatal(err)
	}
	// 只挡 UPDATE：读状态行照常，写不进去——正是磁盘满/连接被断/库只读的形状。
	if err := db.Exec(`CREATE TRIGGER block_state_update BEFORE UPDATE ON plugin_states
BEGIN SELECT RAISE(ABORT, '模拟状态写入失败'); END`).Error; err != nil {
		t.Fatalf("装触发器失败：%v", err)
	}

	err := m.Uninstall("zombie")
	if err == nil {
		t.Fatal("状态行没写成却报卸载成功（旧行为）——面板会显示插件仍在安装中")
	}
	if !errors.Is(err, ErrPluginStateNotCleared) {
		t.Fatalf("应报 ErrPluginStateNotCleared 供 handler 分类, got %v", err)
	}
	for _, want := range []string{"zombie", "状态行", "已安装", "请重试卸载", "模拟状态写入失败"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误里要点名 %q, got %q", want, err.Error())
		}
	}

	// 目录这步确实已经做掉了（不可逆部分已完成，日志/错误里要能对得上）
	if _, e := os.Stat(filepath.Join(root, "zombie")); e == nil {
		t.Error("插件目录应已被删除")
	}

	// 重试必须能收尾：撤掉触发器后再卸载一次，不该被「本机没装」拦住，状态行也应落成为未安装。
	if err := db.Exec(`DROP TRIGGER block_state_update`).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall("zombie"); err != nil {
		t.Fatalf("重试卸载应成功（RemoveAll 对已不存在的目录回 nil），got %v", err)
	}
	var after models.PluginState
	if err := db.First(&after, "id = ?", "zombie").Error; err != nil {
		t.Fatal(err)
	}
	if after.Installed || after.Enabled {
		t.Errorf("重试后状态行应落成未安装/停用, got installed=%v enabled=%v", after.Installed, after.Enabled)
	}
}

// TestUninstallWithoutStateRowStaysOK 没有状态行 = 库里不可能留着假的「已安装」，
// 目录删掉即完成：这条幂等语义被既有测试（market-demo / external-shutdown-demo）钉着，
// 新加的失败分支不许把它一起拦掉。
func TestUninstallWithoutStateRowStaysOK(t *testing.T) {
	m, _, root := newZombieManager(t)
	if err := os.MkdirAll(filepath.Join(root, "dir-only"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall("dir-only"); err != nil {
		t.Fatalf("只有目录、没有状态行时应照常成功，got %v", err)
	}
	if _, e := os.Stat(filepath.Join(root, "dir-only")); !os.IsNotExist(e) {
		t.Error("目录应已删除")
	}
}
