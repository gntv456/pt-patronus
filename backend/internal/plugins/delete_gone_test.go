// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// 分身实例删除与插件卸载的「不存在」分支。
// 旧行为两处都说成功：PluginMarketOper.DeleteInstance 不判 RowsAffected（GORM 删未命中
// 回 nil），Uninstall 对没装的插件走 os.RemoveAll（对不存在路径也回 nil）——
// 于是界面拿 200 弹「已删除 / 已卸载」，而用户多半对着陈旧的列表点了空行。
// 修法在 oper 之上：存在性由 Manager 查，oper 自己的幂等语义（market_charts_test.go 钉着）不动。
func newGoneManager(t *testing.T) (*Manager, *oper.PluginMarketOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/gone.db"), &gorm.Config{})
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
	mop := oper.NewPluginMarketOper(db)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetMarketOper(mop)
	return m, mop
}

func TestDeleteInstanceReportsMissing(t *testing.T) {
	m, mop := newGoneManager(t)

	if _, err := mop.CreateInstance("brush", "分身-A", "{}"); err != nil {
		t.Fatal(err)
	}
	// 正例：真有这条时删除照旧成功
	if err := m.DeletePluginInstance("brush", "分身-A"); err != nil {
		t.Fatalf("删除已存在的分身不应报错，got %v", err)
	}
	// 反例：再删同一条必须说「不存在」（旧行为是 nil → handler 200 {"deleted":key}）
	if err := m.DeletePluginInstance("brush", "分身-A"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("重复删除应报记录不存在，got %v", err)
	}
	// oper 层自己的幂等语义不许被顺手改掉（级联/重复清理依赖它）
	if err := mop.DeleteInstance("brush", "分身-A"); err != nil {
		t.Fatalf("oper DeleteInstance 应仍幂等，got %v", err)
	}
}

func TestUninstallReportsNotInstalled(t *testing.T) {
	m, _ := newGoneManager(t)
	root := t.TempDir()
	m.SetPluginRoot(root)

	// 三条正向证据都不成立（没加载、目录不在、状态行没有）→ 说「本机没装」。
	if err := m.Uninstall("ghost"); !errors.Is(err, ErrPluginNotInstalled) {
		t.Fatalf("卸载从未装过的插件应报未安装，got %v", err)
	}

	// 正例：目录在就照旧往下走，不许把真卸载拦成「未安装」。
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall("real"); errors.Is(err, ErrPluginNotInstalled) {
		t.Fatalf("插件目录存在却被判未安装：拦截条件过宽")
	}
}
