// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"testing"
	"time"
)

// TestOpsExecContextSurvivesClientDisconnect 审计型批量执行不能被客户端断开腰斩。
// 曾经直接用 c.Request.Context()：关标签页/前端超时 → 在途 SSH 全部取消，
// 而远端命令多半已经跑完，审计记录却把它们写成「context canceled」的失败。
func TestOpsExecContextSurvivesClientDisconnect(t *testing.T) {
	req, cancelReq := context.WithCancel(context.Background())
	exec, cancelExec := opsExecContext(req)
	defer cancelExec()

	cancelReq() // 模拟客户端断开
	select {
	case <-exec.Done():
		t.Fatal("请求上下文一取消就中断批量执行：远端可能已执行完命令，审计却记成失败")
	default:
	}

	deadline, ok := exec.Deadline()
	if !ok {
		t.Fatal("脱离请求后必须有总时长上限兜底，否则无人管的批量执行会一直挂着")
	}
	if left := time.Until(deadline); left <= 0 || left > opsRunMaxDuration+time.Minute {
		t.Errorf("总时长上限异常，剩余 %v（上限 %v）", left, opsRunMaxDuration)
	}

	cancelExec() // 自己派生的取消函数仍要能收口（否则 goroutine 与连接泄漏）
	select {
	case <-exec.Done():
	case <-time.After(2 * time.Second):
		t.Error("cancelExec 未能结束派生上下文")
	}
}
