package event

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestBroadcastFanout(t *testing.T) {
	bus := New()
	var hits int32
	var mu sync.Mutex
	got := map[string]bool{}
	for i := 0; i < 5; i++ {
		bus.Subscribe("evt", 0, func(ctx context.Context, e Event) {
			atomic.AddInt32(&hits, 1)
			mu.Lock()
			got[e.Data["k"].(string)] = true
			mu.Unlock()
		})
	}
	bus.Publish(context.Background(), "evt", map[string]any{"k": "v"})
	if atomic.LoadInt32(&hits) != 5 {
		t.Fatalf("expected 5 handler hits, got %d", hits)
	}
	if !got["v"] {
		t.Error("data not delivered")
	}
}

func TestBroadcastPanicIsolation(t *testing.T) {
	bus := New()
	var ok int32
	bus.Subscribe("evt", 0, func(ctx context.Context, e Event) { panic("boom") })
	bus.Subscribe("evt", 0, func(ctx context.Context, e Event) { atomic.AddInt32(&ok, 1) })
	bus.Publish(context.Background(), "evt", nil)
	if atomic.LoadInt32(&ok) != 1 {
		t.Fatal("panicking handler should not affect others")
	}
}

func TestChainOrderAndMutation(t *testing.T) {
	bus := New()
	var order []int
	bus.SubscribeChain("evt", 2, func(ctx context.Context, e Event) (Event, error) {
		order = append(order, 2)
		e.Data["v"] = e.Data["v"].(int) + 1
		return e, nil
	})
	bus.SubscribeChain("evt", 1, func(ctx context.Context, e Event) (Event, error) {
		order = append(order, 1)
		e.Data["v"] = e.Data["v"].(int) * 10
		return e, nil
	})
	out, err := bus.PublishChain(context.Background(), "evt", map[string]any{"v": 1})
	if err != nil {
		t.Fatal(err)
	}
	// prio 1 先跑：1*10=10；prio 2 后跑：10+1=11
	if order[0] != 1 || order[1] != 2 {
		t.Errorf("order = %v", order)
	}
	if out["v"].(int) != 11 {
		t.Errorf("v = %v, want 11", out["v"])
	}
}
