package chain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDirectStrmFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mkv.strm")
	if err := os.WriteFile(p, []byte("/api/v1/cloud/direct/3/媒体库/电影/a.mkv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, driveID, cp, ok := parseDirectStrmFile(p)
	if !ok || driveID != 3 || cp != "/媒体库/电影/a.mkv" {
		t.Errorf("parse failed: ok=%v drive=%d cp=%q", ok, driveID, cp)
	}
	// 挂载路径型（非本端 direct）应返回 false
	p2 := filepath.Join(dir, "b.mkv.strm")
	os.WriteFile(p2, []byte("/mnt/115/媒体库/b.mkv"), 0o644)
	if _, _, _, ok := parseDirectStrmFile(p2); ok {
		t.Error("mount-path strm should not parse as direct")
	}
	// 不存在
	if _, _, _, ok := parseDirectStrmFile(filepath.Join(dir, "none.strm")); ok {
		t.Error("missing file should be false")
	}
}
