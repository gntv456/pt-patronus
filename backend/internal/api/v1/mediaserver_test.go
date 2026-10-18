package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// setupMediaServerEnv 构造真实 MediaServerChain（SQLite 持久层）+ 路由。
// Test/Libraries 等外呼端点以不可达地址覆盖「失败语义」（ok=false 不 500）。
func setupMediaServerEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ms.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.MediaServer{}); err != nil {
		t.Fatal(err)
	}
	msOp := oper.NewMediaServerOper(db, security.NewCrypto("ms-test-seed"))
	svc, err := chain.NewMediaServerChain(chain.NewBase(module.New(), event.New()), msOp, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewMediaServerHandler(svc)
	r := gin.New()
	r.GET("/mediaservers", h.List)
	r.POST("/mediaservers", h.Create)
	r.PUT("/mediaservers/:id", h.Update)
	r.DELETE("/mediaservers/:id", h.Delete)
	r.GET("/mediaservers/:id/test", h.Test)
	r.GET("/mediaservers/:id/libraries", h.Libraries)
	r.GET("/mediaservers/:id/health", h.Health)
	r.POST("/mediaservers/:id/scan", h.Scan)
	r.POST("/mediaservers/:id/refresh", h.Refresh)
	r.POST("/mediaservers/:id/webhook", h.Webhook)
	r.POST("/mediaservers/:id/playback-report", h.PlaybackReport)
	r.GET("/mediaservers/:id/duplicates", h.Duplicates)
	return r, db
}

