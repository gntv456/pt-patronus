// Package event 提供事件总线：广播事件（一对多，并发扇出）与链式事件
// （按优先级串行的中间件链，可改写数据）。借鉴 MoviePilot 的 EventManager。
package event

import (
	"context"
	"log/slog"
	"sort"
	"sync"
)

// Event 是一个事件负载。
type Event struct {
	Type string
	Data map[string]any
}

// Copy 返回顶层 key 浅拷贝，隔离不同处理器的并发改写。
func (e Event) Copy() Event {
	d := make(map[string]any, len(e.Data))
	for k, v := range e.Data {
		d[k] = v
	}
	return Event{Type: e.Type, Data: d}
}

// Handler 处理一个广播事件。
type Handler func(ctx context.Context, e Event)

// ChainHandler 处理一个链式事件，返回（可能被改写的）事件与错误。
type ChainHandler func(ctx context.Context, e Event) (Event, error)

// Bus 是事件总线。
type Bus struct {
	mu        sync.RWMutex
	handlers  map[string][]handlerEntry
	chainHand map[string][]chainEntry
}

type handlerEntry struct {
	h    Handler
	prio int
}
type chainEntry struct {
	h    ChainHandler
	prio int
}

// New 构造一个事件总线。
func New() *Bus {
	return &Bus{
		handlers:  make(map[string][]handlerEntry),
		chainHand: make(map[string][]chainEntry),
	}
}

// Subscribe 订阅广播事件（priority 越小越先；广播实际并发执行）。
func (b *Bus) Subscribe(eventType string, prio int, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handlerEntry{h: h, prio: prio})
}

// Publish 并发扇出广播事件；每个 handler 独立 recover，互不影响。
func (b *Bus) Publish(ctx context.Context, eventType string, data map[string]any) {
	b.mu.RLock()
	handlers := make([]handlerEntry, len(b.handlers[eventType]))
	copy(handlers, b.handlers[eventType])
	b.mu.RUnlock()

	if len(handlers) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, he := range handlers {
		wg.Add(1)
		go func(he handlerEntry) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("event handler panic", "type", eventType, "panic", r)
				}
			}()
			he.h(ctx, Event{Type: eventType, Data: data}.Copy())
		}(he)
	}
	wg.Wait()
}

// SubscribeChain 订阅链式事件（按 priority 升序串行）。
func (b *Bus) SubscribeChain(eventType string, prio int, h ChainHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chainHand[eventType] = append(b.chainHand[eventType], chainEntry{h: h, prio: prio})
	sort.SliceStable(b.chainHand[eventType], func(i, j int) bool {
		return b.chainHand[eventType][i].prio < b.chainHand[eventType][j].prio
	})
}

// PublishChain 按优先级串行执行链式处理器，每个可改写 Data；任一出错即中止。
func (b *Bus) PublishChain(ctx context.Context, eventType string, data map[string]any) (map[string]any, error) {
	b.mu.RLock()
	chain := make([]chainEntry, len(b.chainHand[eventType]))
	copy(chain, b.chainHand[eventType])
	b.mu.RUnlock()

	e := Event{Type: eventType, Data: data}
	for _, ce := range chain {
		var err error
		e, err = ce.h(ctx, e)
		if err != nil {
			return e.Data, err
		}
	}
	return e.Data, nil
}
