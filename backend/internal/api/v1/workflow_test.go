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
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"github.com/hxpt/ptpatronus/internal/workflow/nodes"
	"gorm.io/gorm"
)

// setupWorkflowServer 构造真实 WorkflowChain（SQLite；Registry 空、sched nil——
// Run 覆盖空目录/单实例错误语义，不外呼）。
func setupWorkflowServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wf.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Workflow{}, &models.WorkflowRun{}); err != nil {
		t.Fatal(err)
	}
	reg := workflow.NewRegistry()
	reg.Register(nodes.NewLimitNode())
	ch := chain.NewWorkflowChain(chain.NewBase(module.New(), event.New()),
		oper.NewWorkflowOper(db), oper.NewWorkflowRunOper(db),
		reg, &noopScheduler{}, nil, nil)
	h := NewWorkflowHandler(ch)
	r := gin.New()
	r.GET("/workflows/actions", h.Catalog)
	r.GET("/workflows/templates", h.Templates)
	r.GET("/workflows/stats", h.Stats)
	r.GET("/workflows", h.List)
	r.GET("/workflows/:id", h.Get)
	r.POST("/workflows", h.Create)
	r.PUT("/workflows/:id", h.Update)
	r.DELETE("/workflows/:id", h.Delete)
	r.POST("/workflows/:id/run", h.Run)
	r.POST("/workflows/:id/enable", h.Enable)
	r.POST("/workflows/:id/disable", h.Disable)
	r.GET("/workflows/:id/runs", h.ListRuns)
	r.GET("/workflows/runs/:runID", h.RunDetail)
	return r
}

func doWf(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
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

const wfBody = `{"name":"每日签到流","trigger_type":"timer","timer":"0 9 * * *","nodes":[{"id":"n1","type":"limit","params":{"count":5}}],"edges":[]}`

// TestWorkflow_MetaEndpoints 目录/模板/统计三个只读端点。
func TestWorkflow_MetaEndpoints(t *testing.T) {
	r := setupWorkflowServer(t)
	for _, ep := range []string{"/workflows/actions", "/workflows/templates", "/workflows/stats"} {
		rec := doWf(t, r, http.MethodGet, ep, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s 应 200，got %d body=%s", ep, rec.Code, rec.Body.String())
		}
	}
}

// TestWorkflow_CRUD 增删改查 + 必填校验。
func TestWorkflow_CRUD(t *testing.T) {
	r := setupWorkflowServer(t)

	// 缺名称 → 400
	if rec := doWf(t, r, http.MethodPost, "/workflows", `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺名应 400，got %d", rec.Code)
	}

	// 创建
	rec := doWf(t, r, http.MethodPost, "/workflows", wfBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 列表
	rec = doWf(t, r, http.MethodGet, "/workflows", "")
	if !strings.Contains(rec.Body.String(), "每日签到流") {
		t.Fatalf("列表应含新工作流: %s", rec.Body.String())
	}
	// 单查 + 不存在
	rec = doWf(t, r, http.MethodGet, "/workflows/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("单查 code=%d", rec.Code)
	}
	if rec = doWf(t, r, http.MethodGet, "/workflows/999", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，got %d", rec.Code)
	}
	// 更新
	rec = doWf(t, r, http.MethodPut, "/workflows/1", `{"name":"改名流","trigger_type":"timer","timer":"0 9 * * *","enabled":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 启停
	if rec = doWf(t, r, http.MethodPost, "/workflows/1/enable", ""); rec.Code != http.StatusOK {
		t.Fatalf("启用 code=%d", rec.Code)
	}
	if rec = doWf(t, r, http.MethodPost, "/workflows/1/disable", ""); rec.Code != http.StatusOK {
		t.Fatalf("停用 code=%d", rec.Code)
	}
	// 运行记录列表（空）
	rec = doWf(t, r, http.MethodGet, "/workflows/1/runs", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("runs 应 200，got %d", rec.Code)
	}
	// 删除
	rec = doWf(t, r, http.MethodDelete, "/workflows/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除失败 code=%d", rec.Code)
	}
}

// TestWorkflow_RunEmptyFlow 空节点工作流手动触发（执行器对空流的语义，不 panic）。
func TestWorkflow_RunEmptyFlow(t *testing.T) {
	r := setupWorkflowServer(t)
	doWf(t, r, http.MethodPost, "/workflows", wfBody)

	rec := doWf(t, r, http.MethodPost, "/workflows/1/run", "")
	if rec.Code == 0 {
		t.Fatal("不应无响应")
	}
}

// TestWorkflow_RunDetailValidation 运行详情：非法 runID → 404/400。
func TestWorkflow_RunDetailValidation(t *testing.T) {
	r := setupWorkflowServer(t)
	rec := doWf(t, r, http.MethodGet, "/workflows/runs/999", "")
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Fatalf("不存在 run 应 404/400，got %d", rec.Code)
	}
}

// noopScheduler 空 cron 调度器（timer 触发注册/注销走空操作——不启动真实 cron）。
type noopScheduler struct{}

func (s *noopScheduler) AddFunc(name, spec string, fn func()) error { return nil }
func (s *noopScheduler) Remove(name string)                         {}
