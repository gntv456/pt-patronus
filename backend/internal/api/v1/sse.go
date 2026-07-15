package v1

import (
	"net/http"
	"sync"
)

// sseEmitter 线程安全地写一条 SSE 事件；返回 false 表示客户端已断开，调用方可据此停止。
type sseEmitter func(event string, data any) bool

// beginSSE 写 SSE 响应头（text/event-stream + 关闭 Nginx 缓冲，使每帧即时到前端）并返回一个
// 线程安全的 emit。供跨源/跨站聚合的流式端点：chain 层并发扇出、并发回调 emit，此处用互斥串行化
// 对同一 ResponseWriter 的写入（HTTP response writer 非并发安全）。emit 返回 false 表示客户端已断开，
// 后续写入被跳过（chain 仍会跑完，靠 request ctx 取消 + 各源独立超时自然收敛）。
func beginSSE(w http.ResponseWriter) sseEmitter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	var mu sync.Mutex
	stopped := false
	return func(event string, data any) bool {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			return false
		}
		if !writeSSE(w, event, data) {
			stopped = true
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}
}
