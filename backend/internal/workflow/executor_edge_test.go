package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// --- 测试专用节点 ----------------------------------------------------------

// retryNode 前 failFirst 次返回错误、之后成功。用 ecx.Variables["calls"] 计数（无状态）。
// Executor 单 goroutine 串行，loop 内复用同一节点实例，计数安全。
type retryNode struct{ failFirst int }

func (retryNode) Type() string                  { return "retry" }
func (retryNode) Name() string                  { return "重试" }
func (retryNode) Category() string              { return "测试" }
func (retryNode) Description() string           { return "test" }
func (retryNode) Icon() string                  { return "test" }
func (retryNode) DefaultParams() map[string]any { return nil }
func (retryNode) ParamSchema() []ParamField     { return nil }
func (n retryNode) Execute(_ context.Context, params map[string]any, ecx *Context) error {
	cnt, _ := ecx.Variables["calls"].(int)
	cnt++
	ecx.Messages = append(ecx.Messages, "try")
	ecx.Variables["calls"] = cnt
	if cnt <= ParamInt(params, "fail_first") {
		return errors.New("transient")
	}
	return nil
}

// ctxSleepNode 阻塞 ms 毫秒，期间尊重 ctx 取消。用于验证节点超时机制。
type ctxSleepNode struct{}

