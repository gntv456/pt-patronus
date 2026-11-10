// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/oper"
)

func seedBrokenRule(t *testing.T, op *oper.StrmRuleOper, name string) {
	t.Helper()
	// DriveID 指向不存在的盘：GenerateStrm 在取客户端一步就失败，
	// 不需要真网盘即可复现「整条规则失败」。
	if _, err := op.Create(oper.StrmRuleInput{
		Name: name, DriveID: 999, CloudPath: "/电影", OutputDir: "/strm",
	}); err != nil {
		t.Fatalf("seed rule %s: %v", name, err)
	}
}

// TestGenerateAllReportsPerRuleFailures 回归：GenerateAll 原来无返回值，
// 每条规则的失败只 slog.Warn；UI 的「全部生成」按钮、AI 工具与插件因此一律显示成功，
// 用户以为 .strm 已经落地，实际一条都没生成。
func TestGenerateAllReportsPerRuleFailures(t *testing.T) {
	ch, _, rOp := newCloudTestChain(t)
	seedBrokenRule(t, rOp, "rule-movie")
	seedBrokenRule(t, rOp, "rule-tv")

	s := ch.GenerateAll(context.Background())
	if s.Rules != 2 {
		t.Fatalf("应统计到 2 条启用规则，got %d", s.Rules)
	}
	if len(s.Failed) != 2 {
		t.Fatalf("两条规则都该被点名为失败，got %v", s.Failed)
	}
	joined := strings.Join(s.Failed, "\n")
	for _, want := range []string{"rule-movie", "rule-tv"} {
		if !strings.Contains(joined, want) {
			t.Errorf("失败项应带规则名 %s，got %q", want, joined)
		}
	}
	if s.Generated != 0 || s.Skipped != 0 {
		t.Errorf("全失败轮不应有产出，got generated=%d skipped=%d", s.Generated, s.Skipped)
	}
}

// TestGenerateAllNoRulesIsNotAFailure 正控：没有启用规则是「无事可做」，
// 不该被算成失败（前端要能区分「没有规则」与「规则都挂了」）。
func TestGenerateAllNoRulesIsNotAFailure(t *testing.T) {
	ch, _, _ := newCloudTestChain(t)
	s := ch.GenerateAll(context.Background())
	if s.Rules != 0 || len(s.Failed) != 0 {
		t.Fatalf("空规则应 0 条且无失败，got %+v", s)
	}
}
