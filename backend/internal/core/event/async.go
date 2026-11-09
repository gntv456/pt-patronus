package event

import (
	"context"
	"log/slog"
)

// PublishAsync 高频路径专用广播（阶段 3「快慢分离」）。
//
// 与 Publish 的区别：
//   - 不为每个 handler 起 goroutine（原实现 N 个 goroutine + wg.Wait，尾延迟由最慢者决定）；
//   - 不复制 Event.Data（原实现每个 handler 一份 map 浅拷贝）；
//   - 不等待任何 handler 完成，fire-and-forget；单个后台 goroutine 串行执行并逐个 recover。
//
// 调用方承诺：data 在返回后不再被修改，且 handler 不改写共享负载。
// 冷路径（站点抓取/通知/工作流）继续用 Publish，保持「等齐 + 副本隔离」语义。
func (b *Bus) PublishAsync(ctx context.Context, eventType string, data map[string]any) {
	b.mu.RLock()
	// 仍需切片快照：Unsubscribe 会原地改底层数组，不能在解锁后直接持有。
	// 这里只复制 handler 条目（廉价），不再复制 map、不再起 N 个 goroutine。
	handlers := append([]handlerEntry(nil), b.handlers[eventType]...)
	b.mu.RUnlock()
	if len(handlers) == 0 {
		return
	}
	e := Event{Type: eventType, Data: data}
	go func() {
		for _, he := range handlers {
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("async event handler panic", "type", eventType, "panic", r)
					}
				}()
				he.h(ctx, e)
			}()
		}
	}()
}