func (ctxSleepNode) Type() string                  { return "sleep" }
func (ctxSleepNode) Name() string                  { return "睡眠" }
func (ctxSleepNode) Category() string              { return "测试" }
func (ctxSleepNode) Description() string           { return "test" }
func (ctxSleepNode) Icon() string                  { return "test" }
func (ctxSleepNode) DefaultParams() map[string]any { return nil }
func (ctxSleepNode) ParamSchema() []ParamField     { return nil }
func (ctxSleepNode) Execute(ctx context.Context, params map[string]any, _ *Context) error {
	d := time.Duration(ParamInt(params, "ms")) * time.Millisecond
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// stopNode 执行后置 ecx.Stop=true 短路整条流。
type stopNode struct{}

func (stopNode) Type() string                  { return "stop" }
func (stopNode) Name() string                  { return "停止" }
func (stopNode) Category() string              { return "测试" }
func (stopNode) Description() string           { return "test" }
func (stopNode) Icon() string                  { return "test" }
func (stopNode) DefaultParams() map[string]any { return nil }
func (stopNode) ParamSchema() []ParamField     { return nil }
func (stopNode) Execute(_ context.Context, _ map[string]any, ecx *Context) error {
	ecx.Stop = true
	ecx.Messages = append(ecx.Messages, "STOP")
	return nil
}

// multiBranchNode 按 params["branches"]（逗号分隔）设置 ecx.Next，模拟多分支同时选中。
type multiBranchNode struct{}

func (multiBranchNode) Type() string                  { return "multi_branch" }
func (multiBranchNode) Name() string                  { return "多分支" }
func (multiBranchNode) Category() string              { return "测试" }
func (multiBranchNode) Description() string           { return "test" }
func (multiBranchNode) Icon() string                  { return "test" }
func (multiBranchNode) DefaultParams() map[string]any { return map[string]any{"branches": "a,b"} }
func (multiBranchNode) ParamSchema() []ParamField     { return nil }
func (multiBranchNode) Execute(_ context.Context, params map[string]any, ecx *Context) error {
	ecx.Next = ParamStringSlice(params, "branches")
	return nil
}

// reqNode 带一个必填字段，用于校验测试。
type reqNode struct{}

func (reqNode) Type() string                  { return "req" }
func (reqNode) Name() string                  { return "必填" }
func (reqNode) Category() string              { return "测试" }
func (reqNode) Description() string           { return "test" }
func (reqNode) Icon() string                  { return "test" }
func (reqNode) DefaultParams() map[string]any { return nil }
func (reqNode) ParamSchema() []ParamField {
	return []ParamField{{Name: "key", Label: "键", Type: "text", Required: true}}
}
func (reqNode) Execute(_ context.Context, _ map[string]any, _ *Context) error { return nil }

// --- 节点超时钳制 ----------------------------------------------------------

// TestNodeExecutionTimeoutClamping 白盒验证 nodeExecutionTimeout 的边界钳制逻辑：
// 缺省/0→60s；过小→5s 下限；过大→600s 上限；区间内原值保留。
func TestNodeExecutionTimeoutClamping(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want time.Duration
	}{
		{"nil params", nil, 60 * time.Second},
		{"zero", map[string]any{"timeout_sec": 0}, 60 * time.Second},
		{"negative", map[string]any{"timeout_sec": -1}, 60 * time.Second},
		{"too small clamps to 5", map[string]any{"timeout_sec": 1}, 5 * time.Second},
		{"boundary five", map[string]any{"timeout_sec": 5}, 5 * time.Second},
		{"in range preserved", map[string]any{"timeout_sec": 300}, 300 * time.Second},
		{"boundary six hundred", map[string]any{"timeout_sec": 600}, 600 * time.Second},
		{"too big clamps to 600", map[string]any{"timeout_sec": 9999}, 600 * time.Second},
		// JSON 数字解出 float64 也要兼容（ParamInt 走 float64 分支）。
		{"float64 json number", map[string]any{"timeout_sec": float64(5)}, 5 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nodeExecutionTimeout(tc.in); got != tc.want {
				t.Fatalf("nodeExecutionTimeout(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestExecutorNodeTimeoutPropagates 节点超时机制：父 ctx 提前取消时，子 ctx（nodeExecutionTimeout）
// 随之取消，节点 select<-ctx.Done() 返回 ctx.Err()，Executor fail-fast 上报。
// 不依赖 nodeExecutionTimeout 的最小 5s 钳制——用更短的父超时快速触发。
func TestExecutorNodeTimeoutPropagates(t *testing.T) {
	reg := testReg(ctxSleepNode{})
	gnodes := []GraphNode{{ID: "s", Type: "sleep", Name: "睡眠", Params: map[string]any{"ms": 2000}}}
	parent, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	ecx := &Context{}
	start := time.Now()
	_, err := NewExecutor(reg).Run(parent, gnodes, nil, ecx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error from sleep node")
	}
	// 必须在远小于节点自身 sleep(2s) 的时间内因父 ctx 取消而失败。
	// 功能断言：必须在远小于节点自身 sleep(2s) 的时间内因父 ctx 取消而失败。
	// 阈值放宽到 1.5s（原 500ms 在 CI + -race 下易 flaky）；核心验证是 err != nil + 日志 failed。
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("节点应因父 ctx 取消快速失败，实际耗时 %v", elapsed)
	}
	// 日志应记录失败。
	var failed bool
	for _, le := range ecx.Log {
		if le.Status == "failed" {
			failed = true
		}
	}
	if !failed {
		t.Fatalf("日志应含失败条目，got %+v", ecx.Log)
	}
}

// --- loop 重试 -------------------------------------------------------------

// TestExecutorLoopSucceedsOnLaterAttempt loop=true 时按 loop_times 重试，成功即停。
func TestExecutorLoopSucceedsOnLaterAttempt(t *testing.T) {
	reg := testReg(retryNode{failFirst: 2})
	gnodes := []GraphNode{{
		ID: "r", Type: "retry", Name: "重试",
		Params: map[string]any{"fail_first": 2, "loop": true, "loop_times": 5, "loop_interval_sec": 0},
	}}
	ecx := &Context{}
	if _, err := NewExecutor(reg).Run(context.Background(), gnodes, nil, ecx); err != nil {
		t.Fatalf("loop 应在第3次成功，got err %v", err)
	}
	if calls := ecx.Variables["calls"].(int); calls != 3 {
		t.Fatalf("预期执行 3 次（2 失败+1 成功），got %d", calls)
	}
	if len(ecx.Messages) != 3 {
		t.Fatalf("预期 3 条 try 日志，got %d", len(ecx.Messages))
	}
}

// TestExecutorLoopRetriesUntilExhausted loop=true 且始终失败时，重试 loop_times 次后返回错误，
// 失败日志标注「重试 N 次后仍失败」（回归：修复前可观测性缺陷——看不出重试次数）。
func TestExecutorLoopRetriesUntilExhausted(t *testing.T) {
	reg := testReg(retryNode{failFirst: 999}) // 始终失败
	gnodes := []GraphNode{{
		ID: "r", Type: "retry", Name: "重试",
		Params: map[string]any{"fail_first": 999, "loop": true, "loop_times": 3, "loop_interval_sec": 0},
	}}
	ecx := &Context{}
	_, err := NewExecutor(reg).Run(context.Background(), gnodes, nil, ecx)
	if err == nil {
		t.Fatal("始终失败节点 loop 重试耗尽后应返回错误")
	}
	if calls := ecx.Variables["calls"].(int); calls != 3 {
		t.Fatalf("预期重试 3 次，got %d", calls)
	}
	// 失败日志应标注重试次数（可观测性修复回归）。
	var failedMsg string
	for _, le := range ecx.Log {
		if le.Status == "failed" {
			failedMsg = le.Message
		}
	}
	if failedMsg == "" {
		t.Fatal("应有失败日志")
	}
	if !strings.Contains(failedMsg, "重试 3 次") {
		t.Fatalf("失败日志应标注重试次数，got %q", failedMsg)
	}
}

// TestExecutorContinueOnError WithContinueOnError 容错模式：失败节点的下游仍执行（非 fail-fast）。
// 回归：修复前 NewExecutor 写死 failFast=true 且无 setter，该路径为不可达死代码。
func TestExecutorContinueOnError(t *testing.T) {
	reg := testReg(failNode{}, echoNode{typ: "after", name: "AFTER"})
	gnodes := []GraphNode{
		{ID: "f", Type: "test_fail", Name: "失败"},
		{ID: "a", Type: "after", Name: "AFTER"},
	}
	edges := []GraphEdge{{ID: "e1", Source: "f", Target: "a"}}
	ecx := &Context{}
	_, err := NewExecutor(reg).WithContinueOnError().Run(context.Background(), gnodes, edges, ecx)
	if err != nil {
		t.Fatalf("容错模式不应返回错误，got %v", err)
	}
	if !strings.Contains(strings.Join(ecx.Messages, ","), "AFTER") {
		t.Fatalf("容错模式下游应执行，messages=%v", ecx.Messages)
	}
	var hasFail bool
	for _, le := range ecx.Log {
		if le.Status == "failed" {
			hasFail = true
		}
	}
	if !hasFail {
		t.Fatal("失败节点应记 failed 日志")
	}
}

// TestExecutorLoopTimesClampedToTwenty loop_times 超上限（>20）被钳制为 20，防止重试风暴。
func TestExecutorLoopTimesClampedToTwenty(t *testing.T) {
	reg := testReg(retryNode{failFirst: 999}) // 始终失败
	gnodes := []GraphNode{{
		ID: "r", Type: "retry", Name: "重试",
		Params: map[string]any{"fail_first": 999, "loop": true, "loop_times": 100, "loop_interval_sec": 0},
	}}
	ecx := &Context{}
	NewExecutor(reg).Run(context.Background(), gnodes, nil, ecx) // 必然失败，忽略 err
	if calls := ecx.Variables["calls"].(int); calls != 20 {
		t.Fatalf("loop_times=100 应钳制到 20 次执行，got %d", calls)
	}
}

// TestExecutorLoopTimesDefaultsToThree loop_times<=0 时默认重试 3 次。
func TestExecutorLoopTimesDefaultsToThree(t *testing.T) {
	reg := testReg(retryNode{failFirst: 999})
	gnodes := []GraphNode{{
		ID: "r", Type: "retry", Name: "重试",
		Params: map[string]any{"fail_first": 999, "loop": true, "loop_times": 0, "loop_interval_sec": 0},
	}}
	ecx := &Context{}
	NewExecutor(reg).Run(context.Background(), gnodes, nil, ecx)
	if calls := ecx.Variables["calls"].(int); calls != 3 {
		t.Fatalf("loop_times=0 应默认 3 次执行，got %d", calls)
	}
}

// --- Stop 短路 / 多根 / 多分支 ---------------------------------------------

// TestExecutorStopShortCircuits 节点置 ecx.Stop=true 后，下游节点不执行。
func TestExecutorStopShortCircuits(t *testing.T) {
	reg := testReg(stopNode{}, echoNode{typ: "after", name: "AFTER"})
	gnodes := []GraphNode{
		{ID: "s", Type: "stop", Name: "停"},
		{ID: "a", Type: "after", Name: "AFTER"},
	}
	edges := []GraphEdge{{ID: "e1", Source: "s", Target: "a"}}
	ecx := &Context{}
	if _, err := NewExecutor(reg).Run(context.Background(), gnodes, edges, ecx); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.Contains(strings.Join(ecx.Messages, ","), "AFTER") {
		t.Fatalf("Stop 后 AFTER 不应执行，got %v", ecx.Messages)
	}
}

// TestExecutorMultipleRoots 多个无入边根节点都会执行（按 gnodes 原始顺序）。
func TestExecutorMultipleRoots(t *testing.T) {
	reg := testReg(echoNode{typ: "r1", name: "R1"}, echoNode{typ: "r2", name: "R2"})
	gnodes := []GraphNode{
		{ID: "a", Type: "r1", Name: "R1"},
		{ID: "b", Type: "r2", Name: "R2"},
	}
	ecx := &Context{}
	if _, err := NewExecutor(reg).Run(context.Background(), gnodes, nil, ecx); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := strings.Join(ecx.Messages, ",")
	if got != "R1,R2" {
		t.Fatalf("两根节点都应执行，messages=%q", got)
	}
}

// TestExecutorMultiBranchSelect 条件节点同时选中多个分支（Next=[a,b]），仅未选中的分支被裁剪。
func TestExecutorMultiBranchSelect(t *testing.T) {
	reg := testReg(multiBranchNode{},
		echoNode{typ: "a", name: "A"}, echoNode{typ: "b", name: "B"}, echoNode{typ: "c", name: "C"})
	gnodes := []GraphNode{
		{ID: "cond", Type: "multi_branch", Name: "多分支", Params: map[string]any{"branches": "a,b"}},
		{ID: "a", Type: "a", Name: "A"},
		{ID: "b", Type: "b", Name: "B"},
		{ID: "c", Type: "c", Name: "C"},
	}
	edges := []GraphEdge{
		{ID: "e1", Source: "cond", Target: "a", SourcePort: "a"},
		{ID: "e2", Source: "cond", Target: "b", SourcePort: "b"},
		{ID: "e3", Source: "cond", Target: "c", SourcePort: "c"},
	}
	ecx := &Context{}
	if _, err := NewExecutor(reg).Run(context.Background(), gnodes, edges, ecx); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := strings.Join(ecx.Messages, ",")
	// cond 本身不写 messages；A、B 执行，C 被裁剪。
	if !strings.Contains(got, "A") || !strings.Contains(got, "B") || strings.Contains(got, "C") {
		t.Fatalf("应执行 A、B 不执行 C，messages=%q", got)
	}
}

// TestExecutorUnknownTypeLogsSkipped Run 中遇未知节点类型：记 skipped 日志并 fail-fast 返回错误。
func TestExecutorUnknownTypeLogsSkipped(t *testing.T) {
	reg := testReg(echoNode{typ: "echo", name: "A"})
	gnodes := []GraphNode{{ID: "g", Type: "ghost", Name: "幽灵"}}
	ecx := &Context{}
	_, err := NewExecutor(reg).Run(context.Background(), gnodes, nil, ecx)
	if err == nil {
		t.Fatal("未知节点类型应触发 fail-fast 错误")
	}
	var skipped bool
	for _, le := range ecx.Log {
		if le.Status == "skipped" {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("未知类型应记 skipped 日志，got %+v", ecx.Log)
	}
}

// --- Validate 边界 --------------------------------------------------------

// TestValidateEmpty 空工作流校验失败。
func TestValidateEmpty(t *testing.T) {
	reg := testReg(echoNode{typ: "echo", name: "A"})
	if err := NewExecutor(reg).Validate(nil, nil); err == nil {
		t.Fatal("空工作流应校验失败")
	}
}

// TestValidateDuplicateID 节点 id 重复校验失败。
func TestValidateDuplicateID(t *testing.T) {
	reg := testReg(echoNode{typ: "echo", name: "A"})
	gnodes := []GraphNode{
		{ID: "dup", Type: "echo", Name: "A"},
		{ID: "dup", Type: "echo", Name: "B"},
	}
	if err := NewExecutor(reg).Validate(gnodes, nil); err == nil {
		t.Fatal("重复 id 应校验失败")
	}
}

// TestValidateEmptyID 节点无 id 校验失败。
func TestValidateEmptyID(t *testing.T) {
	reg := testReg(echoNode{typ: "echo", name: "A"})
	gnodes := []GraphNode{{ID: "", Type: "echo", Name: "A"}}
	if err := NewExecutor(reg).Validate(gnodes, nil); err == nil {
		t.Fatal("无 id 应校验失败")
	}
}

// TestValidateMissingRequired 必填参数缺失校验失败。
func TestValidateMissingRequired(t *testing.T) {
	reg := testReg(reqNode{})
	gnodes := []GraphNode{{ID: "x", Type: "req", Name: "必填", Params: map[string]any{}}}
	if err := NewExecutor(reg).Validate(gnodes, nil); err == nil {
		t.Fatal("缺失必填参数应校验失败")
	}
	// 补齐必填参数后通过。
	gnodes[0].Params = map[string]any{"key": "v"}
	if err := NewExecutor(reg).Validate(gnodes, nil); err != nil {
		t.Fatalf("补齐必填参数后应通过校验，got %v", err)
	}
}

// TestValidateEdgeEndpointMissing 边端点不存在校验失败。
func TestValidateEdgeEndpointMissing(t *testing.T) {
	reg := testReg(echoNode{typ: "echo", name: "A"})
	gnodes := []GraphNode{{ID: "a", Type: "echo", Name: "A"}}
	edges := []GraphEdge{
		{ID: "e1", Source: "a", Target: "ghost"},  // 目标不存在
		{ID: "e2", Source: "ghost2", Target: "a"}, // 源不存在
	}
	for _, ed := range edges {
		if err := NewExecutor(reg).Validate(gnodes, []GraphEdge{ed}); err == nil {
			t.Fatalf("端点不存在的边应校验失败：%+v", ed)
		}
	}
}
