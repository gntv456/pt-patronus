package ws

import (
	"testing"
	"time"
)

func testHubWithClient(t *testing.T, cap int) (*Hub, *Client) {
	t.Helper()
	h := NewHub()
	c := &Client{send: make(chan []byte, cap)}
	h.register(c)
	return h, c
}

func TestBatchFlushPreservesOrder(t *testing.T) {
	h, c := testHubWithClient(t, 10)
	b := NewBatch(h)
	b.Add([]byte("a")); b.Add([]byte("b")); b.Add([]byte("c"))
	if b.Len() != 3 { t.Fatalf("Len = %d", b.Len()) }
	b.Flush()
	if b.Len() != 0 { t.Fatal("Flush 后未清空") }
	for _, want := range []string{"a", "b", "c"} {
		if got := string(<-c.send); got != want { t.Fatalf("got %q want %q", got, want) }
	}
}

func TestBatchFlushNonBlockingOnFullChannel(t *testing.T) {
	h, _ := testHubWithClient(t, 1)
	b := NewBatch(h)
	for i := 0; i < 100; i++ { b.Add([]byte("x")) }
	done := make(chan struct{})
	go func() { b.Flush(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Flush 在慢连接上阻塞了")
	}
}

func TestBatchFlushEmpty(t *testing.T) {
	h := NewHub()
	NewBatch(h).Flush()
}