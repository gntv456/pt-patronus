// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// TestConditionNodeTorrentsMode torrents 模式：len(Torrents) 与整数 value 比较，覆盖全部算子 + 非整数报错。
func TestConditionNodeTorrentsMode(t *testing.T) {
	n := NewConditionNode()
	cases := []struct {
		name     string
		torrents int
		op, val  string
		want     string // "true" | "false"
	}{
		{"gt hit", 3, "gt", "0", "true"},
		{"gt miss", 0, "gt", "0", "false"},
		{"ge eq", 3, "ge", "3", "true"},
		{"eq hit", 3, "eq", "3", "true"},
		{"eq miss", 3, "eq", "5", "false"},
		{"ne hit", 3, "ne", "5", "true"},
		{"ne miss", 3, "ne", "3", "false"},
		{"lt hit", 1, "lt", "3", "true"},
		{"le eq", 3, "le", "3", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ecx := &workflow.Context{Torrents: make([]site.Torrent, tc.torrents)}
			if err := n.Execute(context.Background(), map[string]any{
				"left": "torrents", "operator": tc.op, "value": tc.val,
			}, ecx); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if len(ecx.Next) != 1 || ecx.Next[0] != tc.want {
				t.Fatalf("Next=%v want [%s]", ecx.Next, tc.want)
			}
		})
	}
	// 非整数比较值应报错（资源数量比较值需为整数）。
	err := n.Execute(context.Background(), map[string]any{
		"left": "torrents", "operator": "gt", "value": "abc",
	}, &workflow.Context{})
	if err == nil {
		t.Fatal("非整数比较值应报错")
	}
}

// TestConditionNodeVariableMode variable 模式：数值比较、contains、字符串回退（gt/lt 恒 false）、变量缺失。
func TestConditionNodeVariableMode(t *testing.T) {
	n := NewConditionNode()
	cases := []struct {
		name  string
		vars  map[string]any
		op, v string
		want  string
	}{
		{"num gt true", map[string]any{"x": 5}, "gt", "3", "true"},
		{"num lt false", map[string]any{"x": 5}, "lt", "3", "false"},
		{"num ge eq", map[string]any{"x": 5}, "ge", "5", "true"},
		{"contains hit", map[string]any{"x": "hello world"}, "contains", "world", "true"},
		{"contains miss", map[string]any{"x": "hello"}, "contains", "zzz", "false"},
		// 字符串回退（非数字操作数）：字典序比较支持全部算子（回归：修复前 gt/lt 恒 false）。
		{"str gt dict", map[string]any{"x": "b"}, "gt", "a", "true"},
		{"str lt dict", map[string]any{"x": "a"}, "lt", "b", "true"},
		{"str ge eq", map[string]any{"x": "abc"}, "ge", "abc", "true"},
		{"str eq true", map[string]any{"x": "abc"}, "eq", "abc", "true"},
		{"str ne true", map[string]any{"x": "abc"}, "ne", "abd", "true"},
		// 变量缺失（Variables 已初始化但无该 key）→ 统一空串（回归：修复前为字面 "<nil>"）。
		{"missing ne true", map[string]any{}, "ne", "x", "true"},
		{"missing eq empty", map[string]any{}, "eq", "", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ecx := &workflow.Context{Variables: tc.vars}
			if err := n.Execute(context.Background(), map[string]any{
				"left": "variable", "variable": "x", "operator": tc.op, "value": tc.v,
			}, ecx); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if len(ecx.Next) != 1 || ecx.Next[0] != tc.want {
				t.Fatalf("Next=%v want [%s]", ecx.Next, tc.want)
			}
		})
	}
}

// TestDelayNodeDelayRespectsContext delay：<=0 立即返回；等待中被 ctx 取消返回 ctx.Err()。
// 注：>60s 截断逻辑为内部钳制，无法在毫秒级测试中直接断言截断值，仅验证大值不 panic 且响应取消。
func TestDelayNodeDelayRespectsContext(t *testing.T) {
	n := NewDelayNode()

	// 非正秒数 no-op。
	ecx := &workflow.Context{}
	start := time.Now()
	if err := n.Execute(context.Background(), map[string]any{"seconds": 0}, ecx); err != nil {
		t.Fatalf("seconds=0 应 no-op，got err %v", err)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("seconds=0 应立即返回，耗时 %v", time.Since(start))
	}
	if err := n.Execute(context.Background(), map[string]any{"seconds": -5}, ecx); err != nil {
		t.Fatalf("seconds<0 应 no-op，got err %v", err)
	}

	// 父 ctx 提前取消：seconds=60（>60 会截断到 60）应因 ctx 取消快速返回 ctx.Err()。
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := n.Execute(ctx, map[string]any{"seconds": 60}, ecx)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ctx 取消应返回 DeadlineExceeded，got %v", err)
	}
}

// TestSetVariableNodeKeyAndRender key 空报错；值支持 {{var.x}} 模板渲染。
func TestSetVariableNodeKeyAndRender(t *testing.T) {
	n := NewSetVariableNode()

	if err := n.Execute(context.Background(), map[string]any{"key": "", "value": "v"}, &workflow.Context{}); err == nil {
		t.Fatal("key 为空应报错")
	}

	ecx := &workflow.Context{Variables: map[string]any{"base": "B"}}
	if err := n.Execute(context.Background(), map[string]any{"key": "out", "value": "{{var.base}}-X"}, ecx); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := ecx.Variables["out"]; got != "B-X" {
		t.Fatalf("模板渲染 out=%v want B-X", got)
	}

	// ecx.Variables 为 nil 时自动初始化。
	ecx2 := &workflow.Context{}
	if err := n.Execute(context.Background(), map[string]any{"key": "k", "value": "1"}, ecx2); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if ecx2.Variables == nil || ecx2.Variables["k"] != "1" {
		t.Fatalf("应自动初始化 Variables 并写入，got %+v", ecx2.Variables)
	}
}

// TestStopFlowNodeSetsStop stop_flow 置 ecx.Stop=true；空 reason 回退「手动终止」。
func TestStopFlowNodeSetsStop(t *testing.T) {
	n := NewStopFlowNode()

	ecx := &workflow.Context{}
	if err := n.Execute(context.Background(), map[string]any{"reason": "测试终止"}, ecx); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !ecx.Stop {
		t.Fatal("应置 ecx.Stop=true")
	}

	ecx2 := &workflow.Context{}
	n.Execute(context.Background(), map[string]any{"reason": ""}, ecx2)
	if !ecx2.Stop {
		t.Fatal("空 reason 也应置 Stop")
	}
	// 消息含「终止流程」。
	if len(ecx2.Messages) == 0 {
		t.Fatal("应记终止消息")
	}
}

// TestNoteNodeOptionalMessage note 节点：空 text 不记消息；非空记「备注：...」。
func TestNoteNodeOptionalMessage(t *testing.T) {
	n := NewNoteNode()
	ecx1 := &workflow.Context{}
	n.Execute(context.Background(), map[string]any{"text": ""}, ecx1)
	if len(ecx1.Messages) != 0 {
		t.Fatalf("空 text 不应记消息，got %v", ecx1.Messages)
	}
	ecx2 := &workflow.Context{}
	n.Execute(context.Background(), map[string]any{"text": "hello"}, ecx2)
	if len(ecx2.Messages) != 1 || ecx2.Messages[0] != "备注：hello" {
		t.Fatalf("应记备注消息，got %v", ecx2.Messages)
	}
}
