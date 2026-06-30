package log

import (
	"log/slog"
	"testing"
)

// TestRingBufferClear 验证清空：Recent 归零，且清空后 Push 仍正常（不越界、计数正确）。
func TestRingBufferClear(t *testing.T) {
	r := &RingBuffer{cap: 4, buf: make([]LogEntry, 0, 4)}
	r.Push(LogEntry{Level: "INFO", Message: "a"})
	r.Push(LogEntry{Level: "INFO", Message: "b"})
	if got := r.Recent(0, slog.LevelInfo); len(got) != 2 {
		t.Fatalf("before clear: want 2, got %d", len(got))
	}

	r.Clear()
	if got := r.Recent(0, slog.LevelInfo); len(got) != 0 {
		t.Fatalf("after clear: want 0, got %d", len(got))
	}

	// 清空后仍可正常 Push
	r.Push(LogEntry{Level: "INFO", Message: "c"})
	got := r.Recent(0, slog.LevelInfo)
	if len(got) != 1 || got[0].Message != "c" {
		t.Fatalf("after clear+push: want [c], got %+v", got)
	}
}
