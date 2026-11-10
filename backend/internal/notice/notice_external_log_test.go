// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package notice

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/notification"
)

// safeBuf：外发跑在 goroutine 里，裸 bytes.Buffer 会被主线程并发读到 -race 报警。
type safeBuf struct {
	mu sync.Mutex
	sb strings.Builder
}

func (b *safeBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.Write(p)
}

func (b *safeBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.String()
}

// alwaysFailChannel 是一条永远发不出去的渠道（渠道侧原因，如 TG 被墙/token 失效）。
type alwaysFailChannel struct{ name string }

func (c alwaysFailChannel) Name() string { return c.name }
func (c alwaysFailChannel) Send(context.Context, notification.Message) error {
	return errors.New("dial tcp: lookup api.telegram.org: no such host")
}

type okChannel struct{ name string }

func (c okChannel) Name() string                                     { return c.name }
func (c okChannel) Send(context.Context, notification.Message) error { return nil }

func waitFor(t *testing.T, cond func() bool, why string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等不到%s（3s 超时）", why)
}

// TestNoticeLogsUnrecoverableExternalFailure danger/warning 级通知会外发（TG/Bark/webhook），
// 失败后重试一次——代码注释写明这条重试是给「备份结果这类一次性事件丢失不可恢复」兜底的。
// 渠道级失败原本就有一行 Warn("notify failed")（manager.go:119，第一次与重试各一条），
// 但那两行同形、只有渠道名与错误：**没有这条通知是谁，也没有「重试也没救回来」这个终局结论**，
// 而重试的结果返回值原本更是整份丢弃（连变量都没接）。缺的正是那条能指导「要不要手工补发」的读数。
func TestNoticeLogsUnrecoverableExternalFailure(t *testing.T) {
	oldDelay := noticeRetryDelay
	noticeRetryDelay = time.Millisecond
	t.Cleanup(func() { noticeRetryDelay = oldDelay })

	buf := &safeBuf{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	svc := NewService(&fakeStore{}, nil, notification.NewManager(alwaysFailChannel{"probe-tg"}))
	if err := svc.Notice(context.Background(), Input{
		Source: SourceBackupDone, Key: "scheduled-backup:2026-10-13",
		Title: "定时备份失败", Body: "磁盘写满", Level: "danger",
	}, false); err != nil {
		t.Fatalf("站内落库这一路不该失败: %v", err)
	}

	waitFor(t, func() bool { return strings.Contains(buf.String(), "通知外发重试仍失败") }, "重试仍失败的 Error 日志")
	logged := buf.String()
	// 渠道级失败本来就有 notify failed 一行（manager.go:119），但那条只有渠道名与错误；
	// 这里要的是**终局结论 + 这条通知是谁**，缺一就无法判断「还要不要再手工补一次」。
	for _, want := range []string{"定时备份失败", "probe-tg", "no such host", "level=danger", "notice_id=1", "source=backup_done"} {
		if !strings.Contains(logged, want) {
			t.Errorf("日志里要点名 %q, got %q", want, logged)
		}
	}
	if !strings.Contains(logged, "notify failed") {
		t.Errorf("渠道级明细（既有那条）应仍在, got %q", logged)
	}

	// 正控：渠道正常时不许出现这两句（否则「外发坏了」会被平时的噪音盖掉）。
	buf2 := &safeBuf{}
	slog.SetDefault(slog.New(slog.NewTextHandler(buf2, nil)))
	svc2 := NewService(&fakeStore{}, nil, notification.NewManager(okChannel{"tg-ok"}))
	if err := svc2.Notice(context.Background(), Input{
		Source: SourceBackupDone, Key: "ok:1", Title: "定时备份完成", Level: "success",
	}, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if s := buf2.String(); strings.Contains(s, "外发失败") || strings.Contains(s, "重试仍失败") {
		t.Errorf("成功渠道混进了失败日志, got %q", s)
	}
}
