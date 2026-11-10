// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"gorm.io/gorm"
)

// setupMonitorServer 构造真实 MonitorOper + Service（SQLite；Registry 空——
// Probe 覆盖「无采集器」失败语义）+ 路由。
func setupMonitorServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/mon.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.MonitorTarget{}, &models.MonitorAlert{}, &models.MetricSnapshot{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewMonitorOper(db, security.NewCrypto("mon-test-seed"))
	svc := monitor.NewService(op, monitor.NewRegistry(), nil, 0, nil)
	h := NewMonitorHandler(op, svc, oper.NewSSHHostKeyOper(db))
	r := gin.New()
	r.GET("/monitor/targets", h.List)
	r.POST("/monitor/targets", h.Create)
	r.PUT("/monitor/targets/:id", h.Update)
	r.DELETE("/monitor/targets/:id", h.Delete)
	r.DELETE("/monitor/alerts/:id", h.DeleteAlert)
	r.POST("/monitor/targets/:id/probe", h.Probe)
	r.GET("/monitor/targets/:id/metrics", h.Metrics)
	return r, db
}

func doMon(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req, _ = http.NewRequest(method, path, nil)
	} else {
		req, _ = http.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestMonitor_CRUD 监控目标增删改查 + 必填校验。
func TestMonitor_CRUD(t *testing.T) {
	r, _ := setupMonitorServer(t)

	// 缺 name/type → 400
	if rec := doMon(t, r, http.MethodPost, "/monitor/targets", `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺必填应 400，got %d", rec.Code)
	}

	// 创建
	rec := doMon(t, r, http.MethodPost, "/monitor/targets", `{"name":"我的NAS","type":"nas","host":"192.168.1.10"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 列表
	rec = doMon(t, r, http.MethodGet, "/monitor/targets", "")
	if !strings.Contains(rec.Body.String(), "我的NAS") {
		t.Fatalf("列表应含新目标: %s", rec.Body.String())
	}
	// 更新
	rec = doMon(t, r, http.MethodPut, "/monitor/targets/1", `{"name":"改名NAS","type":"nas","host":"192.168.1.11"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 非法 id → 400
	if rec = doMon(t, r, http.MethodPut, "/monitor/targets/zz", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	// 删除
	rec = doMon(t, r, http.MethodDelete, "/monitor/targets/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d", rec.Code)
	}
}

// TestMonitor_ProbeAndMetrics 探活（无采集器失败语义）与指标查询。
func TestMonitor_ProbeAndMetrics(t *testing.T) {
	r, _ := setupMonitorServer(t)
	doMon(t, r, http.MethodPost, "/monitor/targets", `{"name":"测试机","type":"host","host":"127.0.0.1"}`)

	// 探活失败 → 200 ok=false（不 5xx）
	rec := doMon(t, r, http.MethodPost, "/monitor/targets/1/probe", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("探活失败也应 200 ok=false，got %d", rec.Code)
	}
	// 非法 id → 400
	if rec = doMon(t, r, http.MethodPost, "/monitor/targets/zz/probe", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	// 指标查询（空历史 → 200 空语义）
	rec = doMon(t, r, http.MethodGet, "/monitor/targets/1/metrics", "")
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("空历史不应 500: %d %s", rec.Code, rec.Body.String())
	}
}
