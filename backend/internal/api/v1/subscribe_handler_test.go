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

// setupSubscribeServer 构造真实 SubscribeChain（SQLite 持久层；mediaCh=nil——
// ListWithPosters 对 nil 媒体链安全短路，不发外网）+ 路由。
func setupSubscribeServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sub.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Subscription{}, &models.SubscriptionHit{}, &models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	subOp := oper.NewSubscriptionOper(db)
	subCh := chain.NewSubscribeChain(nil, nil, nil, subOp, nil, nil, nil, nil)
	h := NewSubscribeHandler(subCh)
	r := gin.New()
	r.GET("/subscribe", h.List)
	r.POST("/subscribe", h.Create)
	r.PUT("/subscribe/:id", h.Update)
	r.DELETE("/subscribe/:id", h.Delete)
	r.GET("/subscribe/:id/check", h.Check)
	r.POST("/subscribe/batch", h.Batch)
	r.GET("/subscribe/settings", h.GetGlobalSettings)
	r.PUT("/subscribe/settings", h.UpdateGlobalSettings)
	return r, db
}

func doSub(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestSubscribe_ListEmpty 空列表返回空数组（不 null）。
func TestSubscribe_ListEmpty(t *testing.T) {
	r, _ := setupSubscribeServer(t)
	rec := doSub(t, r, http.MethodGet, "/subscribe", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestSubscribe_CreateAndList 建订阅（必填校验 + 可选字段）后可列出。
func TestSubscribe_CreateAndList(t *testing.T) {
	r, _ := setupSubscribeServer(t)

	// 缺 source/source_id → 400
	rec := doSub(t, r, http.MethodPost, "/subscribe", `{"title":"只有标题"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺必填应 400，got %d", rec.Code)
	}

	// 完整创建
	rec = doSub(t, r, http.MethodPost, "/subscribe",
		`{"title":"测试剧集","source":"tmdb","source_id":"tv:123","type":"电视剧","year":2024,"resolution":"1080p","site_ids":[1,2],"promo_types":["free"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doSub(t, r, http.MethodGet, "/subscribe", "")
	if !strings.Contains(rec.Body.String(), "测试剧集") {
		t.Fatalf("列表应含新订阅: %s", rec.Body.String())
	}
}

// TestSubscribe_Delete 删除（含非法 id 与不存在 id）。
func TestSubscribe_Delete(t *testing.T) {
	r, _ := setupSubscribeServer(t)
	doSub(t, r, http.MethodPost, "/subscribe", `{"title":"待删","source":"tmdb","source_id":"tv:9"}`)

	if rec := doSub(t, r, http.MethodDelete, "/subscribe/abc", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	rec := doSub(t, r, http.MethodDelete, "/subscribe/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestSubscribe_CheckMissing 检查不存在订阅 → 404。
func TestSubscribe_CheckMissing(t *testing.T) {
	r, _ := setupSubscribeServer(t)
	rec := doSub(t, r, http.MethodGet, "/subscribe/999/check", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，got %d", rec.Code)
	}
}

// TestSubscribe_BatchValidation 批量操作入参校验。
func TestSubscribe_BatchValidation(t *testing.T) {
	r, _ := setupSubscribeServer(t)
	// 空 ids → 400
	if rec := doSub(t, r, http.MethodPost, "/subscribe/batch", `{"ids":[],"op":"delete"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("空 ids 应 400，got %d", rec.Code)
	}
	// 非法 op → 400
	if rec := doSub(t, r, http.MethodPost, "/subscribe/batch", `{"ids":[1],"op":"bad"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 op 应 400，got %d", rec.Code)
	}
	// 合法 delete（id 不存在——逐条执行单条失败不影响汇总）
	rec := doSub(t, r, http.MethodPost, "/subscribe/batch", `{"ids":[999],"op":"delete"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("批量 delete 应 200 汇总，got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestSubscribe_GlobalSettings 全局设置读写。
func TestSubscribe_GlobalSettings(t *testing.T) {
	r, _ := setupSubscribeServer(t)
	rec := doSub(t, r, http.MethodGet, "/subscribe/settings", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}
