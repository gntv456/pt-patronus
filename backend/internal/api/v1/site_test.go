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
	"gorm.io/gorm"
)

// setupSiteServer 构造真实 SiteOper + SiteChain（SQLite；hc 传 nil——
// SyncHostLimits 对 nil client 安全跳过，不触发外网）。
func setupSiteServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/site.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto("site-test-seed"))
	siteCh := chain.NewSiteChain(chain.NewBase(module.New(), event.New()), siteOp, nil)
	h := NewSiteHandler(siteOp, siteCh, nil, nil)
	r := gin.New()
	r.GET("/sites", h.List)
	r.GET("/sites/:id", h.Get)
	r.POST("/sites", h.Create)
	r.PUT("/sites/:id", h.Update)
	r.DELETE("/sites/:id", h.Delete)
	r.POST("/sites/batch-delete", h.BatchDelete)
	r.PUT("/sites/:id/cookie", h.UpdateCookie)
	r.GET("/sites/cookies", h.GetCookies)
	r.GET("/sites/cookies/reveal", h.GetCookiesReveal)
	return r, db
}

func doSite(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

const siteBody = `{"name":"测试站","platform":"nexusphp","base_url":"https://t.example.org","cookie":"uid=1","is_active":true}`

// TestSite_CRUD 全生命周期：建 → 查 → 改 → 删。
func TestSite_CRUD(t *testing.T) {
	r, _ := setupSiteServer(t)

	// 建
	rec := doSite(t, r, http.MethodPost, "/sites", siteBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 列表
	rec = doSite(t, r, http.MethodGet, "/sites", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "测试站") {
		t.Fatalf("列表应含新站: %s", rec.Body.String())
	}
	// 单查（id=1）
	rec = doSite(t, r, http.MethodGet, "/sites/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("单查 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 非法 id → 400（parseID 拦截）
	rec = doSite(t, r, http.MethodGet, "/sites/abc", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
	// 不存在 → 404
	rec = doSite(t, r, http.MethodGet, "/sites/999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，got %d", rec.Code)
	}
	// 改（cookie 空 → 保留原值语义由 oper 层处理）
	rec = doSite(t, r, http.MethodPut, "/sites/1", `{"name":"改名站","platform":"nexusphp","base_url":"https://t.example.org","is_active":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doSite(t, r, http.MethodGet, "/sites/1", "")
	if !strings.Contains(rec.Body.String(), "改名站") {
		t.Fatalf("更新后应读到新名: %s", rec.Body.String())
	}
	// 非法输入（缺 name）→ 400
	rec = doSite(t, r, http.MethodPost, "/sites", `{"platform":"nexusphp","base_url":"https://x.org"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 name 应 400，got %d", rec.Code)
	}
	// 删（四轮起要求 confirm_risk 二次确认）
	rec = doSite(t, r, http.MethodDelete, "/sites/1", `{"confirm_risk":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d", rec.Code)
	}
	// 不带确认 → risk_confirmation_required（400）
	rec = doSite(t, r, http.MethodDelete, "/sites/1", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 confirm_risk 应 400，got %d", rec.Code)
	}
	rec = doSite(t, r, http.MethodGet, "/sites/1", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("删除后应 404，got %d", rec.Code)
	}
}

// TestSite_BatchDelete 批量删除：混合存在/不存在 id。
func TestSite_BatchDelete(t *testing.T) {
	r, db := setupSiteServer(t)
	hosts := []string{"a-test1.org", "b-test2.org", "c-test3.org"}
	for i, host := range hosts {
		body := strings.Replace(siteBody, "测试站", "站"+strings.Repeat("a", i+1), 1)
		body = strings.Replace(body, "t.example.org", host, 1)
		rec := doSite(t, r, http.MethodPost, "/sites", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("预置站点 %s 失败: %s", host, rec.Body.String())
		}
	}
	var cnt int64
	db.Model(&models.Site{}).Count(&cnt)
	if cnt != 3 {
		t.Fatalf("预置应有 3 站，got %d", cnt)
	}

	// 删 [1,2,999]：2 成功 + 1 失败
	rec := doSite(t, r, http.MethodPost, "/sites/batch-delete", `{"ids":[1,2,999],"confirm_risk":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"deleted":2`) || !strings.Contains(rec.Body.String(), "999") {
		t.Fatalf("应报 2 删 1 失败: %s", rec.Body.String())
	}
	// 空 ids → 400
	rec = doSite(t, r, http.MethodPost, "/sites/batch-delete", `{"ids":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("空 ids 应 400，got %d", rec.Code)
	}
}

// TestSite_CookieLedger Cookie 台账：指纹脱敏 + reveal 明文 + 更新。
func TestSite_CookieLedger(t *testing.T) {
	r, _ := setupSiteServer(t)
	doSite(t, r, http.MethodPost, "/sites", siteBody)

	// 默认（指纹视图）：含 fingerprint/has_cookie，不泄露明文
	rec := doSite(t, r, http.MethodGet, "/sites/cookies", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "fingerprint") {
		t.Fatalf("台账应含指纹: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "uid=1") {
		t.Fatalf("指纹视图不得回明文 cookie: %s", rec.Body.String())
	}

	// 越权回归（P0）：非 admin 端点带 ?reveal=1 也不得回明文——明文只属于
	// 路由层挂 RequireAdmin 的 /sites/cookies/reveal，query 参数不能穿透闸门。
	rec = doSite(t, r, http.MethodGet, "/sites/cookies?reveal=1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("?reveal=1 应正常 200: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "uid=1") {
		t.Fatalf("非 admin 端点 ?reveal=1 不得回明文 cookie: %s", rec.Body.String())
	}

	// reveal：管理员明文版（cookie 值可见）
	rec = doSite(t, r, http.MethodGet, "/sites/cookies/reveal", "")
	if !strings.Contains(rec.Body.String(), "uid=1") {
		t.Fatalf("reveal 应回明文: %s", rec.Body.String())
	}

	// 更新 cookie
	rec = doSite(t, r, http.MethodPut, "/sites/1/cookie", `{"cookie":"uid=2;new=1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新 cookie 失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 空非法 body → 400
	if rec = doSite(t, r, http.MethodPut, "/sites/1/cookie", `{bad`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 body 应 400，got %d", rec.Code)
	}
	// 非法 id → 400
	if rec = doSite(t, r, http.MethodPut, "/sites/zz/cookie", `{"cookie":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，got %d", rec.Code)
	}
}
