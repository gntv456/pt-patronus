// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// echoNode 测试用节点：执行时把自身 name 追加到 ecx.Messages。
type echoNode struct {
	typ, name string
}

func (n echoNode) Type() string                  { return n.typ }
func (n echoNode) Name() string                  { return n.name }
func (n echoNode) Category() string              { return "测试" }
func (n echoNode) Description() string           { return "test" }
func (n echoNode) Icon() string                  { return "test" }
func (n echoNode) DefaultParams() map[string]any { return nil }
func (n echoNode) ParamSchema() []ParamField     { return nil }
func (n echoNode) Execute(_ context.Context, _ map[string]any, ecx *Context) error {
	ecx.Messages = append(ecx.Messages, n.name)
	return nil
}

// branchNode 模拟条件节点：按 param "choose" 设置 ecx.Next。
type branchNode struct{}

func (b branchNode) Type() string                  { return "test_branch" }
func (b branchNode) Name() string                  { return "分支" }
func (b branchNode) Category() string              { return "测试" }
func (b branchNode) Description() string           { return "test" }
func (b branchNode) Icon() string                  { return "test" }
func (b branchNode) DefaultParams() map[string]any { return map[string]any{"choose": "true"} }
func (b branchNode) ParamSchema() []ParamField     { return nil }
func (b branchNode) Execute(_ context.Context, params map[string]any, ecx *Context) error {
	ecx.Next = []string{ParamString(params, "choose")}
	return nil
}

// failNode 模拟失败节点。
type failNode struct{}

func (failNode) Type() string                  { return "test_fail" }
func (failNode) Name() string                  { return "失败" }
func (failNode) Category() string              { return "测试" }
func (failNode) Description() string           { return "test" }
func (failNode) Icon() string                  { return "test" }
func (failNode) DefaultParams() map[string]any { return nil }
func (failNode) ParamSchema() []ParamField     { return nil }
func (failNode) Execute(_ context.Context, _ map[string]any, _ *Context) error {
	return errors.New("boom")
}

func testReg(nodes ...Node) *Registry {
	r := NewRegistry()
	for _, n := range nodes {
		r.Register(n)
	}
	return r
}

