package chain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/scheduler"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"gorm.io/gorm"
)

// --- 测试基础设施 ---------------------------------------------------------

// echoN 测试节点：追加固定消息，可选失败。用于 chain.Run 端到端验证。
type echoN struct {
	typ  string
	msg  string
	fail bool
}

func (n echoN) Type() string                     { return n.typ }
func (echoN) Name() string                       { return "回显" }
func (echoN) Category() string                   { return "测试" }
func (echoN) Description() string                { return "test" }
func (echoN) Icon() string                       { return "test" }
func (echoN) DefaultParams() map[string]any      { return nil }
func (echoN) ParamSchema() []workflow.ParamField { return nil }
func (n echoN) Execute(_ context.Context, _ map[string]any, ecx *workflow.Context) error {
	if n.fail {
		return errBoom
	}
	ecx.Messages = append(ecx.Messages, n.msg)
	return nil
}

var errBoom = strErr("boom")

type strErr string

func (e strErr) Error() string { return string(e) }

// panicN 测试节点：Execute 直接 panic，模拟节点实现 bug（如 nil 指针解引用）。
// 用于验证 WorkflowChain.Run 的 defer recover 兜底：panic 后 wf.State 必须置 failed 而非永久卡 running。
type panicN struct{ typ string }

func (n panicN) Type() string                     { return n.typ }
func (panicN) Name() string                       { return "panic" }
func (panicN) Category() string                   { return "测试" }
func (panicN) Description() string                { return "test panic" }
func (panicN) Icon() string                       { return "test" }
func (panicN) DefaultParams() map[string]any      { return nil }
func (panicN) ParamSchema() []workflow.ParamField { return nil }
func (panicN) Execute(_ context.Context, _ map[string]any, _ *workflow.Context) error {
	panic("node boom")
}

