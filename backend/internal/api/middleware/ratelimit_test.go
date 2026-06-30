package middleware

import "testing"

func TestRateLimiterBurst(t *testing.T) {
	l := NewRateLimiter(1, 2) // 突发 2
	if !l.Allow("1.2.3.4") {
		t.Error("first request should pass")
	}
	if !l.Allow("1.2.3.4") {
		t.Error("second request (within burst) should pass")
	}
	if l.Allow("1.2.3.4") {
		t.Error("third request should be rate-limited")
	}
}

func TestRateLimiterPerKey(t *testing.T) {
	l := NewRateLimiter(1, 1)
	if !l.Allow("a") {
		t.Error("a should pass")
	}
	if l.Allow("a") {
		t.Error("a second should be limited")
	}
	// 不同 key 独立
	if !l.Allow("b") {
		t.Error("b should pass independently")
	}
}
