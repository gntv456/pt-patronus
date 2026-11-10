// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"gorm.io/gorm"
)

// newFailingChain 一条会失败的工作流链 + 真通知库（notice 传 nil 时失败自曝整段被跳过）。
func newFailingChain(t *testing.T) (*WorkflowChain, *notice.Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wf_notify.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Workflow{}, &models.WorkflowRun{}, &models.Notice{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewWorkflowOper(db)
	runOp := oper.NewWorkflowRunOper(db)
	reg := workflow.NewRegistry()
	reg.Register(echoN{typ: "t_fail", fail: true})
	ns := notice.NewService(oper.NewNoticeOper(db), nil, nil)
	return NewWorkflowChain(Base{}, op, runOp, reg, nil, nil, ns), ns
}

// TestWorkflowFailureNoticeDedupPerDay 回归：失败通知按「工作流+当天」去重。
// 分钟级定时器一旦坏了，每轮一条同名 warning 一天能刷 1440 条，用户会直接关掉通知——
// 那比漏报更糟；逐轮原因完整留在 workflow_run 里，不靠通知堆量。
func TestWorkflowFailureNoticeDedupPerDay(t *testing.T) {
	c, ns := newFailingChain(t)
	nodes := mustJSON(t, []workflow.GraphNode{{ID: "n1", Type: "t_fail", Name: "失败节点"}})
	wf := &models.Workflow{Name: "定时失败流", TriggerType: "timer", State: "idle", Nodes: nodes, Edges: "[]"}
	created, err2 := c.Create(wf)
	if err2 != nil {
		t.Fatal(err2)
	}

	for i := 0; i < 3; i++ {
		if _, err := c.Run(context.Background(), created.ID, "timer", nil); err == nil {
			t.Fatalf("第 %d 轮应失败", i+1)
		}
	}

	all, err := ns.List(false, 50)
	if err != nil {
		t.Fatal(err)
	}
	hits := 0
	var key string
	for _, n := range all {
		if n.Source == notice.SourceWorkflow {
			hits++
			key = n.Key
		}
	}
	if hits != 1 {
		t.Fatalf("3 轮失败应只发 1 条当天去重通知，got %d 条（key=%q）", hits, key)
	}
	if !strings.Contains(key, "fail:") || !strings.Contains(key, strconv.Itoa(int(created.ID))) {
		t.Fatalf("去重键应按工作流分桶，got %q", key)
	}
}
