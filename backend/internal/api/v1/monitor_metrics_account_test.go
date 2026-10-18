package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"gorm.io/gorm"
)

// callDeviceMetrics 打一次 GET /monitors/:id/metrics 并解出响应体。
func callDeviceMetrics(t *testing.T, op *oper.MonitorOper, targetID uint, query string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewMonitorHandler(op, nil, nil)
	router := gin.New()
	router.GET("/monitors/:id/metrics", h.Metrics)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/monitors/%d/metrics%s", targetID, query), nil)
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

// TestDeviceMetricsAccount 趋势图取数账面：
// ① 被上限截断时必须报 total/truncated，否则「最近 3 次采集」被画成「这台设备的全部历史」；
// ② 请求里被归一的 limit 要回显生效值（前端不能以为自己拿到了 999999 条）；
// ③ 一次拿全时 truncated 必须为假（恒真是另一种说谎）。
func TestDeviceMetricsAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/metrics.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.MonitorTarget{}, &models.MetricSnapshot{}, &models.MonitorAlert{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewMonitorOper(db, security.NewCrypto("metrics-account-seed"))
	target, err := op.Create(oper.TargetInput{Name: "nas-trend", Type: "host", Protocol: "local", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 8; i++ {
		if err := db.Create(&models.MetricSnapshot{
			TargetID: target.ID,
			SnapAt:   base.Add(time.Duration(i) * time.Minute),
			Online:   true,
			Metrics:  monitor.MetricsJSON(monitor.Metrics{Online: true, CPUUsage: float64(i)}),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	out := callDeviceMetrics(t, op, target.ID, "?limit=3")
	if got := out["total"]; got != float64(8) {
		t.Errorf("total 应为 8，got %v", got)
	}
	if out["truncated"] != true {
		t.Errorf("只回 3 条时必须报截断，got %v", out["truncated"])
	}
	if out["limit"] != float64(3) {
		t.Errorf("limit 应回显 3，got %v", out["limit"])
	}
	if snaps, _ := out["snapshots"].([]any); len(snaps) != 3 {
		t.Errorf("清单应 3 条，got %v", out["snapshots"])
	}

	out = callDeviceMetrics(t, op, target.ID, "?limit=999999")
	if out["limit"] != float64(oper.SnapshotListLimit) {
		t.Errorf("超上限请求应回显生效上限 %d，got %v", oper.SnapshotListLimit, out["limit"])
	}
	if out["truncated"] != false {
		t.Errorf("8 条一次拿全不该报截断，got %v", out["truncated"])
	}

	out = callDeviceMetrics(t, op, target.ID, "")
	if out["limit"] != float64(oper.SnapshotListDefault) {
		t.Errorf("留空应回显默认 %d，got %v", oper.SnapshotListDefault, out["limit"])
	}
}
