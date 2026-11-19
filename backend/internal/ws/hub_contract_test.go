// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package ws

import (
	"sync"
	"testing"
	"time"
)

// newTestClient 构造仅用于 Hub 契约测试的 Client（不建真实 websocket 连接）。
func newTestClient(inc bool, buf int) *Client {
	return &Client{send: make(chan []byte, buf), inc: inc}
}

// 契约：零连接广播必须是空操作，不 panic。
func TestHubBroadcastNoClients(t *testing.T) {
	h := NewHub()
	h.Broadcast([]byte(`{"type":"notice"}`))
	h.BroadcastInc([]byte("legacy"), []byte("inc"))
	if h.Count() != 0 || h.HasLegacyClients() {
		t.Fatalf("unexpected hub state: count=%d legacy=%v", h.Count(), h.HasLegacyClients())
	}
}

// 契约：Broadcast 把同一帧投递给所有已注册客户端。
func TestHubBroadcastFansOut(t *testing.T) {
	h := NewHub()
	c1, c2 := newTestClient(false, 1), newTestClient(true, 1)
	h.register(c1)
	h.register(c2)
	payload := []byte(`{"type":"notice","data":{}}`)
	h.Broadcast(payload)
	for i, c := range []*Client{c1, c2} {
		select {
		case got := <-c.send:
			if string(got) != string(payload) {
				t.Fatalf("client %d got %s", i, got)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("client %d did not receive broadcast", i)
		}
	}
}

// 契约：BroadcastInc 按协议分发——旧客户端收 legacy，增量客户端收 inc。
func TestHubBroadcastIncCohorts(t *testing.T) {
	h := NewHub()
	legacy, inc := newTestClient(false, 1), newTestClient(true, 1)
	h.register(legacy)
	h.register(inc)
	h.BroadcastInc([]byte("LEGACY"), []byte("INC"))
	if got := <-legacy.send; string(got) != "LEGACY" {
		t.Fatalf("legacy client got %s", got)
	}
	if got := <-inc.send; string(got) != "INC" {
		t.Fatalf("inc client got %s", got)
	}
	if !h.HasLegacyClients() {
		t.Fatal("expected legacy client present")
	}
}

// 契约：某 cohort payload 为空时该 cohort 跳过，不影响另一 cohort。
func TestHubBroadcastIncEmptyCohortSkipped(t *testing.T) {
	h := NewHub()
	legacy, inc := newTestClient(false, 1), newTestClient(true, 1)
	h.register(legacy)
	h.register(inc)
	h.BroadcastInc(nil, []byte("INC-ONLY"))
	select {
	case got := <-legacy.send:
		t.Fatalf("legacy cohort should be skipped, got %s", got)
	default:
	}
	if got := <-inc.send; string(got) != "INC-ONLY" {
		t.Fatalf("inc client got %s", got)
	}
}

// 契约：慢客户端发送缓冲满时不得阻塞广播。
func TestHubBroadcastSlowClientNonBlocking(t *testing.T) {
	h := NewHub()
	slow := newTestClient(false, 0)
	h.register(slow)
	done := make(chan struct{})
	go func() { h.Broadcast([]byte("x")); close(done) }()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Broadcast blocked on a slow client")
	}
}

// 契约：unregister 幂等——并发重复注销不得 panic。
func TestHubUnregisterIdempotent(t *testing.T) {
	h := NewHub()
	c := newTestClient(false, 1)
	h.register(c)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); h.unregister(c) }()
	}
	wg.Wait()
	if h.Count() != 0 {
		t.Fatalf("count=%d, want 0", h.Count())
	}
}
