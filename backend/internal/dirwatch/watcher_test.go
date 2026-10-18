package dirwatch

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/transfer"
)

func TestScanAndProcessSkipsTempAndNonMedia(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "Movie.2024.mkv"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "incomplete.part"), []byte("x"), 0o644) // 临时扩展名
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)       // 非媒体

	var mu sync.Mutex
	var got []transfer.FileInput
	w := New(Config{DebounceMs: 50}, func(ctx context.Context, srcDir string, files []transfer.FileInput) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, files...)
	})
	w.scanAndProcess(context.Background(), dir)

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("want 1 media file (跳过临时/非媒体), got %d: %+v", len(got), got)
	}
	if filepath.Base(got[0].Path) != "Movie.2024.mkv" {
		t.Errorf("unexpected file: %s", got[0].Path)
	}
}

func TestScanAndProcessTTLDedup(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "A.mkv"), []byte("x"), 0o644)
	var n int
	w := New(Config{DebounceMs: 50}, func(ctx context.Context, srcDir string, files []transfer.FileInput) { n++ })
	w.scanAndProcess(context.Background(), dir)
	w.scanAndProcess(context.Background(), dir) // 10s TTL 内应被去重
	if n != 1 {
		t.Errorf("want process once (TTL 去重), got %d", n)
	}
}

// TestScanAndProcessDiscoversM2TS 发现范围必须覆盖引擎可整理的全部视频（.m2ts 为 BD
// remux 常见容器）：旧实现 dirwatch 自有清单含 .m2ts 而 transfer 不认，反复失败。
func TestScanAndProcessDiscoversM2TS(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "Movie.2024.2160p.REMUX.m2ts"), []byte("x"), 0o644)
	var got []transfer.FileInput
	w := New(Config{DebounceMs: 50}, func(ctx context.Context, srcDir string, files []transfer.FileInput) {
		got = files
	})
	w.scanAndProcess(context.Background(), dir)
	if len(got) != 1 {
		t.Fatalf(".m2ts 应被发现, got %d files", len(got))
	}
}
