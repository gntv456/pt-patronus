package oper

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// wfOperDB 构造一个接真实 sqlite 的 DB，迁移 Workflow/WorkflowRun 表。
func wfOperDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wf_oper.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.Workflow{}, &models.WorkflowRun{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// TestWorkflowCRUD Create→Get→List→Update→Delete 全流程。
func TestWorkflowCRUD(t *testing.T) {
	db := wfOperDB(t)
	op := NewWorkflowOper(db)

	wf := &models.Workflow{Name: "crud", TriggerType: "manual", State: "idle", Enabled: true}
	if err := op.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wf.ID == 0 {
		t.Fatal("Create 后 ID 应非零")
	}
	got, err := op.Get(wf.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "crud" {
		t.Fatalf("Get Name=%q", got.Name)
	}
	list, _ := op.List()
	if len(list) != 1 {
		t.Fatalf("List 应 1 条，got %d", len(list))
	}

	got.Description = "changed"
	if err := op.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got2, _ := op.Get(wf.ID)
	if got2.Description != "changed" {
		t.Fatalf("Update 未生效，Description=%q", got2.Description)
	}
	if err := op.Delete(wf.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := op.Get(wf.ID); err == nil {
		t.Fatal("删除后 Get 应报错")
	}
}

// TestSetState SetState 用 map 形式写，绕过 GORM 零值坑（state 可置空串语义）。
func TestSetState(t *testing.T) {
	db := wfOperDB(t)
	op := NewWorkflowOper(db)
	wf := &models.Workflow{Name: "s", State: "idle"}
	op.Create(wf)

	for _, st := range []string{"running", "success", "failed", "idle"} {
		if err := op.SetState(wf.ID, st); err != nil {
			t.Fatalf("SetState(%s): %v", st, err)
		}
		got, _ := op.Get(wf.ID)
		if got.State != st {
			t.Fatalf("SetState(%s) 后 State=%q", st, got.State)
		}
	}
}

// TestMarkRunIncrementsRunCount MarkRun 用 gorm.Expr 自增 run_count（绕零值坑），多次累加 + 更新 state/result。
func TestMarkRunIncrementsRunCount(t *testing.T) {
	db := wfOperDB(t)
	op := NewWorkflowOper(db)
	wf := &models.Workflow{Name: "mk", State: "idle"}
	op.Create(wf)

	if err := op.MarkRun(wf.ID, "success", "ok1"); err != nil {
		t.Fatalf("MarkRun: %v", err)
	}
	got, _ := op.Get(wf.ID)
	if got.RunCount != 1 || got.State != "success" || got.LastResult != "ok1" || got.LastRunAt == nil {
		t.Fatalf("首次 MarkRun 后状态异常: %+v", got)
	}
	op.MarkRun(wf.ID, "failed", "boom")
	got2, _ := op.Get(wf.ID)
	if got2.RunCount != 2 || got2.State != "failed" || got2.LastResult != "boom" {
		t.Fatalf("二次 MarkRun 后状态异常: %+v", got2)
	}
}

// TestResetStuckRunning 把 state=running 的工作流重置为 failed（启动恢复），idle 等其它状态不变。
func TestResetStuckRunning(t *testing.T) {
	db := wfOperDB(t)
	op := NewWorkflowOper(db)
	stuck := &models.Workflow{Name: "stuck", State: "idle"}
	idle := &models.Workflow{Name: "idle", State: "idle"}
	ok := &models.Workflow{Name: "ok", State: "success"}
	op.Create(stuck)
	op.Create(idle)
	op.Create(ok)
	op.SetState(stuck.ID, "running")

	if err := op.ResetStuckRunning(); err != nil {
		t.Fatalf("ResetStuckRunning: %v", err)
	}
	gotStuck, _ := op.Get(stuck.ID)
	if gotStuck.State != "failed" {
		t.Fatalf("running 应被重置为 failed，got %q", gotStuck.State)
	}
	for _, w := range []struct {
		id   uint
		want string
	}{{idle.ID, "idle"}, {ok.ID, "success"}} {
		got, _ := op.Get(w.id)
		if got.State != w.want {
			t.Fatalf("非 running 状态不应被改：id=%d got %q want %q", w.id, got.State, w.want)
		}
	}
}

// TestResetStateClearsStatus ResetState 清 state/last_result 但不动 run_count/last_run_at（与现有回归测试互补）。
func TestResetStateClearsStatus(t *testing.T) {
	db := wfOperDB(t)
	op := NewWorkflowOper(db)
	wf := &models.Workflow{Name: "rs", State: "idle"}
	op.Create(wf)
	op.MarkRun(wf.ID, "failed", "boom")

	if err := op.ResetState(wf.ID); err != nil {
		t.Fatalf("ResetState: %v", err)
	}
	got, _ := op.Get(wf.ID)
	if got.State != "idle" {
		t.Fatalf("State=%q want idle", got.State)
	}
	if got.LastResult != "" {
		t.Fatalf("LastResult 应清空，got %q", got.LastResult)
	}
	if got.RunCount != 1 {
		t.Fatalf("ResetState 不应动 run_count，got %d", got.RunCount)
	}
}

// TestListEnabled ListEnabled 只返回 Enabled=true 的工作流。
func TestListEnabled(t *testing.T) {
	db := wfOperDB(t)
	op := NewWorkflowOper(db)
	op.Create(&models.Workflow{Name: "on", Enabled: true, State: "idle"})
	op.Create(&models.Workflow{Name: "off", Enabled: false, State: "idle"})
	op.Create(&models.Workflow{Name: "on2", Enabled: true, State: "idle"})

	enabled, _ := op.ListEnabled()
	if len(enabled) != 2 {
		t.Fatalf("ListEnabled 应 2 条，got %d", len(enabled))
	}
	for _, w := range enabled {
		if !w.Enabled {
			t.Fatalf("ListEnabled 不应返回禁用工作流：%s", w.Name)
		}
	}
}

// TestDeleteCascadesRuns Delete 在事务内级联删除该工作流的 WorkflowRun 记录。
func TestDeleteCascadesRuns(t *testing.T) {
	db := wfOperDB(t)
	op := NewWorkflowOper(db)
	runOp := NewWorkflowRunOper(db)
	wf := &models.Workflow{Name: "casc", State: "idle"}
	op.Create(wf)
	runOp.Create(&models.WorkflowRun{WorkflowID: wf.ID, State: "success", StartedAt: time.Now()})
	runOp.Create(&models.WorkflowRun{WorkflowID: wf.ID, State: "failed", StartedAt: time.Now()})

	runs, _ := runOp.ListByWorkflow(wf.ID, 100)
	if len(runs) != 2 {
		t.Fatalf("删除前应 2 条 run，got %d", len(runs))
	}
	if err := op.Delete(wf.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	runsAfter, _ := runOp.ListByWorkflow(wf.ID, 100)
	if len(runsAfter) != 0 {
		t.Fatalf("Delete 应级联删除 run，残留 %d 条", len(runsAfter))
	}
}

// TestWorkflowRunOperLifecycle Run 记录 Create/Get/ListByWorkflow/ListRecent/Cleanup 全流程。
func TestWorkflowRunOperLifecycle(t *testing.T) {
	db := wfOperDB(t)
	op := NewWorkflowOper(db)
	runOp := NewWorkflowRunOper(db)
	wf := &models.Workflow{Name: "lc", State: "idle"}
	op.Create(wf)

	r1 := &models.WorkflowRun{WorkflowID: wf.ID, State: "success", Trigger: "manual", StartedAt: time.Now()}
	r2 := &models.WorkflowRun{WorkflowID: wf.ID, State: "failed", Trigger: "timer", StartedAt: time.Now()}
	runOp.Create(r1)
	runOp.Create(r2)
	if r1.ID == 0 || r2.ID == 0 {
		t.Fatal("run ID 应非零")
	}
	got, _ := runOp.Get(r1.ID)
	if got.State != "success" {
		t.Fatalf("Get run State=%q", got.State)
	}

	byWf, _ := runOp.ListByWorkflow(wf.ID, 100)
	if len(byWf) != 2 {
		t.Fatalf("ListByWorkflow 应 2 条，got %d", len(byWf))
	}
	// 倒序：r2（后创建）在前。
	if byWf[0].ID != r2.ID {
		t.Fatalf("ListByWorkflow 应倒序，首条 id=%d want %d", byWf[0].ID, r2.ID)
	}

	recent, _ := runOp.ListRecent(100)
	if len(recent) != 2 {
		t.Fatalf("ListRecent 应 2 条，got %d", len(recent))
	}

	// Cleanup 删除 1ms 前的记录（两条都创建于此刻之前）。
	n, err := runOp.Cleanup(time.Now().Add(time.Millisecond))
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if n != 2 {
		t.Fatalf("Cleanup 应删 2 条，got %d", n)
	}
	after, _ := runOp.ListRecent(100)
	if len(after) != 0 {
		t.Fatalf("Cleanup 后应无残留，got %d", len(after))
	}
}