// TestExecutorLinear 线性 A→B→C 全部执行且有序。
func TestExecutorLinear(t *testing.T) {
	reg := testReg(echoNode{typ: "echo", name: "A"}, echoNode{typ: "echo2", name: "B"}, echoNode{typ: "echo3", name: "C"})
	gnodes := []GraphNode{
		{ID: "a", Type: "echo", Name: "A"},
		{ID: "b", Type: "echo2", Name: "B"},
		{ID: "c", Type: "echo3", Name: "C"},
	}
	edges := []GraphEdge{{ID: "e1", Source: "a", Target: "b"}, {ID: "e2", Source: "b", Target: "c"}}
	ecx := &Context{}
	_, err := NewExecutor(reg).Run(context.Background(), gnodes, edges, ecx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := strings.Join(ecx.Messages, ",")
	want := "A,B,C"
	if got != want {
		t.Fatalf("messages = %q, want %q", got, want)
	}
}

// TestExecutorConditionBranch 条件节点只走选中分支；未选中分支及其下游不执行。
func TestExecutorConditionBranch(t *testing.T) {
	reg := testReg(branchNode{}, echoNode{typ: "trueNode", name: "TRUE分支"}, echoNode{typ: "falseNode", name: "FALSE分支"})
	gnodes := []GraphNode{
		{ID: "cond", Type: "test_branch", Name: "分支", Params: map[string]any{"choose": "true"}},
		{ID: "t", Type: "trueNode", Name: "TRUE分支"},
		{ID: "f", Type: "falseNode", Name: "FALSE分支"},
	}
	edges := []GraphEdge{
		{ID: "e1", Source: "cond", Target: "t", SourcePort: "true"},
		{ID: "e2", Source: "cond", Target: "f", SourcePort: "false"},
	}
	ecx := &Context{}
	if _, err := NewExecutor(reg).Run(context.Background(), gnodes, edges, ecx); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := strings.Join(ecx.Messages, ",")
	if got != "TRUE分支" {
		t.Fatalf("messages = %q, want only TRUE分支", got)
	}

	// 选 false 分支
	gnodes[0].Params = map[string]any{"choose": "false"}
	ecx2 := &Context{}
	if _, err := NewExecutor(reg).Run(context.Background(), gnodes, edges, ecx2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := strings.Join(ecx2.Messages, ","); got != "FALSE分支" {
		t.Fatalf("messages = %q, want only FALSE分支", got)
	}
}

// TestExecutorConditionMerge 合并节点在选中分支执行后运行（不等待未选中的死分支）。
func TestExecutorConditionMerge(t *testing.T) {
	reg := testReg(branchNode{}, echoNode{typ: "tB", name: "TRUE"}, echoNode{typ: "fB", name: "FALSE"}, echoNode{typ: "merge", name: "MERGE"})
	gnodes := []GraphNode{
		{ID: "cond", Type: "test_branch", Name: "分支", Params: map[string]any{"choose": "true"}},
		{ID: "t", Type: "tB", Name: "TRUE"},
		{ID: "f", Type: "fB", Name: "FALSE"},
		{ID: "m", Type: "merge", Name: "MERGE"},
	}
	edges := []GraphEdge{
		{ID: "e1", Source: "cond", Target: "t", SourcePort: "true"},
		{ID: "e2", Source: "cond", Target: "f", SourcePort: "false"},
		{ID: "e3", Source: "t", Target: "m"},
		{ID: "e4", Source: "f", Target: "m"},
	}
	ecx := &Context{}
	if _, err := NewExecutor(reg).Run(context.Background(), gnodes, edges, ecx); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := strings.Join(ecx.Messages, ",")
	// 选 true：TRUE 执行、FALSE 不执行、MERGE 执行
	if !strings.Contains(got, "TRUE") || strings.Contains(got, "FALSE") || !strings.Contains(got, "MERGE") {
		t.Fatalf("messages = %q, want TRUE+MERGE only", got)
	}
}

// TestExecutorFailFast 失败节点 fail-fast 终止。
func TestExecutorFailFast(t *testing.T) {
	reg := testReg(failNode{}, echoNode{typ: "after", name: "AFTER"})
	gnodes := []GraphNode{
		{ID: "f", Type: "test_fail", Name: "失败"},
		{ID: "a", Type: "after", Name: "AFTER"},
	}
	edges := []GraphEdge{{ID: "e1", Source: "f", Target: "a"}}
	ecx := &Context{}
	_, err := NewExecutor(reg).Run(context.Background(), gnodes, edges, ecx)
	if err == nil {
		t.Fatal("expected error from fail node")
	}
	if len(ecx.Messages) != 0 {
		t.Fatalf("AFTER should not run, got messages %v", ecx.Messages)
	}
}

// TestValidateCycle 环检测。
func TestValidateCycle(t *testing.T) {
	reg := testReg(echoNode{typ: "echo", name: "A"})
	gnodes := []GraphNode{{ID: "a", Type: "echo", Name: "A"}, {ID: "b", Type: "echo", Name: "B"}}
	edges := []GraphEdge{{ID: "e1", Source: "a", Target: "b"}, {ID: "e2", Source: "b", Target: "a"}}
	err := NewExecutor(reg).Validate(gnodes, edges)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

// TestValidateUnknownType 未知节点类型。
func TestValidateUnknownType(t *testing.T) {
	reg := testReg(echoNode{typ: "echo", name: "A"})
	gnodes := []GraphNode{{ID: "a", Type: "nope", Name: "X"}}
	err := NewExecutor(reg).Validate(gnodes, nil)
	if err == nil {
		t.Fatal("expected unknown type error")
	}
}

// TestParamFloat64 JSON 数字解出 float64，ParamInt 兼容。
func TestParamFloat64(t *testing.T) {
	if got := ParamInt(map[string]any{"count": float64(3)}, "count"); got != 3 {
		t.Fatalf("ParamInt(float64) = %d, want 3", got)
	}
	if got := ParamString(map[string]any{"n": float64(42)}, "n"); got != "42" {
		t.Fatalf("ParamString(float64) = %q, want 42", got)
	}
}
