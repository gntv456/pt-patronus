package auth

import (
	"testing"
	"time"
)

func TestTicketSingleUse(t *testing.T) {
	s := NewTicketStore()
	tk, exp, err := s.Issue(7)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if exp.IsZero() {
		t.Fatal("exp should be set")
	}
	uid, ok := s.Consume(tk)
	if !ok || uid != 7 {
		t.Fatalf("first consume should succeed: uid=%d ok=%v", uid, ok)
	}
	if _, ok := s.Consume(tk); ok {
		t.Fatal("second consume should fail (single-use)")
	}
	if _, ok := s.Consume(""); ok {
		t.Fatal("empty ticket should fail")
	}
	if _, ok := s.Consume("nonexistent"); ok {
		t.Fatal("unknown ticket should fail")
	}
}

func TestTicketExpiry(t *testing.T) {
	s := &TicketStore{tickets: map[string]ticket{}, ttl: time.Millisecond, lastSweep: time.Now()}
	tk, _, err := s.Issue(1)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, ok := s.Consume(tk); ok {
		t.Fatal("expired ticket should fail")
	}
}
