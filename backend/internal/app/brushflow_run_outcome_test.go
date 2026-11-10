// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/mcp"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"gorm.io/gorm"
)

// newBrushFlowFixture 建一条只带任务表的 BrushFlowChain（GetTask/诊断回读要真库）。
func newBrushFlowFixture(t *testing.T) (*chain.BrushFlowChain, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/bf.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.BrushFlowTask{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	bOp := oper.NewBrushFlowOper(db)
	base := chain.NewBase(module.New(), event.New())
	bf := chain.NewBrushFlowChain(base, nil, nil, nil, bOp, nil, nil)
	return bf, db
}

func seedTask(t *testing.T, db *gorm.DB, id uint, enabled bool) {
	t.Helper()
	task := models.BrushFlowTask{ID: id, Name: "t", Enabled: enabled, DownloaderID: 1, SiteID: 1}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

// TestBrushflowRunOutcomeRejectsDisabledTask 回归核心：RunSelect/RunSweep 对未启用任务
// 直接 return nil，工具原本 okCount(...,1,nil) → 回执 {"selected":1}，
// 模型/插件据此认为「已把这批种子送进下载器」，而实际一条都没动。
func TestBrushflowRunOutcomeRejectsDisabledTask(t *testing.T) {
	bf, db := newBrushFlowFixture(t)
	seedTask(t, db, 1, false)
	ran := 0
	out, err := brushflowRunOutcome(context.Background(), bf, 1, "select", "selected",
		func(context.Context, uint) error { ran++; return nil })
	if err == nil {
		t.Fatalf("未启用任务必须失败，got %+v", out)
	}
	if !strings.Contains(err.Error(), "未启用") {
		t.Errorf("错误应说明任务未启用，got %v", err)
	}
	if ran != 0 {
		t.Errorf("未启用时不得执行任何选取/删除，ran=%d", ran)
	}
	if out != nil {
		t.Errorf("失败时不应带回回执，got %+v", out)
	}
}

// TestBrushflowRunOutcomeCarriesTaskDiagnostics 逐项失败（推送下载器失败 N 条）只落在
// 任务诊断字段、chain 仍 return nil；工具必须把它带回，否则空跑与全失败轮都显示成功。
func TestBrushflowRunOutcomeCarriesTaskDiagnostics(t *testing.T) {
	bf, db := newBrushFlowFixture(t)
	seedTask(t, db, 2, true)
	op := oper.NewBrushFlowOper(db)
	out, err := brushflowRunOutcome(context.Background(), bf, 2, "select", "selected",
		func(context.Context, uint) error {
			return op.SetTaskError(2, "select", "推送下载器失败 3 条: add status 403")
		})
	if err != nil {
		t.Fatalf("chain 本身成功时不该报错: %v", err)
	}
	if out["selected"] != 1 {
		t.Errorf("回执应保留计数，got %+v", out)
	}
	if got, _ := out["task_error"].(string); !strings.Contains(got, "推送下载器失败 3 条") {
		t.Errorf("回执必须带回本轮诊断，got %+v", out)
	}
	if out["task_error_new"] != true {
		t.Errorf("本轮新增的诊断要标 new，got %+v", out)
	}
}

// TestBrushflowRunOutcomeDoesNotReattributeOldError 正控边界：上一次运行留下的同一条诊断
// 不能被打成「本次新发生」（否则模型把旧账当本轮故障）。
func TestBrushflowRunOutcomeDoesNotReattributeOldError(t *testing.T) {
	bf, db := newBrushFlowFixture(t)
	seedTask(t, db, 3, true)
	const msg = "推送下载器失败 1 条: 密码过期"
	if err := db.Model(&models.BrushFlowTask{}).Where("id = ?", 3).
		Updates(map[string]any{"last_error": msg, "last_error_from": "select"}).Error; err != nil {
		t.Fatal(err)
	}
	out, err := brushflowRunOutcome(context.Background(), bf, 3, "select", "selected",
		func(context.Context, uint) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := out["task_error"].(string); got != msg {
		t.Errorf("旧诊断仍应透出（用户要看到未结案），got %+v", out)
	}
	if _, ok := out["task_error_new"]; ok {
		t.Errorf("内容没变化时不得标 task_error_new，got %+v", out)
	}
}

func TestBrushflowRunOutcomeCleanRun(t *testing.T) {
	bf, db := newBrushFlowFixture(t)
	seedTask(t, db, 4, true)
	out, err := brushflowRunOutcome(context.Background(), bf, 4, "sweep", "swept",
		func(context.Context, uint) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if out["swept"] != 1 {
		t.Fatalf("干净一轮应只带回计数，got %+v", out)
	}
	if _, ok := out["task_error"]; ok {
		t.Errorf("无诊断时不该有无中生有，got %+v", out)
	}
	if _, err := brushflowRunOutcome(context.Background(), bf, 0, "select", "selected",
		func(context.Context, uint) error { return nil }); err == nil {
		t.Error("task_id=0 必须报错")
	}
}

// TestBrushflowToolsSurfaceDisabledTask 走工具面（agent + plugin）确认接线：
// 光有 helper 不够，注册表里的 handler 也得真的用它。
func TestBrushflowToolsSurfaceDisabledTask(t *testing.T) {
	bf, db := newBrushFlowFixture(t)
	seedTask(t, db, 9, false)
	ctx := context.Background()

	var agentTool mcp.Tool
	for _, tl := range buildBrushFlowTools(bf) {
		if tl.Name == "brushflow_run_sweep" {
			agentTool = tl
		}
	}
	if agentTool.Name == "" {
		t.Fatal("agent 面没有 brushflow_run_sweep")
	}
	if got, err := agentTool.Handler(ctx, map[string]any{"task_id": float64(9)}); err == nil {
		t.Errorf("agent 工具必须拒绝未启用任务，got %s", got)
	} else if !strings.Contains(err.Error(), "未启用") {
		t.Errorf("agent 工具错误应说明未启用，got %v", err)
	}

	reg := plugins.NewHostToolRegistry()
	registerBrushFlowTools(reg, bf)
	rt, ok := reg.Lookup("brushflow_run_select")
	if !ok {
		t.Fatal("plugin 面没有 brushflow_run_select")
	}
	if got, err := rt.Handler(ctx, map[string]any{"task_id": float64(9)}); err == nil {
		t.Errorf("plugin 工具必须拒绝未启用任务，got %+v", got)
	} else if !strings.Contains(err.Error(), "未启用") {
		t.Errorf("plugin 工具错误应说明未启用，got %v", err)
	}
}
