// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package assessment

import (
	"context"
	"strings"
	"testing"
)

// mockDoer 记录所有 GET 请求 URL，用于验证 FetchAssessments 的 userdetails.php?id=<uid> 拼装。
type mockDoer struct{ gets []string }

func (m *mockDoer) Get(ctx context.Context, url, cookie string, ssrf bool) (*Response, error) {
	m.gets = append(m.gets, url)
	return &Response{StatusCode: 200, Body: []byte("<html></html>")}, nil
}

func (m *mockDoer) Do(ctx context.Context, method, url string, body []byte, headers map[string]string, cookie string, ssrf bool) (*Response, error) {
	return &Response{StatusCode: 200}, nil
}

// TestNexusPHPFetchAssessments_UIDRequired 验证 P0 修复：assessment 绝不裸访 userdetails.php。
//   - uid 为空 → 不发任何请求（chain 层负责提供 uid：聚合直传 / 独立路径回退 UserInfo）
//   - uid 非空 → GET userdetails.php?id=<uid>（带参数，否则站点视为无效参数 → 52movie 警告根因）
func TestNexusPHPFetchAssessments_UIDRequired(t *testing.T) {
	ctx := context.Background()

	// uid 空：必须零请求（不裸访 userdetails.php）。
	m := &mockDoer{}
	tasks, err := NexusPHPAdapter{}.FetchAssessments(ctx, m, Def{}, "https://s.test", "ck", "")
	if err != nil || tasks != nil {
		t.Fatalf("uid empty: want (nil,nil), got (%v,%v)", tasks, err)
	}
	if len(m.gets) != 0 {
		t.Fatalf("uid empty: must NOT GET userdetails, got %d request(s): %v", len(m.gets), m.gets)
	}

	// uid 非空：GET userdetails.php?id=<uid>。
	m2 := &mockDoer{}
	NexusPHPAdapter{}.FetchAssessments(ctx, m2, Def{}, "https://s.test", "ck", "7788")
	if len(m2.gets) != 1 {
		t.Fatalf("uid set: want exactly 1 GET, got %d", len(m2.gets))
	}
	if !strings.Contains(m2.gets[0], "userdetails.php?id=7788") {
		t.Fatalf("uid set: want URL containing 'userdetails.php?id=7788', got %q", m2.gets[0])
	}
}
