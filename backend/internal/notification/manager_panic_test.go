// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package notification

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
)

// panickingChannel Send 现场 panic（模拟渠道解析畸形响应/空指针那类崩法），并计调用次数。
type panickingChannel struct {
	name  string
	kind  string
	calls *atomic.Int64
}

func (p panickingChannel) Name() string { return p.name }
func (p panickingChannel) Send(context.Context, Message) error {
	p.calls.Add(1)
	panic(p.kind)
}

// TestNotifyChannelPanicLeavesNamedRow 一个渠道崩掉时，扇出的每一行都必须「有名、有原因」。
// 修复前 safego 在更外层 recover ⇒ 崩掉的 goroutine 从没写 results[i]，槽位停在零值：
// 面板拿到一行「渠道名空 + 原因空」的失败（看不出是谁坏了、为什么坏），
// 而 notice.Service 用空名字去重试 ⇒ 匹配不到任何渠道，这个渠道此后再也不会被重发，
// #225 那条「重试仍没送达」的终局告警也就永远不响。
func TestNotifyChannelPanicLeavesNamedRow(t *testing.T) {
	var boomCalls atomic.Int64
	m := NewManager(
		successChannel{"ok-a"},
		panickingChannel{name: "boom", kind: "invalid memory address or nil pointer dereference", calls: &boomCalls},
		successChannel{"ok-c"},
	)
	res := m.Notify(context.Background(), Message{Title: "hi"})
	if len(res) != 3 {
		t.Fatalf("三个渠道必须有三行账面，got %d", len(res))
	}
	for _, r := range res {
		if r.Channel == "" {
			t.Errorf("账面里出现无名行（缺陷本体旧形状：崩掉的槽位没被写）：%+v", res)
			break
		}
	}
	var boom *SendResult
	for i := range res {
		if res[i].Channel == "boom" {
			boom = &res[i]
		}
	}
	if boom == nil {
		t.Fatalf("崩掉的渠道必须留下以自己名字命名的一行，got %+v", res)
	}
	if boom.Ok {
		t.Errorf("崩掉不能算送达成功，got %+v", *boom)
	}
	if !strings.Contains(boom.Error, "渠道异常") || !strings.Contains(boom.Error, "nil pointer") {
		t.Errorf("原因要点名「渠道异常」并带上现场信息，got %q", boom.Error)
	}
	// 其它渠道不受牵连（Manager 的并发隔离是本行的既有承诺）
	byName := map[string]SendResult{}
	for _, r := range res {
		byName[r.Channel] = r
	}
	for _, n := range []string{"ok-a", "ok-c"} {
		if r, ok := byName[n]; !ok || !r.Ok {
			t.Errorf("正常渠道 %s 要照常成功，got %+v", n, byName[n])
		}
	}
}

// TestPanicRowIsRetryable 用户级后果：失败行必须能喂给「只重发失败渠道」那条路。
// notice.Service 拿 SendResult 的渠道名调 NotifyChannels；名字是空串时匹配不到任何渠道，
// 于是重试静默什么都不做（这里直接断言崩掉的渠道真的被再次调用）。
func TestPanicRowIsRetryable(t *testing.T) {
	var boomCalls atomic.Int64
	boom := panickingChannel{name: "boom", kind: "panic on malformed response", calls: &boomCalls}
	m := NewManager(boom, successChannel{"ok"})

	ctx := context.Background()
	res := m.Notify(ctx, Message{Title: "hi"})
	var failed []string
	for _, r := range res {
		if !r.Ok {
			if r.Error == "" {
				t.Errorf("失败行必须带原因（空原因=重试与面板都读不出方向）：%+v", r)
			}
			failed = append(failed, r.Channel)
		}
	}
	before := boomCalls.Load()
	retried := m.NotifyChannels(ctx, failed, Message{Title: "retry"})
	if boomCalls.Load() <= before {
		t.Errorf("失败重试必须真的打到崩掉的那个渠道，got failed=%v calls=%d→%d 重试行=%+v",
			failed, before, boomCalls.Load(), retried)
	}
	for _, r := range retried {
		if r.Channel == "" {
			t.Errorf("重试账面同样不许出现无名行，got %+v", retried)
		}
	}
}
