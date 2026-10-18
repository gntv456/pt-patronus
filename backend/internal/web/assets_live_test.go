package web

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLiveAutoDownload 真连上游跑一遍完整的「自动获取」链路。
//
// 标注为短测试之外：需要外网，故默认跳过，`go test -run TestLiveAutoDownload` 或
// 设 JB_LIVE=1 才跑。目的是验证真实产物与锁定校验和一致（上游若重新发布导致
// 字节变化，这里会立即暴露，而不是等到用户预览失败）。
func TestLiveAutoDownload(t *testing.T) {
	if os.Getenv("JB_LIVE") != "1" {
		t.Skip("需要外网，设 JB_LIVE=1 才跑")
	}
	dir := t.TempDir()
	s := NewAssetStore(dir)
	if s.Ready() {
		t.Fatal("空目录不应就位")
	}
	if err := s.EnsureAll(); err != nil {
		t.Fatalf("自动获取失败：%v", err)
	}
	if !s.Ready() {
		t.Fatal("获取后应就位")
	}
	entries, err := os.ReadDir(filepath.Join(dir, "jessibuca"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(Assets) {
		t.Fatalf("落盘 %d 个文件，应为 %d", len(entries), len(Assets))
	}
	for _, st := range s.Status() {
		t.Logf("%-14s %8d ready=%v", st.File, st.Bytes, st.Ready)
	}
}