func doMS(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestMediaServer_ListEmptyAndCreate 空列表空数组（非 null）+ CRUD。
func TestMediaServer_ListEmptyAndCreate(t *testing.T) {
	r, _ := setupMediaServerEnv(t)

	rec := doMS(t, r, http.MethodGet, "/mediaservers", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"servers":[]`) {
		t.Fatalf("空列表应空数组: %d %s", rec.Code, rec.Body.String())
	}

	rec = doMS(t, r, http.MethodPost, "/mediaservers", `{"name":"我的Emby","type":"emby","url":"http://127.0.0.1:8096","token":"abc"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doMS(t, r, http.MethodGet, "/mediaservers", "")
	if !strings.Contains(rec.Body.String(), "我的Emby") {
		t.Fatalf("列表应含新服务器: %s", rec.Body.String())
	}

	// 更新（token 空=保留）
	rec = doMS(t, r, http.MethodPut, "/mediaservers/1", `{"name":"改名Emby","type":"emby","url":"http://127.0.0.1:8096","token":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 非法 id → 400
	if rec = doMS(t, r, http.MethodPut, "/mediaservers/xx", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	// 删除
	rec = doMS(t, r, http.MethodDelete, "/mediaservers/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestMediaServer_TestConnectionFailure 探活失败语义：ok=false + 原因（不 5xx）。
func TestMediaServer_TestConnectionFailure(t *testing.T) {
	r, _ := setupMediaServerEnv(t)
	doMS(t, r, http.MethodPost, "/mediaservers", `{"name":"死服","type":"emby","url":"http://127.0.0.1:1","token":"x"}`)

	rec := doMS(t, r, http.MethodGet, "/mediaservers/1/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("探活失败也应 200 携带 ok=false，got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("不可达服务器不应 ok=true: %s", rec.Body.String())
	}
}

// TestMediaServer_LibrariesParamValidation /libraries 的 id 校验路径。
// （/health 是全实例健康报告、不按 id 寻址，无参数校验分支。）
func TestMediaServer_LibrariesParamValidation(t *testing.T) {
	r, _ := setupMediaServerEnv(t)
	if rec := doMS(t, r, http.MethodGet, "/mediaservers/bad/libraries", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
}

// TestMediaServer_ActionEndpoints 扫描/刷新/webhook 等操作端点（连接失败语义不 5xx 崩）。
func TestMediaServer_ActionEndpoints(t *testing.T) {
	r, _ := setupMediaServerEnv(t)
	doMS(t, r, http.MethodPost, "/mediaservers", `{"name":"操作服","type":"emby","url":"http://127.0.0.1:1","token":"x"}`)

	endpoints := []struct{ method, path, body string }{
		{http.MethodPost, "/mediaservers/1/scan", `{}`},
		{http.MethodPost, "/mediaservers/1/scan", ``},
		{http.MethodPost, "/mediaservers/1/refresh", `{}`},
		{http.MethodPost, "/mediaservers/1/webhook", `{}`},
		{http.MethodPost, "/mediaservers/1/playback-report", `{}`},
		{http.MethodGet, "/mediaservers/1/duplicates", ``},
	}
	for _, ep := range endpoints {
		rec := doMS(t, r, ep.method, ep.path, ep.body)
		if rec.Code == 0 {
			t.Fatalf("%s 不应无响应", ep.path)
		}
		if rec.Code >= 500 && !strings.Contains(rec.Body.String(), "error") {
			t.Fatalf("%s 5xx 应带 error: %d %s", ep.path, rec.Code, rec.Body.String())
		}
	}

	// 非法 id → 400（parseID 拦截）
	if rec := doMS(t, r, http.MethodPost, "/mediaservers/zz/scan", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("scan 非法 id 应 400，got %d", rec.Code)
	}
	// 非法 JSON body（非 EOF）→ 400
	if rec := doMS(t, r, http.MethodPost, "/mediaservers/1/scan", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("scan 非法 JSON 应 400，got %d", rec.Code)
	}
}

// TestMediaServer_Health 全实例健康报告（无服务器实例 → 空报告不崩）。
func TestMediaServer_Health(t *testing.T) {
	r, _ := setupMediaServerEnv(t)
	rec := doMS(t, r, http.MethodGet, "/mediaservers/1/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("health 应 200，got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestMediaServer_MissingRowIsNotFoundNotBadRequest 回归：库里没有这台服务器要说「不存在」(404)。
// 修复前 10 处一律 InvalidInput → 用户删掉服务器后另一个标签页/书签里的旧 id 换来
// 「请求参数有误，请检查后重试」，被支去改参数而不是重选服务器；软失败面还把
// raw Go 错误 "record not found" 原样弹给用户。
func TestMediaServer_MissingRowIsNotFoundNotBadRequest(t *testing.T) {
	r, _ := setupMediaServerEnv(t)
	rec := doMS(t, r, http.MethodGet, "/mediaservers/999/libraries", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在的服务器应 404，got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "mediaserver_not_found") {
		t.Fatalf("应带 mediaserver_not_found 码，got %s", rec.Body.String())
	}

	rec2 := doMS(t, r, http.MethodGet, "/mediaservers/999/duplicates?library_id=1", "")
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("duplicates 同样应 404，got %d body=%s", rec2.Code, rec2.Body.String())
	}

	rec3 := doMS(t, r, http.MethodGet, "/mediaservers/999/test", "")
	if !strings.Contains(rec3.Body.String(), "媒体服务器不存在") {
		t.Fatalf("软失败面应给中文而不是 raw 错误，got %s", rec3.Body.String())
	}
	if strings.Contains(rec3.Body.String(), "record not found") {
		t.Fatalf("不得把 Go 原文弹给用户，got %s", rec3.Body.String())
	}
}

// TestMediaServerCreateValidationReason 校验类 400 要带 oper 写好的中文原因。
// 修复前一律「请求参数有误，请检查后重试」：用户填错协议或选了不支持的类型，
// 不知道是哪一个字段错，只能瞎试。
func TestMediaServerCreateValidationReason(t *testing.T) {
	r, _ := setupMediaServerEnv(t)

	badURL := doMS(t, r, http.MethodPost, "/mediaservers", `{"name":"m1","type":"emby","url":"ftp://192.168.1.5:8096","api_key":"k"}`)
	if badURL.Code != http.StatusBadRequest {
		t.Fatalf("非法协议应 400，got %d body=%s", badURL.Code, badURL.Body.String())
	}
	if !strings.Contains(badURL.Body.String(), "http/https") {
		t.Fatalf("应说明协议要求，got %s", badURL.Body.String())
	}

	badType := doMS(t, r, http.MethodPost, "/mediaservers", `{"name":"m2","type":"kodi","url":"http://127.0.0.1:8096","api_key":"k"}`)
	if badType.Code != http.StatusBadRequest {
		t.Fatalf("未知类型应 400，got %d", badType.Code)
	}
	if !strings.Contains(badType.Body.String(), "媒体服务器类型") {
		t.Fatalf("应点名类型白名单问题，got %s", badType.Body.String())
	}
}
