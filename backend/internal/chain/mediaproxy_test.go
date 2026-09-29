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
	_, driveID, cp, sign, ok := parseDirectStrmFile(p)
	if !ok || driveID != 3 || cp != "/媒体库/电影/a.mkv" {
		t.Errorf("parse failed: ok=%v drive=%d cp=%q", ok, driveID, cp)
	}
	if sign != "" {
		t.Errorf("无签名内容 sign 应为空, got %q", sign)
	}
	// 二轮 P1-2 回归：带 ?sign= 的内容路径必须剥离 query、签名串独立返回
	ps := filepath.Join(dir, "s.mkv.strm")
	os.WriteFile(ps, []byte("http://nas:8080/api/v1/cloud/direct/9/媒体/s.mkv?sign=1790000000.abc123\n"), 0o644)
	_, d2, cp2, sign2, ok2 := parseDirectStrmFile(ps)
	if !ok2 || d2 != 9 || cp2 != "/媒体/s.mkv" {
		t.Errorf("signed parse failed: ok=%v drive=%d cp=%q", ok2, d2, cp2)
	}
	if sign2 != "1790000000.abc123" {
		t.Errorf("sign 解析错: %q", sign2)
	}
	// 挂载路径型（非本端 direct）应返回 false
	p2 := filepath.Join(dir, "b.mkv.strm")
	os.WriteFile(p2, []byte("/mnt/115/媒体库/b.mkv"), 0o644)
	if _, _, _, _, ok := parseDirectStrmFile(p2); ok {
		t.Error("mount-path strm should not parse as direct")
	}
	// 不存在
	if _, _, _, _, ok := parseDirectStrmFile(filepath.Join(dir, "none.strm")); ok {
		t.Error("missing file should be false")
	}
}
