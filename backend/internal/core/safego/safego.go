package safego

import (
	"log/slog"
	"runtime/debug"
)

// Go 启动一个带 panic 兜底的 goroutine：panic 时记录堆栈而非崩溃整个进程。
// name 为日志标识（调用方语义，如 "monitor.collectOne"），便于定位。
// 用法：safego.Go("site.snapshot", func() { ... })
func Go(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panic recovered",
					"name", name, "panic", r, "stack", string(debug.Stack()))
			}
		}()
		fn()
	}()
}
