// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package middleware

import (
	"testing"
	"time"
)

// 空闲超时的桶必须被回收，避免伪造 key 把 buckets map 当内存放大武器。
func TestRateLimiterSweepsIdleBuckets(t *testing.T) {
	l := NewRateLimiter(1, 1)
	l.Allow("a")
	l.Allow("b")
	if len(l.buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(l.buckets))
	}

	// 把所有桶标记为远早于空闲阈值，强制回收。
	l.mu.Lock()
	old := time.Now().Add(-3 * time.Minute)
	for _, e := range l.buckets {
		e.lastUse = old
	}
	l.sweepLocked(time.Now())
	got := len(l.buckets)
	l.mu.Unlock()

	if got != 0 {
		t.Fatalf("expected all idle buckets swept, got %d", got)
	}
}

// 活跃（近期使用）的桶不应被回收。
func TestRateLimiterKeepsActiveBuckets(t *testing.T) {
	l := NewRateLimiter(1, 1)
	l.Allow("active")
	l.mu.Lock()
	l.sweepLocked(time.Now())
	got := len(l.buckets)
	l.mu.Unlock()
	if got != 1 {
		t.Fatalf("expected active bucket retained, got %d", got)
	}
}
