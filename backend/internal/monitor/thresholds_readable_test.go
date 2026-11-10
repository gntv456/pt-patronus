// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// #311 ParseThresholds 读不出来时静默回退内置默认（90/90/90/75/offline）——
// 用户以为设了 50%，实际按 90% 报警，且没有任何读数。本轮把两件事钉住：
// ①「读不出来」必须留一条 Warn（含原文片段，好定位是哪台设备被写坏了 JSON）；
// ②给写入侧一个与消费方同一张表的判据 ThresholdsReadable（别在 oper 再抄一遍 Unmarshal）。

type capHandler struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (h *capHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capHandler) Handle(_ context.Context, r slog.Record) error {
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

func (h *capHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capHandler) WithGroup(string) slog.Handler      { return h }

func (h *capHandler) drain() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.buf.String()
	h.buf.Reset()
	return s
}

func TestParseThresholdsWarnsWhenUnreadable(t *testing.T) {
	old := slog.Default()
	h := &capHandler{}
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(old) })

	// 正控：合法 JSON 按用户填的数走，且不出声。
	if got := ParseThresholds(`{"cpu_usage":50}`); got.CPUUsage != 50 {
		t.Fatalf("合法阈值要读出 50，实际 %v", got.CPUUsage)
	}
	if out := h.drain(); out != "" {
		t.Errorf("合法 JSON 不该出声，实际 %q", out)
	}

	// 缺陷本体：非法 JSON 回默认可以，但不许全程静默。
	got := ParseThresholds("{cpu_usage:50}")
	if got.CPUUsage != 90 {
		t.Errorf("读不出来时要按内置默认 90，实际 %v", got.CPUUsage)
	}
	logged := h.drain()
	if !strings.Contains(logged, "cpu_usage") {
		t.Errorf("静默回默认必须留痕并带原文片段（靠这条才能定位是哪台设备），实际 %q", logged)
	}

	// 顶层不是对象（数组）也属于「读不出来」。
	ParseThresholds(`[{"cpu_usage":50}]`)
	if out := h.drain(); !strings.Contains(out, "cpu_usage") {
		t.Errorf("非对象阈值也要留痕，实际 %q", out)
	}
}

func TestThresholdsReadableMatchesConsumer(t *testing.T) {
	if ThresholdsReadable("{cpu_usage:50}") {
		t.Errorf("非法 JSON 必须判为读不出来（否则写进去就等于没写）")
	}
	if ThresholdsReadable(`[{"cpu_usage":50}]`) {
		t.Errorf("顶层不是对象要判读不出来：ParseThresholds 对它也是静默回默认")
	}
	// 正控：空串 = 交给内置默认，是设计值；对象 = 可读。
	if !ThresholdsReadable("") {
		t.Errorf("空串要放行（Web 对没有自定义配置的设备就发空串）")
	}
	if !ThresholdsReadable("   ") {
		t.Errorf("纯空白等同空串")
	}
	if !ThresholdsReadable(`{"offline":false,"temp":70}`) {
		t.Errorf("合法对象要判可读")
	}
}
