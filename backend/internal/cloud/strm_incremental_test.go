package cloud

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStrmSkipExisting 增量模式：已存在的 .strm 直接跳过（内容变化不重写）。
func TestStrmSkipExisting(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "a.strm")
	if err := os.WriteFile(out, []byte("stale-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := GenerateOptions{CloudPath: "/电影", OutputDir: dir, Template: "{mount}{cloudpath}", MountRoot: "/mnt", KeepExt: false, SkipExisting: true}
	written, _, err := writeStrm(g, File{Name: "a.mkv", Path: "/电影/a.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Error("existing file should be skipped in incremental mode")
	}
	b, _ := os.ReadFile(out)
	if string(b) != "stale-content" {
		t.Errorf("content changed in skip mode: %q", b)
	}
	// 关闭增量：内容不同会重写
	g.SkipExisting = false
	written, _, err = writeStrm(g, File{Name: "a.mkv", Path: "/电影/a.mkv"})
	if err != nil || !written {
		t.Errorf("non-incremental should rewrite: written=%v err=%v", written, err)
	}
	b, _ = os.ReadFile(out)
	if string(b) != "/mnt/电影/a.mkv\n" {
		t.Errorf("rewritten content wrong: %q", b)
	}
}
