// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package transfer

import "testing"

// TestShouldProcessTagBypass 二轮 P1-1 回归：TR（Category 恒空）靠 subscribe 标签进门。
func TestShouldProcessTagBypass(t *testing.T) {
	cfg := DefaultConfig()
	// TR 场景：category 空，tags 含订阅链路打的 subscribe
	if !cfg.ShouldProcess("", []string{"subscribe", "ptpsrc|douban|123|movie"}) {
		t.Fatal("TR 场景：subscribe 标签应放行整理（白名单分类名的标签旁路）")
	}
	// workflow 标签同理
	if !cfg.ShouldProcess("", []string{"workflow"}) {
		t.Fatal("workflow 标签应放行")
	}
	// 用户自填非白名单分类（RSS P1-2 残留面）：标签仍救得回来
	if !cfg.ShouldProcess("mycat", []string{"subscribe"}) {
		t.Fatal("自填分类+subscribe 标签应放行")
	}
	// 无标签无白名单分类：仍然拦（保持原语义，不放松）
	if cfg.ShouldProcess("", nil) {
		t.Fatal("空分类空标签不得放行")
	}
	if cfg.ShouldProcess("mycat", nil) {
		t.Fatal("非白名单分类无标签不得放行")
	}
}
