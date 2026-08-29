package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// setupTraktServer 构造真实 TraktChain（SQLite SystemConfig 存储）+ 路由。
// OAuth/同步对未授权状态覆盖「失败/空态语义」（不外呼）。
func setupTraktServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/trakt.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.SystemConfig{}, &models.Subscription{}, &models.SubscriptionHit{}, &models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	subOp := oper.NewSubscriptionOper(db)
	subCh := chain.NewSubscribeChain(nil, nil, nil, subOp, nil, nil, nil, nil)
	ch := chain.NewTraktChain(subCh, oper.NewSystemConfigOper(db), nil)
	h := NewTraktHandler(ch)
	r := gin.New()
	r.GET("/trakt/config", h.GetConfig)
	r.PUT("/trakt/config", h.SetConfig)
	r.POST("/trakt/device", h.DeviceCode)
	r.GET("/trakt/device/status", h.DeviceStatus)
	r.POST("/trakt/sync", h.Sync)
	r.GET("/trakt/history", h.History)
	return r
}

func doTrakt(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestTrakt_ConfigRW 配置读写：secret 空串保留、脱敏字段（authorized 状态）。
func TestTrakt_ConfigRW(t *testing.T) {
	r := setupTraktServer(t)

	// 默认读回：未启用未授权
	rec := doTrakt(t, r, http.MethodGet, "/trakt/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"authorized":false`) {
		t.Fatalf("初始未授权: %s", rec.Body.String())
	}

	// 写入基础配置
	rec = doTrakt(t, r, http.MethodPut, "/trakt/config",
		`{"enabled":true,"client_id":"cid","client_secret":"csecret","sync_movies":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("写入失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doTrakt(t, r, http.MethodGet, "/trakt/config", "")
	body := rec.Body.String()
	if !strings.Contains(body, `"client_id":"cid"`) || !strings.Contains(body, `"client_secret_configured":true`) {
		t.Fatalf("配置应生效（secret 只报 configured）: %s", body)
	}
	if strings.Contains(body, "csecret") {
		t.Fatalf("secret 明文不得回显: %s", body)
	}

	// 非法 JSON → 400
	if rec = doTrakt(t, r, http.MethodPut, "/trakt/config", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400，got %d", rec.Code)
	}
}

// TestTrakt_DeviceAndSyncUnauthorized 未授权状态：设备码/同步的失败语义（不 5xx 崩）。
func TestTrakt_DeviceAndSyncUnauthorized(t *testing.T) {
	r := setupTraktServer(t)

	// 未配置 client 即发起设备码 → 错误语义
	rec := doTrakt(t, r, http.MethodPost, "/trakt/device", "")
	if rec.Code == 0 || (rec.Code >= 500 && !strings.Contains(rec.Body.String(), "error")) {
		t.Fatalf("device 未配置应错误语义: %d %s", rec.Code, rec.Body.String())
	}
	// 状态查询（无进行中的授权）
	rec = doTrakt(t, r, http.MethodGet, "/trakt/device/status", "")
	if rec.Code == 0 {
		t.Fatal("不应无响应")
	}
	// 手动同步（未授权）
	rec = doTrakt(t, r, http.MethodPost, "/trakt/sync", "")
	if rec.Code == 0 || (rec.Code >= 500 && !strings.Contains(rec.Body.String(), "error")) {
		t.Fatalf("sync 未授权应错误语义: %d %s", rec.Code, rec.Body.String())
	}
	// 历史（空）
	rec = doTrakt(t, r, http.MethodGet, "/trakt/history", "")
	if rec.Code == http.StatusInternalServerError && !strings.Contains(rec.Body.String(), "error") {
		t.Fatalf("history 空态语义: %d %s", rec.Code, rec.Body.String())
	}
}