// newChainTB 构造一个接真实 sqlite 的 WorkflowChain，registry 预注册给定节点。
// scheduler/bus 可传 nil；传 nil 时 timer/event 触发器相关测试不可用。
func newChainTB(t *testing.T, sched cronScheduler, bus *event.Bus, nodes ...workflow.Node) (
	*WorkflowChain, *oper.WorkflowOper, *oper.WorkflowRunOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wf_chain.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.Workflow{}, &models.WorkflowRun{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewWorkflowOper(db)
	runOp := oper.NewWorkflowRunOper(db)
	reg := workflow.NewRegistry()
	for _, n := range nodes {
		reg.Register(n)
	}
	c := NewWorkflowChain(Base{}, op, runOp, reg, sched, bus, nil)
	return c, op, runOp
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- Run 端到端 -----------------------------------------------------------

// TestChainRunSuccessSuccessState 端到端：解析 JSON 图 → executor 执行 → 落 WorkflowRun 终态 +
// MarkRun 累加 run_count/state + NodeExecutions 日志序列化。覆盖 chain.Run 整段执行路径。
func TestChainRunSuccessSuccessState(t *testing.T) {
	c, op, runOp := newChainTB(t, nil, nil, echoN{typ: "t_echo", msg: "hi"})
	nodes := mustJSON(t, []workflow.GraphNode{{ID: "n1", Type: "t_echo", Name: "打招呼"}})
	wf := &models.Workflow{Name: "成功流", TriggerType: "manual", State: "idle", Nodes: nodes, Edges: "[]"}
	if _, err := c.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	run, err := c.Run(context.Background(), wf.ID, "manual", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.State != "success" {
		t.Fatalf("run.State=%s want success", run.State)
	}
	if run.Trigger != "manual" {
		t.Fatalf("run.Trigger=%s want manual", run.Trigger)
	}
	if !strings.Contains(run.Result, "hi") {
		t.Fatalf("run.Result 应含消息 hi，got %q", run.Result)
	}
	if run.NodeExecutions == "" {
		t.Fatal("NodeExecutions 不应为空")
	}
	var entries []workflow.LogEntry
	if err := json.Unmarshal([]byte(run.NodeExecutions), &entries); err != nil {
		t.Fatalf("NodeExecutions 非合法 LogEntry JSON: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != "success" {
		t.Fatalf("应含 1 条 success 日志，got %+v", entries)
	}

	// MarkRun 累加：run_count=1、state=success、last_run_at 非空。
	got, _ := op.Get(wf.ID)
	if got.RunCount != 1 {
		t.Fatalf("RunCount=%d want 1", got.RunCount)
	}
	if got.State != "success" {
		t.Fatalf("wf.State=%s want success", got.State)
	}
	if got.LastRunAt == nil {
		t.Fatal("LastRunAt 不应为空")
	}
	runs, _ := runOp.ListByWorkflow(wf.ID, 10)
	if len(runs) != 1 {
		t.Fatalf("运行历史应 1 条，got %d", len(runs))
	}
}

// TestChainRunFailedSetsStateAndResult 端到端失败：节点失败 → run.State=failed、result 含错误、wf 终态 failed。
func TestChainRunFailedSetsStateAndResult(t *testing.T) {
	c, op, _ := newChainTB(t, nil, nil, echoN{typ: "t_fail", fail: true})
	nodes := mustJSON(t, []workflow.GraphNode{{ID: "n1", Type: "t_fail", Name: "失败节点"}})
	wf := &models.Workflow{Name: "失败流", TriggerType: "manual", State: "idle", Nodes: nodes, Edges: "[]"}
	_, _ = c.Create(wf)

	run, err := c.Run(context.Background(), wf.ID, "manual", nil)
	if err == nil {
		t.Fatal("失败节点应返回错误")
	}
	if run.State != "failed" {
		t.Fatalf("run.State=%s want failed", run.State)
	}
	if !strings.Contains(run.Result, "boom") {
		t.Fatalf("result 应含错误信息，got %q", run.Result)
	}
	got, _ := op.Get(wf.ID)
	if got.State != "failed" {
		t.Fatalf("wf.State=%s want failed", got.State)
	}
}

// TestChainRunPanicSetsFailedState 回归：节点 Execute panic 曾让 wf.State 永久卡在 running
// （panic 跳过末尾 MarkRun/Update，二者非 defer）。修复后 defer recover 兜底写 failed 终态，
// 且 Run 把 panic 转为错误返回（不逃逸出函数），run 记录同样置 failed。
func TestChainRunPanicSetsFailedState(t *testing.T) {
	c, op, _ := newChainTB(t, nil, nil, panicN{typ: "t_panic"})
	nodes := mustJSON(t, []workflow.GraphNode{{ID: "n1", Type: "t_panic", Name: "panic节点"}})
	wf := &models.Workflow{Name: "panic流", TriggerType: "manual", State: "idle", Nodes: nodes, Edges: "[]"}
	if _, err := c.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	run, err := c.Run(context.Background(), wf.ID, "manual", nil)
	if err == nil {
		t.Fatal("panic 节点应被 recover 并返回错误，不应逃逸")
	}
	if run == nil {
		t.Fatal("panic 后 run 记录不应为 nil")
	}
	if run.State != "failed" {
		t.Fatalf("run.State=%s want failed", run.State)
	}
	if !strings.Contains(run.Result, "node boom") {
		t.Fatalf("run.Result 应含 panic 信息，got %q", run.Result)
	}
	got, _ := op.Get(wf.ID)
	if got.State == "running" {
		t.Fatal("panic 让工作流卡 running（回归缺陷），应为 failed")
	}
	if got.State != "failed" {
		t.Fatalf("panic 后 wf.State=%s want failed", got.State)
	}
	// 重入守卫不再被卡：panic 后状态已 failed，可再次触发（应再次 panic→failed，而非「执行中」拒绝）。
	_, err = c.Run(context.Background(), wf.ID, "manual", nil)
	if err == nil || strings.Contains(err.Error(), "执行中") {
		t.Fatalf("panic 后应允许重跑而非被重入守卫挡住，got err=%v", err)
	}
}

// TestChainRunReentryGuard 重入守卫：工作流 State==running 时拒绝再次执行，run_count 不增。
func TestChainRunReentryGuard(t *testing.T) {
	c, op, _ := newChainTB(t, nil, nil, echoN{typ: "t_echo", msg: "hi"})
	nodes := mustJSON(t, []workflow.GraphNode{{ID: "n1", Type: "t_echo", Name: "打招呼"}})
	wf := &models.Workflow{Name: "守卫流", TriggerType: "manual", Nodes: nodes, Edges: "[]"}
	if _, err := c.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// 模拟"正在执行"：用生产同款 SetState 置 running。
	op.SetState(wf.ID, "running")

	_, err := c.Run(context.Background(), wf.ID, "manual", nil)
	if err == nil || !strings.Contains(err.Error(), "执行中") {
		t.Fatalf("应被重入守卫拒绝，got err=%v", err)
	}
	got, _ := op.Get(wf.ID)
	if got.RunCount != 0 {
		t.Fatalf("被拒不应累加 run_count，got %d", got.RunCount)
	}
}

// TestChainRunEventPayloadSeedsVariables 事件触发：trigger 写入 run 记录（eventData 种子由节点消费）。
func TestChainRunEventPayloadSeedsVariables(t *testing.T) {
	c, op, _ := newChainTB(t, nil, nil, echoN{typ: "t_echo", msg: "ok"})
	wf := &models.Workflow{Name: "事件流", TriggerType: "event", EventType: "x", State: "idle"}
	op.Create(wf)

	run, err := c.Run(context.Background(), wf.ID, "event", map[string]any{"title": "T1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.Trigger != "event" {
		t.Fatalf("Trigger=%s want event", run.Trigger)
	}
}

// --- Create 校验拒绝 -----------------------------------------------------

// TestCreateRejectsBadCron enabled timer 工作流 cron 非法 → Create 拒绝且不落库。
func TestCreateRejectsBadCron(t *testing.T) {
	c, op, _ := newChainTB(t, scheduler.New(), nil)
	wf := &models.Workflow{Name: "坏cron", TriggerType: "timer", Timer: "not a cron", Enabled: true, State: "idle"}
	if _, err := c.Create(wf); err == nil {
		t.Fatal("非法 cron 应被拒绝")
	}
	list, _ := op.List()
	if len(list) != 0 {
		t.Fatalf("被拒工作流不应落库，got %d 条", len(list))
	}
}

// TestCreateRejectsBadGraph 图含未知节点类型 → Create 拒绝（validateGraph 前置）。
func TestCreateRejectsBadGraph(t *testing.T) {
	c, op, _ := newChainTB(t, nil, nil)
	nodes := mustJSON(t, []workflow.GraphNode{{ID: "n1", Type: "ghost_type", Name: "幽灵"}})
	wf := &models.Workflow{Name: "坏图", TriggerType: "manual", Nodes: nodes, Edges: "[]", State: "idle"}
	if _, err := c.Create(wf); err == nil {
		t.Fatal("未知节点类型应被拒绝")
	}
	list, _ := op.List()
	if len(list) != 0 {
		t.Fatalf("坏图工作流不应落库，got %d 条", len(list))
	}
}

// TestCreateRejectsCycle 图含环 → Create 拒绝。
func TestCreateRejectsCycle(t *testing.T) {
	c, op, _ := newChainTB(t, nil, nil, echoN{typ: "t_echo"})
	nodes := mustJSON(t, []workflow.GraphNode{{ID: "a", Type: "t_echo", Name: "A"}, {ID: "b", Type: "t_echo", Name: "B"}})
	edges := mustJSON(t, []workflow.GraphEdge{{ID: "e1", Source: "a", Target: "b"}, {ID: "e2", Source: "b", Target: "a"}})
	wf := &models.Workflow{Name: "环", TriggerType: "manual", Nodes: nodes, Edges: edges, State: "idle"}
	if _, err := c.Create(wf); err == nil {
		t.Fatal("含环工作流应被拒绝")
	}
	list, _ := op.List()
	if len(list) != 0 {
		t.Fatalf("含环工作流不应落库，got %d 条", len(list))
	}
}

// --- timer 触发器注册/注销 ------------------------------------------------

// TestTimerTriggerRegisterAndUnregister 合法 cron 的 enabled timer 工作流 Create 成功
// （间接证明 sched.AddFunc 成功，否则 Create 会回滚返回错误）；UnregisterTriggers 幂等不 panic。
func TestTimerTriggerRegisterAndUnregister(t *testing.T) {
	sched := scheduler.New()
	c, _, _ := newChainTB(t, sched, nil, echoN{typ: "t_echo", msg: "hi"})
	nodes := mustJSON(t, []workflow.GraphNode{{ID: "n1", Type: "t_echo", Name: "打招呼"}})
	wf := &models.Workflow{Name: "timer", TriggerType: "timer", Timer: "*/5 * * * *", Enabled: true, State: "idle", Nodes: nodes, Edges: "[]"}
	if _, err := c.Create(wf); err != nil {
		t.Fatalf("合法 cron 注册应成功，got %v", err)
	}
	// 注销幂等：连续两次不 panic。
	c.UnregisterTriggers(wf)
	c.UnregisterTriggers(wf)
}

// TestRegisterAllTriggersOnStartup 启动重建：op 直接落库的 enabled 工作流，经 RegisterAllTriggers
// 后 event 类被订阅可触发（验证 ListEnabled 遍历 + RegisterTriggers 的启动恢复链）。
func TestRegisterAllTriggersOnStartup(t *testing.T) {
	bus := event.New()
	c, op, _ := newChainTB(t, scheduler.New(), bus, echoN{typ: "t_echo", msg: "hi"})
	// 直接 op.Create 绕过 chain 的 RegisterTriggers，模拟"DB 已有工作流但触发器随重启丢失"。
	wf := &models.Workflow{Name: "event", TriggerType: "event", EventType: "download.completed", Enabled: true, State: "idle"}
	op.Create(wf)

	if err := c.RegisterAllTriggers(context.Background()); err != nil {
		t.Fatalf("RegisterAllTriggers: %v", err)
	}
	bus.Publish(context.Background(), "download.completed", map[string]any{"k": "v"})
	// Publish 并发扇出 + wg.Wait，返回时 handler 已完成；校验 run_count 已增。
	got, _ := op.Get(wf.ID)
	if got.RunCount != 1 {
		t.Fatalf("启动注册后事件应触发一次，run_count=%d", got.RunCount)
	}
}

// --- 事件条件 -------------------------------------------------------------

// TestEvaluateEventConditionsEmptyPassesEmptyAndMissing 空条件放行；字段缺失不放行。
func TestEvaluateEventConditionsEmptyPassesEmptyAndMissing(t *testing.T) {
	if !evaluateEventConditions("", map[string]any{"a": "b"}) {
		t.Fatal("空条件应放行")
	}
	if !evaluateEventConditions("   ", map[string]any{"a": "b"}) {
		t.Fatal("空白条件应放行")
	}
	// 条件字段在 data 中不存在 → 不放行（fails-closed）。
	if evaluateEventConditions(`[{"field":"absent","operator":"eq","value":"x"}]`, map[string]any{"a": "b"}) {
		t.Fatal("缺失字段应不放行")
	}
}

// --- Stats 聚合 -----------------------------------------------------------

// TestStatsAggregation 统计看板聚合：成功率、per-node NodeTiming、RecentFailures 截断。
func TestStatsAggregation(t *testing.T) {
	c, op, runOp := newChainTB(t, nil, nil)
	// 造 1 个工作流 + 3 条 run（2 成功 1 失败）。
	wf := &models.Workflow{Name: "统计", TriggerType: "manual", Enabled: true, State: "idle"}
	op.Create(wf)

	mkRun := func(state string, dur int64, logEntries []workflow.LogEntry) {
		b, _ := json.Marshal(logEntries)
		runOp.Create(&models.WorkflowRun{
			WorkflowID: wf.ID, State: state, Trigger: "manual",
			StartedAt: time.Now(), DurationMS: dur, NodeExecutions: string(b),
		})
	}
	mkRun("success", 100, []workflow.LogEntry{
		{NodeID: "n1", NodeType: "t_echo", NodeName: "回显", Status: "success", DurationMS: 100},
	})
	mkRun("success", 200, []workflow.LogEntry{
		{NodeID: "n1", NodeType: "t_echo", Status: "success", DurationMS: 200},
	})
	mkRun("failed", 50, []workflow.LogEntry{
		{NodeID: "n2", NodeType: "t_fail", Status: "failed", DurationMS: 50},
	})

	st, err := c.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Total != 1 || st.Enabled != 1 {
		t.Fatalf("Total=%d Enabled=%d want 1/1", st.Total, st.Enabled)
	}
	if st.RunsTotal != 3 || st.RunsSuccess != 2 || st.RunsFailed != 1 {
		t.Fatalf("runs total/succ/fail=%d/%d/%d want 3/2/1", st.RunsTotal, st.RunsSuccess, st.RunsFailed)
	}
	if st.AvgDurationMS != (100+200+50)/3 {
		t.Fatalf("AvgDurationMS=%d want %d", st.AvgDurationMS, (100+200+50)/3)
	}
	// NodeTiming 按 NodeType 聚合：t_echo count2 fail0、t_fail count1 fail1。
	timingByType := map[string]NodeTiming{}
	for _, nt := range st.NodeTiming {
		timingByType[nt.NodeType] = nt
	}
	if te, ok := timingByType["t_echo"]; !ok || te.Count != 2 || te.Failed != 0 || te.TotalMS != 300 {
		t.Fatalf("t_echo timing 异常: %+v", te)
	}
	if tf, ok := timingByType["t_fail"]; !ok || tf.Count != 1 || tf.Failed != 1 {
		t.Fatalf("t_fail timing 异常: %+v", tf)
	}
	// RecentFailures 含 1 条失败。
	if len(st.RecentFailures) != 1 {
		t.Fatalf("RecentFailures 应 1 条，got %d", len(st.RecentFailures))
	}
}

// TestStatsEmptyNoRecentFailures 空库时 Stats 不 panic、NodeTiming/RecentFailures 为空切片（非 nil，防前端迭代崩溃）。
func TestStatsEmptyNoRecentFailures(t *testing.T) {
	c, _, _ := newChainTB(t, nil, nil)
	st, err := c.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.NodeTiming == nil || st.RecentFailures == nil {
		t.Fatal("空切片不应为 nil（前端 .map 会崩）")
	}
	if len(st.NodeTiming) != 0 || len(st.RecentFailures) != 0 {
		t.Fatalf("空库 NodeTiming/RecentFailures 应为空")
	}
}

// failScheduler 模拟 AddFunc 总是失败的 scheduler，用于验证 Create 在 RegisterTriggers 失败时回滚。
type failScheduler struct{ err error }

func (f failScheduler) AddFunc(name, spec string, fn func()) error { return f.err }
func (failScheduler) Remove(name string)                           {}

// TestCreateRollbackOnTriggerFailure enabled timer 工作流在 RegisterTriggers(AddFunc) 失败时，
// Create 回滚已落库记录并返回错误。修复前该路径对合法输入不可达（合法 cron 的 AddFunc 几乎不失败），
// 靠 scheduler 接口化 + 故障注入才可覆盖。
func TestCreateRollbackOnTriggerFailure(t *testing.T) {
	c, op, _ := newChainTB(t, failScheduler{err: errBoom}, nil, echoN{typ: "t_echo", msg: "hi"})
	nodes := mustJSON(t, []workflow.GraphNode{{ID: "n1", Type: "t_echo", Name: "打招呼"}})
	wf := &models.Workflow{
		Name: "回滚", TriggerType: "timer", Timer: "*/5 * * * *",
		Enabled: true, State: "idle", Nodes: nodes, Edges: "[]",
	}
	if _, err := c.Create(wf); err == nil {
		t.Fatal("AddFunc 失败时 Create 应回滚并返回错误")
	}
	list, _ := op.List()
	if len(list) != 0 {
		t.Fatalf("回滚后工作流不应残留，got %d 条", len(list))
	}
}
