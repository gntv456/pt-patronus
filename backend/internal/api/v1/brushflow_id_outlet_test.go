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
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// #275：刷流接口面上 6 处 `id, _ := strconv.ParseUint(...)` 一致地把解析错误丢掉——
// `/brushflow/abc` 会被当成 id=0 继续往下走（404「任务不存在」、200 {"ok":false,…}，
// 甚至 PUT 会把一个不存在的 id 写进 Save 路径），而清单读取失败也回 400「请求参数有误」。
// 路由照生产（routes_system.go:166-173）。

func newBrushFlowOutlet(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/brushflow_outlet.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.BrushFlowTask{}, &models.BrushFlowSeed{}); err != nil {
		t.Fatal(err)
	}
	h := NewBrushFlowHandler(chain.NewBrushFlowChain(chain.Base{}, nil, nil, nil, oper.NewBrushFlowOper(db), nil, nil))
	r := gin.New()
	g := r.Group("/brushflow")
	g.GET("", h.List)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.POST("/:id/select", h.Select)
	g.POST("/:id/sweep", h.Sweep)
	g.GET("/:id/seeds", h.Seeds)
	return db, r
}

func bfReq(t *testing.T, r *gin.Engine, method, path, body string) (int, string) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

const bfValidBody = `{"name":"新规则","site_id":3,"downloader_id":4,"enabled":false,"max_seeds":5}`

func TestBrushFlowRejectsUnparseableID(t *testing.T) {
	db, r := newBrushFlowOutlet(t)
	if err := db.Create(&models.BrushFlowTask{Name: "保留", SiteID: 1, DownloaderID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/brushflow/abc", ""},
		{http.MethodPut, "/brushflow/abc", bfValidBody},
		{http.MethodDelete, "/brushflow/abc", ""},
		{http.MethodPost, "/brushflow/abc/select", ""},
		{http.MethodPost, "/brushflow/abc/sweep", ""},
		{http.MethodGet, "/brushflow/abc/seeds", ""},
	}
	for _, tc := range cases {
		status, body := bfReq(t, r, tc.method, tc.path, tc.body)
		if status != http.StatusBadRequest || !strings.Contains(body, errcode.InvalidID.Code) {
			t.Errorf("%s %s ⇒ status=%d body=%q，期望 400 + %q", tc.method, tc.path, status, body, errcode.InvalidID.Code)
		}
	}
	// PUT 的幻影插入防线：坏 id 不能凭空建任务；DELETE 的坏 id 不能碰到真数据。
	var n int64
	if err := db.Model(&models.BrushFlowTask{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("六个坏 id 请求后库里应仍是 1 条任务，实测 %d", n)
	}

	// 正控：合法 id 的读与真清空照旧可用。
	status, body := bfReq(t, r, http.MethodGet, "/brushflow/1", "")
	if status != http.StatusOK || !strings.Contains(body, "保留") {
		t.Fatalf("GET 合法 id 应 200 带回任务：%d %q", status, body)
	}
	if status, body = bfReq(t, r, http.MethodDelete, "/brushflow/1", ""); status != http.StatusOK || !strings.Contains(body, "deleted") {
		t.Fatalf("DELETE 合法 id 应 200：%d %q", status, body)
	}
}

func TestBrushFlowListsSayUnreadableNotEmpty(t *testing.T) {
	db, r := newBrushFlowOutlet(t)

	// 正控：空表（新装）⇒ 200 + tasks:[]，不是故障。
	status, body := bfReq(t, r, http.MethodGet, "/brushflow", "")
	if status != http.StatusOK || !strings.Contains(body, `"tasks":[]`) {
		t.Fatalf("status=%d body=%q，期望 200 + tasks:[]", status, body)
	}

	// 表没了 ⇒ 读不出来必须是专属码，而不是「请求参数有误」（这条 GET 没有入参）。
	if err := db.Migrator().DropTable(&models.BrushFlowTask{}); err != nil {
		t.Fatal(err)
	}
	status, body = bfReq(t, r, http.MethodGet, "/brushflow", "")
	if status != http.StatusInternalServerError || !strings.Contains(body, errcode.BrushflowTasksUnreadable.Code) {
		t.Fatalf("status=%d body=%q，期望 500 + %q", status, body, errcode.BrushflowTasksUnreadable.Code)
	}
	if strings.Contains(body, errcode.InvalidInput.Code) {
		t.Errorf("仍把读失败推成用户参数问题：%q", body)
	}
	// 种子历史同理：任务表还在、种子表没了 ⇒ 专属码（前面的 DropTable 只拿了任务表）。
	if err := db.Migrator().DropTable(&models.BrushFlowSeed{}); err != nil {
		t.Fatal(err)
	}
	status, body = bfReq(t, r, http.MethodGet, "/brushflow/1/seeds", "")
	if status != http.StatusInternalServerError || !strings.Contains(body, errcode.BrushflowSeedsUnreadable.Code) {
		t.Fatalf("status=%d body=%q，期望 500 + %q", status, body, errcode.BrushflowSeedsUnreadable.Code)
	}
}
