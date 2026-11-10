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
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// setupServarrServer 构造真实 SubscribeChain（SQLite）+ Servarr 兼容层路由。
// 覆盖鉴权（apikey 惯例）与订阅映射的读路径。
func setupServarrServer(t *testing.T, apiKey string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/servarr.db"), &gorm.Config{})
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
	h := NewServarrHandler(subCh, nil, nil, apiKey)
	r := gin.New()
	r.GET("/api/v3/system/status", h.SystemStatus)
	r.GET("/api/v3/series", h.ListSeries)
	r.GET("/api/v3/movie", h.ListMovies)
	return r
}

func doArr(t *testing.T, r *gin.Engine, path, apikey string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	if apikey != "" {
		req.Header.Set("X-Api-Key", apikey)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestServarr_AuthGate 鉴权闸门：未配 key / 缺 key / 错 key 全 401，对 key 200。
func TestServarr_AuthGate(t *testing.T) {
	// 未配置 API Key：全部拒绝（不留隐式后门）
	r := setupServarrServer(t, "")
	if rec := doArr(t, r, "/api/v3/system/status", "anything"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("未配 key 应 401，got %d", rec.Code)
	}

	// 配了 key：缺 key / 错 key → 401；对 key → 200
	r2 := setupServarrServer(t, "arr-secret")
	if rec := doArr(t, r2, "/api/v3/system/status", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("缺 key 应 401，got %d", rec.Code)
	}
	if rec := doArr(t, r2, "/api/v3/system/status", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("错 key 应 401，got %d", rec.Code)
	}
	rec := doArr(t, r2, "/api/v3/system/status", "arr-secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("对 key 应 200，got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"appName":"PTPatronus"`) {
		t.Fatalf("status 应带 appName: %s", rec.Body.String())
	}
}

// TestServarr_Lists 订阅映射列表（空态 → 200 空数组语义）。
func TestServarr_Lists(t *testing.T) {
	r := setupServarrServer(t, "arr-secret")

	if rec := doArr(t, r, "/api/v3/series", "arr-secret"); rec.Code != http.StatusOK {
		t.Fatalf("series 应 200，got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := doArr(t, r, "/api/v3/movie", "arr-secret"); rec.Code != http.StatusOK {
		t.Fatalf("movie 应 200，got %d body=%s", rec.Code, rec.Body.String())
	}
}
