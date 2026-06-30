// Package log configures the global slog logger + 内存环形缓冲（供 /system/logs 查看）。
package log

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"
)

// LogEntry 一条日志的结构化快照（供 API 返回）。
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"` // DEBUG/INFO/WARN/ERROR
	Message string    `json:"message"`
	Attrs   []KV      `json:"attrs,omitempty"`
}

// KV 一个日志属性键值对（值转字符串）。
type KV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Ring 全局环形缓冲单例（Init 前也可 Push，零值安全）。
var Ring = &RingBuffer{cap: 2000, buf: make([]LogEntry, 0, 2000)}

// RingBuffer 固定容量环形缓冲（最近 N 条），线程安全。
type RingBuffer struct {
	mu   sync.Mutex
	cap  int
	buf  []LogEntry
	head int
	full bool
}

// Push 追加一条（满后覆盖最旧）。
func (r *RingBuffer) Push(e LogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) < r.cap {
		r.buf = append(r.buf, e)
		return
	}
	r.buf[r.head] = e
	r.head = (r.head + 1) % r.cap
	r.full = true
}

// Recent 返回最近 n 条（最新在前），按 minLevel 过滤（level >= minLevel）。
func (r *RingBuffer) Recent(n int, minLevel slog.Level) []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ordered []LogEntry
	if r.full {
		ordered = make([]LogEntry, 0, r.cap)
		for i := 0; i < r.cap; i++ {
			ordered = append(ordered, r.buf[(r.head+i)%r.cap])
		}
	} else {
		ordered = append(ordered, r.buf...)
	}
	if n <= 0 || n > len(ordered) {
		n = len(ordered)
	}
	out := make([]LogEntry, 0, n)
	for i := len(ordered) - 1; i >= 0 && len(out) < n; i-- {
		if parseLevelStr(ordered[i].Level) >= minLevel {
			out = append(out, ordered[i])
		}
	}
	return out
}

// Clear 清空缓冲（head 归零、full 复位）；新日志照常 Push。不影响 stdout/落盘。
func (r *RingBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = r.buf[:0]
	r.head = 0
	r.full = false
}

func parseLevelStr(s string) slog.Level {
	switch s {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ParseLevel 把请求参数 level(debug/info/warn/error)转为 slog.Level；空/非法→Info。
func ParseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ringHandler 包装 slog.Handler：每条 Record 推入环形缓冲后透传给 inner（stdout）。
// 零侵入——所有 slog.Info/Warn/Error 自动进环，业务代码无需改动。
type ringHandler struct {
	inner slog.Handler
	ring  *RingBuffer
}

func (h *ringHandler) Enabled(ctx context.Context, lv slog.Level) bool {
	return h.inner.Enabled(ctx, lv)
}
func (h *ringHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ringHandler{inner: h.inner.WithAttrs(attrs), ring: h.ring}
}
func (h *ringHandler) WithGroup(name string) slog.Handler {
	return &ringHandler{inner: h.inner.WithGroup(name), ring: h.ring}
}
func (h *ringHandler) Handle(ctx context.Context, r slog.Record) error {
	entry := LogEntry{Time: r.Time, Level: r.Level.String(), Message: r.Message}
	r.Attrs(func(a slog.Attr) bool {
		entry.Attrs = append(entry.Attrs, KV{Key: a.Key, Value: valToString(a.Value)})
		return true
	})
	h.ring.Push(entry)
	return h.inner.Handle(ctx, r)
}

// valToString 把 slog.Value（任意类型）转字符串。
func valToString(v slog.Value) string {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return strconv.FormatInt(v.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(v.Float64(), 'f', -1, 64)
	case slog.KindBool:
		return strconv.FormatBool(v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindAny:
		return fmt.Sprintf("%v", v.Any())
	}
	return ""
}

// Init configures the global slog logger (stdout + 环形缓冲) and returns it.
// level: "debug" | "info" | "warn" | "error".
func Init(env, level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	inner := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	handler := &ringHandler{inner: inner, ring: Ring}
	logger := slog.New(handler).With(
		slog.String("app", "ptpatronus"),
		slog.String("env", env),
	)
	slog.SetDefault(logger)
	return logger
}
