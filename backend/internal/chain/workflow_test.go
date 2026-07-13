package chain

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"gorm.io/gorm"
)

func TestEvaluateEventConditionsFailsClosed(t *testing.T) {
	data := map[string]any{"state": "completed"}
	tests := []struct {
		name string
		raw  string
	}{
		{"malformed json", `[{`},
		{"unknown operator", `[{"field":"state","operator":"oops","value":"completed"}]`},
		{"invalid regex", `[{"field":"state","operator":"regex","value":"["}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if evaluateEventConditions(tt.raw, data) {
				t.Fatal("invalid event condition must not trigger a workflow")
			}
		})
	}
}

func TestEvaluateEventConditionsMatchesSupportedOperators(t *testing.T) {
	data := map[string]any{"state": "completed", "title": "Ubuntu 24.04"}
	for _, raw := range []string{
		`[{"field":"state","operator":"eq","value":"completed"}]`,
		`[{"field":"state","operator":"ne","value":"failed"}]`,
		`[{"field":"title","operator":"contains","value":"24.04"}]`,
		`[{"field":"title","operator":"regex","value":"^Ubuntu"}]`,
	} {
		if !evaluateEventConditions(raw, data) {
			t.Fatalf("expected condition to match: %s", raw)
		}
	}
}

func TestValidateWorkflowTrigger(t *testing.T) {
	valid := []*models.Workflow{
		{TriggerType: "manual"},
		{TriggerType: "timer", Timer: "*/15 * * * *"},
		{TriggerType: "event", EventType: "torrent.completed", EventConditions: `[{"field":"state","operator":"eq","value":"completed"}]`},
	}
	for _, wf := range valid {
		if err := validateWorkflowTrigger(wf); err != nil {
			t.Fatalf("valid trigger rejected: %v", err)
		}
	}

	invalid := []*models.Workflow{
		{TriggerType: "timer", Timer: "not a cron"},
		{TriggerType: "event"},
		{TriggerType: "event", EventType: "x", EventConditions: `[{"field":"x","operator":"regex","value":"["}]`},
		{TriggerType: "unknown"},
	}
	for _, wf := range invalid {
		if err := validateWorkflowTrigger(wf); err == nil {
			t.Fatalf("invalid trigger accepted: %+v", wf)
		}
	}
}

// TestEventWorkflowResubscribesOnEventTypeChange 验证：用户修改事件工作流的 EventType 后，
// 新类型会被重新订阅（能触发），旧类型的残留订阅会因 EventType 不匹配而自废（不误触发）。
// 回归：原先 eventSubscribed 仅按 wf.ID 去重 → 改 EventType 后新类型永不订阅、旧类型仍误触发。
func TestEventWorkflowResubscribesOnEventTypeChange(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wf_evt.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&models.Workflow{}, &models.WorkflowRun{}); err != nil {
		t.Fatal(err)
	}

	op := oper.NewWorkflowOper(db)
	runOp := oper.NewWorkflowRunOper(db)
	bus := event.New()
	c := NewWorkflowChain(Base{}, op, runOp, workflow.NewRegistry(), nil, bus, nil)

	wf := &models.Workflow{
		Name: "evt", TriggerType: "event", EventType: "webhook",
		Enabled: true, State: "idle", // 空 Nodes/Edges：Run 立即成功结束
	}
	if err := op.Create(wf); err != nil {
		t.Fatal(err)
	}
	if err := c.RegisterTriggers(wf); err != nil {
		t.Fatal(err)
	}

	// 模拟 Update 改 EventType：UnregisterTriggers（event 无操作）+ RegisterTriggers(new)
	wf.EventType = "download.completed"
	if err := op.Update(wf); err != nil {
		t.Fatal(err)
	}
	c.UnregisterTriggers(wf)
	if err := c.RegisterTriggers(wf); err != nil {
		t.Fatal(err)
	}

	runCount := func() int {
		got, err := op.Get(wf.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.RunCount
	}

	// 新事件类型必须触发（证明改类型后重新订阅成功）
	bus.Publish(context.Background(), "download.completed", map[string]any{"k": "v"})
	if rc := runCount(); rc != 1 {
		t.Fatalf("改 EventType 后新类型应触发一次，run_count=%d", rc)
	}

	// 旧事件类型的残留订阅必须自废，不再误触发
	bus.Publish(context.Background(), "webhook", map[string]any{"k": "v"})
	if rc := runCount(); rc != 1 {
		t.Fatalf("旧 EventType 残留订阅不应触发，run_count=%d", rc)
	}
}
