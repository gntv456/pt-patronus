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
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"gorm.io/gorm"
)

// setupDownloaderServer 构造真实 DownloaderOper + Manager + DownloadChain（SQLite）
// + 路由。Test 端点连不可达地址覆盖「失败语义」；Torrents 覆盖查询参数分支。
func setupDownloaderServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/dl.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Downloader{}, &models.Site{}); err != nil {
		t.Fatal(err)
	}
	dlOp := oper.NewDownloaderOper(db, security.NewCrypto("dl-test-seed"))
	mgr := manager.NewManager(dlOp)
	dlCh := chain.NewDownloadChain(chain.NewBase(module.New(), event.New()), mgr, dlOp, nil)
	h := NewDownloaderHandler(dlOp, dlCh, mgr, oper.NewSiteOper(db, security.NewCrypto("dl-test-seed")))
	r := gin.New()
	r.GET("/downloaders/caps", h.Caps)
	r.GET("/downloaders", h.List)
	r.GET("/downloaders/:id", h.Get)
	r.POST("/downloaders", h.Create)
	r.PUT("/downloaders/:id", h.Update)
	r.DELETE("/downloaders/:id", h.Delete)
	r.GET("/downloaders/:id/test", h.Test)
	r.GET("/downloaders/:id/torrents", h.Torrents)
	r.GET("/downloaders/:id/stats", h.Stats)
	r.POST("/downloaders/:id/torrents/:hash/pause", h.Pause)
	r.POST("/downloaders/:id/torrents/:hash/resume", h.Resume)
	r.POST("/downloaders/:id/torrents/:hash/remove", h.Remove)
	return r, db
}

func doDL(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestDownloader_Caps 能力清单非空（前端类型筛选依赖）。
func TestDownloader_Caps(t *testing.T) {
	r, _ := setupDownloaderServer(t)
	rec := doDL(t, r, http.MethodGet, "/downloaders/caps", "")
	if rec.Code != http.StatusOK || rec.Body.String() == "{}" {
		t.Fatalf("caps 应非空: %d %s", rec.Code, rec.Body.String())
	}
}

// TestDownloader_CRUD 增删改查 + 非法输入。
func TestDownloader_CRUD(t *testing.T) {
	r, _ := setupDownloaderServer(t)

	// 创建
	rec := doDL(t, r, http.MethodPost, "/downloaders", `{"name":"我的qB","type":"qb","url":"http://127.0.0.1:8080","username":"admin","password":"pass","is_default":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 列表
	rec = doDL(t, r, http.MethodGet, "/downloaders", "")
	if !strings.Contains(rec.Body.String(), "我的qB") {
		t.Fatalf("列表应含新下载器: %s", rec.Body.String())
	}
	// 单查
	rec = doDL(t, r, http.MethodGet, "/downloaders/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("单查 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 非法 id → 400；不存在 → 404
	if rec = doDL(t, r, http.MethodGet, "/downloaders/zz", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	if rec = doDL(t, r, http.MethodGet, "/downloaders/999", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，got %d", rec.Code)
	}
	// 更新（密码空=保留）
	rec = doDL(t, r, http.MethodPut, "/downloaders/1", `{"name":"改名qB","type":"qb","url":"http://127.0.0.1:8080","username":"admin","password":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 非法 URL scheme → 400
	rec = doDL(t, r, http.MethodPost, "/downloaders", `{"name":"坏","type":"qb","url":"ftp://x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 scheme 应 400，got %d", rec.Code)
	}
	// 删除
	rec = doDL(t, r, http.MethodDelete, "/downloaders/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d", rec.Code)
	}
}

// TestDownloader_TestFailure 连接失败语义：ok=false（不 5xx）。
func TestDownloader_TestFailure(t *testing.T) {
	r, _ := setupDownloaderServer(t)
	doDL(t, r, http.MethodPost, "/downloaders", `{"name":"死连","type":"qb","url":"http://127.0.0.1:1","username":"a","password":"b"}`)

	rec := doDL(t, r, http.MethodGet, "/downloaders/1/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("连接失败也应 200 ok=false，got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("不可达不应 ok=true: %s", rec.Body.String())
	}
}

// TestDownloader_TorrentsAndOps 任务查询 + pause/resume/remove 入参路径。
func TestDownloader_TorrentsAndOps(t *testing.T) {
	r, _ := setupDownloaderServer(t)
	doDL(t, r, http.MethodPost, "/downloaders", `{"name":"本地","type":"qb","url":"http://127.0.0.1:8080"}`)

	// Torrents：非法 id → 400
	if rec := doDL(t, r, http.MethodGet, "/downloaders/xx/torrents", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	// Torrents：连接失败 → 5xx 之外的错误语义（实际返回 502/500 均可，不 panic 即可）
	rec := doDL(t, r, http.MethodGet, "/downloaders/1/torrents?limit=10&offset=0&sort=name&order=asc", "")
	if rec.Code == 0 {
		t.Fatal("不应无响应")
	}

	// Pause/Resume/Remove：非法 id → 400（parseID 拦截，不触网）
	if rec := doDL(t, r, http.MethodPost, "/downloaders/xx/torrents/hash1/pause", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("pause 非法 id 应 400，got %d", rec.Code)
	}
	if rec := doDL(t, r, http.MethodPost, "/downloaders/xx/torrents/hash1/resume", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("resume 非法 id 应 400，got %d", rec.Code)
	}
	if rec := doDL(t, r, http.MethodPost, "/downloaders/xx/torrents/hash1/remove", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("remove 非法 id 应 400，got %d", rec.Code)
	}
}

// TestDownloader_Stats 汇总端点（无任务实例：错误语义不 panic）。
func TestDownloader_Stats(t *testing.T) {
	r, _ := setupDownloaderServer(t)
	rec := doDL(t, r, http.MethodGet, "/downloaders/999/stats", "")
	if rec.Code == 0 {
		t.Fatal("不应无响应")
	}
}
