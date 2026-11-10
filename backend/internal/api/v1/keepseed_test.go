// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"gorm.io/gorm"
)

// setupKeepSeedServer 构造真实 KeepSeedChain（SQLite；IYUU client 指向不可达地址，
// Reseed/Find 覆盖失败语义）。
func setupKeepSeedServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ks.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Site{}, &models.Downloader{}, &models.SystemConfig{}, &models.CrossSeedTask{}); err != nil {
		t.Fatal(err)
	}
	base := chain.NewBase(module.New(), event.New())
	siteOp := oper.NewSiteOper(db, security.NewCrypto("ks-test-seed"))
	siteCh := chain.NewSiteChain(base, siteOp, nil)
	hc, _ := httpclient.New(httpclient.Options{Timeout: 3 * time.Second})
	ks := chain.NewKeepSeedChain(base, siteCh, iyuu.New(hc, "http://127.0.0.1:1"),
		nil, nil, siteOp, oper.NewSystemConfigOper(db), db, nil)
	h := NewKeepSeedHandler(ks)
	r := gin.New()
	r.POST("/keepseed/find", h.Find)
	r.GET("/keepseed/config", h.GetConfig)
	r.PUT("/keepseed/config", h.SetConfig)
	r.POST("/keepseed/reseed", h.Reseed)
	r.POST("/keepseed/missing", h.Missing)
	r.GET("/keepseed/tasks", h.Tasks)
	r.POST("/keepseed/tasks/:id/resolve", h.Resolve)
	return r
}

func doKS(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

// TestKeepSeed_FindValidation 辅种查找必填校验。
func TestKeepSeed_FindValidation(t *testing.T) {
	r := setupKeepSeedServer(t)
	if rec := doKS(t, r, http.MethodPost, "/keepseed/find", `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 name 应 400，got %d", rec.Code)
	}
	if rec := doKS(t, r, http.MethodPost, "/keepseed/find", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 body 应 400，got %d", rec.Code)
	}
}

// TestKeepSeed_ConfigRW 配置读写 + token 掩码回显。
func TestKeepSeed_ConfigRW(t *testing.T) {
	r := setupKeepSeedServer(t)

	rec := doKS(t, r, http.MethodGet, "/keepseed/config", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Fatalf("初始未配置: %d %s", rec.Code, rec.Body.String())
	}

	rec = doKS(t, r, http.MethodPut, "/keepseed/config", `{"iyuu_token":"TOKEN123456789"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("写入失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doKS(t, r, http.MethodGet, "/keepseed/config", "")
	body := rec.Body.String()
	if !strings.Contains(body, `"configured":true`) {
		t.Fatalf("配置后应 configured=true: %s", body)
	}
	if strings.Contains(body, "TOKEN123456789") {
		t.Fatalf("token 明文不得回显: %s", body)
	}
	// 掩码形态：前4 + **** + 后4
	if !strings.Contains(body, "TOKE") || !strings.Contains(body, "****") {
		t.Fatalf("应有掩码 token: %s", body)
	}

	// 非法 JSON → 400
	if rec = doKS(t, r, http.MethodPut, "/keepseed/config", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400，got %d", rec.Code)
	}
}

// TestKeepSeed_ReseedValidation 触发辅种与未辅种检查的入参/失败语义。
func TestKeepSeed_ReseedValidation(t *testing.T) {
	r := setupKeepSeedServer(t)

	if rec := doKS(t, r, http.MethodPost, "/keepseed/reseed", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 downloader_id 应 400，got %d", rec.Code)
	}
	if rec := doKS(t, r, http.MethodPost, "/keepseed/missing", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing 缺参应 400，got %d", rec.Code)
	}
}

// TestKeepSeed_TasksAndResolve 任务列表（空）与 resolve 校验。
func TestKeepSeed_TasksAndResolve(t *testing.T) {
	r := setupKeepSeedServer(t)

	rec := doKS(t, r, http.MethodGet, "/keepseed/tasks?status=pending", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "tasks") {
		t.Fatalf("tasks 应 200: %d %s", rec.Code, rec.Body.String())
	}
	if rec = doKS(t, r, http.MethodPost, "/keepseed/tasks/zz/resolve", `{"action":"skip"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("resolve 非法 id 应 400，got %d", rec.Code)
	}
	if rec = doKS(t, r, http.MethodPost, "/keepseed/tasks/999/resolve", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("resolve 缺 action 应 400，got %d", rec.Code)
	}
}
