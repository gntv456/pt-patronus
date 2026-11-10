// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"strings"
	"testing"
)

// CookieCloud 推送的成败只写在响应正文里（HTTP 常是 200）。旧判据
// `json.Unmarshal(...) == nil && action 非 done` 会把非 JSON 的 200 ——Cloudflare
// 「Just a moment…」拦截页、反代改写的错误页——一路放行成「已推送 N 条」，
// 用户以为凭据已上云，实际云端还是旧 cookie（下次拉取就把旧的推回本地）。
func TestJudgeCookieCloudPush(t *testing.T) {
	if err := judgeCookieCloudPush(nil); err != nil {
		t.Errorf("空 body 是既有兼容分支，应算成功, got %v", err)
	}
	if err := judgeCookieCloudPush([]byte(`{"action":"done"}`)); err != nil {
		t.Errorf("action=done 应成功, got %v", err)
	}
	if err := judgeCookieCloudPush([]byte(`{"ok":true}`)); err != nil {
		t.Errorf("合法 JSON 但没有 action 字段仍按兼容放行, got %v", err)
	}

	err := judgeCookieCloudPush([]byte("<html><title>Just a moment...</title></html>"))
	if err == nil {
		t.Fatal("非 JSON 的 200 被当成推送成功（缺陷原形）")
	}
	msg := err.Error()
	if !strings.Contains(msg, "未确认成功") || !strings.Contains(msg, "Just a moment") {
		t.Errorf("要说清未能确认并带原文片段, got %q", msg)
	}

	// 服务给了 error 字段就用它，别只回 action=error
	if m := judgeCookieCloudPush([]byte(`{"action":"error","error":"pwd required"}`)).Error(); !strings.Contains(m, "pwd required") {
		t.Errorf("上游原因要透出来, got %q", m)
	}
	// 没有 error 字段时回退到 action=error，不能留「失败: 」空原因
	if m := judgeCookieCloudPush([]byte(`{"action":"error"}`)).Error(); strings.HasSuffix(m, "失败: ") || !strings.Contains(m, "action=error") {
		t.Errorf("空原因文案复现或回退缺失, got %q", m)
	}
}
