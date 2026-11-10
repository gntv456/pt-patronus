// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package event

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishAsyncDelivers(t *testing.T) {
	bus := New()
	var hits int32
	done := make(chan struct{})
	bus.Subscribe("hot", 0, func(_ context.Context, e Event) {
		if e.Data["k"] == "v" {
			atomic.AddInt32(&hits, 1)
		}
		close(done)
	})
	bus.PublishAsync(context.Background(), "hot", map[string]any{"k": "v"})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("async handler 未执行")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits = %d", hits)
	}
}

func TestPublishAsyncDoesNotBlock(t *testing.T) {
	bus := New()
	release := make(chan struct{})
	bus.Subscribe("hot", 0, func(_ context.Context, _ Event) { <-release })
	start := time.Now()
	bus.PublishAsync(context.Background(), "hot", nil)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("PublishAsync 阻塞了 %v", elapsed)
	}
	close(release)
}

func TestPublishAsyncPanicIsolation(t *testing.T) {
	bus := New()
	done := make(chan struct{})
	bus.Subscribe("hot", 0, func(_ context.Context, _ Event) { panic("boom") })
	bus.Subscribe("hot", 0, func(_ context.Context, _ Event) { close(done) })
	bus.PublishAsync(context.Background(), "hot", nil)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("panic 后后续 handler 未执行")
	}
}
