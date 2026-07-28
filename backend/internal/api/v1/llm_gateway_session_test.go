package v1

import (
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

func newGatewayForTest(t *testing.T) *LLMGatewayHandler {
	t.Helper()
	var n uint
	return &LLMGatewayHandler{
		convCreate: func(c *models.AgentConversation) error {
			n++
			c.ID = n
			return nil
		},
		sessions: map[string]sessionEntry{},
	}
}

// 无 session 每次新建；同 session 复用同一 convID；不同 session 互相隔离。
func TestConvForSessionReusesAndIsolates(t *testing.T) {
	h := newGatewayForTest(t)

	a, err := h.convForSession("")
	if err != nil || a == 0 {
		t.Fatalf("empty session: convID=%d err=%v", a, err)
	}
	b, _ := h.convForSession("")
	if a == b {
		t.Fatal("empty sessions should create distinct conversations")
	}

	c1, _ := h.convForSession("sess-1")
	c2, _ := h.convForSession("sess-1")
	if c1 != c2 {
		t.Fatalf("same session should reuse convID: %d vs %d", c1, c2)
	}

	d, _ := h.convForSession("sess-2")
	if d == c1 {
		t.Fatal("different sessions must have distinct convIDs")
	}
}

// TTL 过期后同 session 应重建会话（防无限堆积陈旧映射）。
func TestConvForSessionTTLExpiry(t *testing.T) {
	h := newGatewayForTest(t)
	old, _ := h.convForSession("sess-ttl")
	h.sessionsMu.Lock()
	h.sessions["sess-ttl"] = sessionEntry{convID: old, last: time.Now().Add(-sessionTTL - time.Minute)}
	h.sessionsMu.Unlock()

	again, _ := h.convForSession("sess-ttl")
	if again == old {
		t.Fatal("expired session should create a new conversation")
	}
}
