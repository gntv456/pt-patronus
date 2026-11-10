// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import "fmt"

// runGuarded 把单项 panic 转成错误交回调用方。
//
// cron 遍历多条目的循环（刷流任务/契约规则/STRM 规则……）里，一个坏条目的 panic
// 若不就地兜住，会沿调用栈向上抛到 scheduler 级 recover —— 进程不崩，但同轮
// 其余条目全部漏跑，且只有一条 "cron job panic" 日志（用户界面全程无感，
// 对应「scheduler 的全局 Recover ≠ 循环内的单项隔离」）。调用方拿到的错误走
// 既有的单条失败通道（Warn + 落 task_error / summary.Failed），账面不丢。
func runGuarded(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn()
}
