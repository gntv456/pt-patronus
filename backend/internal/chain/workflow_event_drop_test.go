// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"os"
	"strings"
	"testing"
)

// #320 事件型工作流的「读不到工作流」必须与「已删除/停用」分开留痕。
// 事件总线不重放：SQLite busy 那一刻到达的触发如果被静默丢掉，这个事件就永久没跑过，
// 而日志里一个字都没有 —— 用户只能看到「它就是没触发」。

func TestEventDropKeepsReadFailureLoud(t *testing.T) {
	src, err := os.ReadFile("workflow_triggers.go")
	if err != nil {
		t.Fatalf("读不到 workflow_triggers.go：%v", err)
	}
	code := stripTriggerComments(string(src))
	i := strings.Index(code, "func (c *WorkflowChain) registerEventHandler")
	if i < 0 {
		t.Fatalf("找不到 registerEventHandler（否则下面几条判据是空的）")
	}
	j := strings.Index(code[i+1:], "\nfunc ")
	body := code[i : i+1+j]

	head := body[strings.Index(body, "cur, err := c.op.Get(wfID)"):]
	head = head[:min(420, len(head))]
	if strings.Contains(head, "err != nil || cur == nil") {
		t.Errorf("读失败与「已删除/停用」又并回同一个条件了 —— 前者必须单独一支（旧形状：%s）", firstLines(head, 3))
	}
	if !strings.Contains(head, `slog.Warn("workflow event dropped: cannot read workflow"`) {
		t.Errorf("读不到工作流这一支必须留痕（事件不重放，不记就永远查不到）")
	}

	// 正控维度：正当丢弃（已删除/停用/改类型）不许跟着吵，否则日志会被正常事件刷满。
	k := strings.Index(body, "if cur == nil || !cur.Enabled")
	if k < 0 {
		t.Fatalf("找不到停用/已删除那一支")
	}
	branch := body[k:]
	branch = branch[:strings.Index(branch, "\n		}")+2]
	if strings.Contains(branch, "slog.") {
		t.Errorf("「已删除/停用/换事件」是正当丢弃，不该留痕：%s", firstLines(branch, 4))
	}
}

func stripTriggerComments(s string) string {
	out := make([]string, 0, 64)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func firstLines(s string, n int) string {
	parts := strings.Split(strings.TrimSpace(s), "\n")
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.Join(parts, " | ")
}
