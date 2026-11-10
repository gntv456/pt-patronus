// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package scheduler 封装 robfig/cron，注册定时任务并内置 panic 恢复。
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"
)

// Scheduler 包装 cron.Cron。
type Scheduler struct {
	c    *cron.Cron
	mu   sync.Mutex
	jobs map[string]cron.EntryID // name → EntryID（Remove/重注册用）
}

// New 构造调度器（Recover 防 panic 致命；SkipIfStillRunning 防慢任务超 tick 重叠——
// 避免上一轮签到/辅种/RSS/统计未跑完时下一轮并发触发重复副作用）。
func New() *Scheduler {
	return &Scheduler{
		c: cron.New(cron.WithChain(
			cron.Recover(cron.PrintfLogger(slogLogger{})),
			cron.SkipIfStillRunning(cron.PrintfLogger(slogLogger{})),
		)),
		jobs: make(map[string]cron.EntryID),
	}
}

// AddFunc 注册一个定时任务。同名任务先移除旧的（幂等替换语义），便于工作流启用/更新重注册。
func (s *Scheduler) AddFunc(name, spec string, fn func()) error {
	id, err := s.c.AddFunc(spec, func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("cron job panic", "job", name, "panic", r)
			}
		}()
		slog.Info("cron job start", "job", name)
		fn()
	})
	if err != nil {
		return fmt.Errorf("add cron %q: %w", name, err)
	}
	s.mu.Lock()
	if old, ok := s.jobs[name]; ok {
		s.c.Remove(old) // 重注册：移除旧 EntryID（cron.Cron.Remove 对 stale id 安全）
	}
	s.jobs[name] = id
	s.mu.Unlock()
	slog.Info("cron registered", "job", name, "spec", spec)
	return nil
}

// Remove 按名称移除定时任务；不存在则无操作（幂等）。
func (s *Scheduler) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.jobs[name]; ok {
		s.c.Remove(id)
		delete(s.jobs, name)
	}
}

// Start 启动调度。
func (s *Scheduler) Start() {
	s.c.Start()
	slog.Info("scheduler started")
}

// Stop 停止调度，返回等待上下文。
func (s *Scheduler) Stop() context.Context {
	return s.c.Stop()
}

// slogLogger 适配 cron 的 PrintfLogger 接口。
type slogLogger struct{}

func (slogLogger) Printf(format string, args ...interface{}) {
	slog.Info("cron: " + fmt.Sprintf(format, args...))
}
