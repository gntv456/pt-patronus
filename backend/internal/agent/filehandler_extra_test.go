package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentArchiveExtract(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "src", "a.txt"), []byte("aaa"), 0o644)

	// zip
	if err := agentZip(filepath.Join(root, "src"), filepath.Join(root, "out.zip")); err != nil {
		t.Fatalf("agentZip: %v", err)
	}
	if err := agentUnzip(filepath.Join(root, "out.zip"), filepath.Join(root, "unzipped")); err != nil {
		t.Fatalf("agentUnzip: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "unzipped", "src", "a.txt"))
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(data) != "aaa" {
		t.Errorf("zip extracted = %q, want aaa", data)
	}

	// tar.gz
	if err := agentTarGz(filepath.Join(root, "src"), filepath.Join(root, "out.tar.gz")); err != nil {
		t.Fatalf("agentTarGz: %v", err)
	}
	if err := agentUntarGz(filepath.Join(root, "out.tar.gz"), filepath.Join(root, "untarred")); err != nil {
		t.Fatalf("agentUntarGz: %v", err)
	}
	data, _ = os.ReadFile(filepath.Join(root, "untarred", "src", "a.txt"))
	if string(data) != "aaa" {
		t.Errorf("tar extracted = %q, want aaa", data)
	}
}

func TestAgentZipSlip(t *testing.T) {
	root := t.TempDir()
	// 构造含 ../escape.txt 的 zip 较复杂，验证正常路径不误伤 + slip 判断逻辑
	if !agentSameOrUnder(filepath.Join(root, "a"), filepath.Join(root, "a", "b")) {
		t.Error("subdir should be under parent")
	}
	if agentSameOrUnder(filepath.Join(root, "a"), filepath.Join(root, "b")) {
		t.Error("sibling should not be under")
	}
}

func TestAgentDirSize(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "f1"), make([]byte, 100), 0o644)
	os.WriteFile(filepath.Join(root, "sub", "f2"), make([]byte, 200), 0o644)
	var total int64
	filepath.Walk(root, func(_ string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	if total != 300 {
		t.Errorf("dir size = %d, want 300", total)
	}
}
