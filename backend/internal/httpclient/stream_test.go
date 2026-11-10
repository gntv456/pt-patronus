// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package httpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// memHandler 把 slog 记到内存缓冲。用互斥锁而不是裸 buffer：断言发生在其他写入之后，
// 无锁版本在这里是稳定绿、真并发下会撕（本仓 -race 需要 cgo，测不出来）。
type memHandler struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (h *memHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *memHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		h.buf.WriteString(" " + a.Key + "=" + a.Value.String())
		return true
	})
	h.buf.WriteString("\n")
	return nil
}

func (h *memHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *memHandler) WithGroup(string) slog.Handler      { return h }

func (h *memHandler) drain() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.buf.String()
	h.buf.Reset()
	return s
}

// 截断（上游提前 EOF）、与声明长度不符三种情况的取舍。
// 旧实现是 `_, _ = io.Copy(...)`：三种都留下零痕迹，播放器卡顿/半张图无从归因。
func TestCopyBodyLeavesTrace(t *testing.T) {
	h := &memHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	// 1) 干净且长度相符 ⇒ 不该有任何告警（正控：判据不是「只要有写就告警」）
	var dst bytes.Buffer
	if n := CopyBody(&dst, strings.NewReader("abcdef"), "probe", 6); n != 6 {
		t.Fatalf("写入 %d，期望 6", n)
	}
	if s := h.drain(); s != "" {
		t.Errorf("正常转发不该告警，got: %s", s)
	}

	// 2) 中途报错
	dst.Reset()
	r := io.MultiReader(strings.NewReader("abc"), errReader{})
	if n := CopyBody(&dst, r, "probe", 30); n != 3 {
		t.Errorf("写入 %d，期望 3", n)
	}
	s := h.drain()
	if !strings.Contains(s, "上游流中断") || !strings.Contains(s, "written=3") || !strings.Contains(s, "expected=30") {
		t.Errorf("断流告警缺要素: %q", s)
	}

	// 3) 无错误但提前 EOF ⇒ 与上游声明不符
	dst.Reset()
	if n := CopyBody(&dst, strings.NewReader("abcd"), "probe", 10); n != 4 {
		t.Errorf("写入 %d，期望 4", n)
	}
	s = h.drain()
	if !strings.Contains(s, "实际写入长度与上游声明不符") || !strings.Contains(s, "written=4") {
		t.Errorf("长度不符告警缺要素: %q", s)
	}

	// 4) expected<=0（未知长度）不该造假告警
	dst.Reset()
	if CopyBody(&dst, strings.NewReader("xy"), "probe", -1) != 2 {
		t.Fatal("未写入")
	}
	if s := h.drain(); strings.Contains(s, "不符") {
		t.Errorf("Content-Length 未知时不该报长度不符: %q", s)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("unexpected eof from upstream") }
