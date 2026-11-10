// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// callListAlerts 打一次 GET /monitor/alerts 并解出响应体。
func callListAlerts(t *testing.T, op *oper.MonitorOper, query string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewMonitorHandler(op, nil, nil)
	router := gin.New()
	router.GET("/monitor/alerts", h.ListAlerts)
	req := httptest.NewRequest(http.MethodGet, "/monitor/alerts"+query, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func alertsOper(t *testing.T, unread int) *oper.MonitorOper {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/alerts.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.MonitorTarget{}, &models.MetricSnapshot{}, &models.MonitorAlert{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewMonitorOper(db, security.NewCrypto("alert-page-seed"))
	rows := make([]models.MonitorAlert, 0, unread)
	for i := 0; i < unread; i++ {
		rows = append(rows, models.MonitorAlert{TargetID: 1, Level: "warning", Metric: "disk", Message: fmt.Sprintf("m%d", i)})
	}
	if len(rows) > 0 {
		if err := db.CreateInBatches(rows, 100).Error; err != nil {
			t.Fatal(err)
		}
	}
	return op
}

// TestListAlertsExposesPageMeta 回归：接口过去只回 {alerts}，前端把「单页上限 200」当成「全部未读」。
// 键名必须与 frontend/src/api/monitor.ts 的 AlertPage 对齐，所以这一层也要有测试。
func TestListAlertsExposesPageMeta(t *testing.T) {
	op := alertsOper(t, oper.MonitorAlertListLimit+3)
	out := callListAlerts(t, op, "?unread=true")
	for _, k := range []string{"alerts", "total", "limit", "truncated"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("响应缺字段 %s：%v", k, out)
		}
	}
	if got := out["total"]; got != float64(oper.MonitorAlertListLimit+3) {
		t.Errorf("total 要是同口径总数 %d，got %v", oper.MonitorAlertListLimit+3, got)
	}
	if out["truncated"] != true {
		t.Errorf("被截断时 truncated 必须为 true，got %v", out["truncated"])
	}
	if list, ok := out["alerts"].([]any); !ok || len(list) != oper.MonitorAlertListLimit {
		t.Errorf("alerts 应截到上限 %d，got %v", oper.MonitorAlertListLimit, len(list))
	}

	small := callListAlerts(t, alertsOper(t, 2), "?unread=true")
	if small["truncated"] != false || small["total"] != float64(2) {
		t.Errorf("没到上限时不该报截断，got %v / %v", small["truncated"], small["total"])
	}
}
