// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"gorm.io/gorm"
)

// TestUninstallHandlerSurfacesStateWriteLoss #227 的出口层红证：链层报出「目录已删、状态行没写进去」
// 之后，HTTP 面必须把它当成服务端自己的失败（500 + 中文原因），而不是像从前那样
// 一律压成 400「请求参数有误，请检查后重试」——用户没有输错任何东西，重试输入也不会有用。
func TestUninstallHandlerSurfacesStateWriteLoss(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/uninstall_outlet.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.PluginState{}, &models.PluginInstance{}); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "zombie"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.PluginState{ID: "zombie", Installed: true, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER block_state_update BEFORE UPDATE ON plugin_states
BEGIN SELECT RAISE(ABORT, '模拟状态写入失败'); END`).Error; err != nil {
		t.Fatalf("装触发器失败：%v", err)
	}

	mgr := plugins.NewManager(db, event.New(), nil, nil, nil)
	mgr.SetPluginRoot(root)
	h := NewPluginHandler(mgr, nil)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/plugins/:id/uninstall", h.Uninstall)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/plugins/zombie/uninstall", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d, want 500（不是用户参数错），body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "plugin_state_not_cleared") {
		t.Errorf("响应里要带本域码，别只剩泛化的 500, got %q", body)
	}
	for _, want := range []string{"请重试卸载", "已安装", "模拟状态写入失败"} {
		if !strings.Contains(body, want) {
			t.Errorf("界面拿到的 message 里要点名 %q（旧出口压成 400「请求参数有误」，什么也不说）, got %q", want, body)
		}
	}
}
