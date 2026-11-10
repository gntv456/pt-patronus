// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package scheduler

import (
	"sync/atomic"
	"testing"
	"time"
)

// AddFunc 注册成功；非法 spec 报错；同名重注册幂等替换；Remove 存在/不存在均安全。
func TestAddFuncAndRemove(t *testing.T) {
	s := New()
	if err := s.AddFunc("job1", "* * * * *", func() {}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.AddFunc("job2", "not-a-cron-spec", func() {}); err == nil {
		t.Fatal("expected error for invalid spec")
	}
	// 同名重注册（工作流启用/更新场景）不应报错且替换旧 EntryID
	if err := s.AddFunc("job1", "*/5 * * * *", func() {}); err != nil {
		t.Fatalf("re-add same name: %v", err)
	}
	s.Remove("job1") // 已注册
	s.Remove("nope") // 未注册：幂等无操作

	s.Start()
	ctx := s.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler Stop did not return within 3s")
	}
}

// TestSchedulerPanicRecovery 验证 panic 任务被 Recover 链 + 内层 defer 捕获，不击溃调度器。
func TestSchedulerPanicRecovery(t *testing.T) {
	s := New()
	var ran atomic.Int32
	if err := s.AddFunc("boom", "@every 1s", func() {
		ran.Add(1)
		panic("explode")
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
	s.Start()
	time.Sleep(1400 * time.Millisecond) // 让 panic 任务至少触发一次
	ctx := s.Stop()
	<-ctx.Done()
	if ran.Load() == 0 {
		t.Fatal("panic job should have executed at least once before recovery")
	}
	// 进程仍存活即说明 panic 被恢复（否则测试进程直接挂）
}
