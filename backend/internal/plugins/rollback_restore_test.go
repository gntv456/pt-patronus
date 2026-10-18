package plugins

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUnder(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// 回滚中途失败时，那份被改名出去的旧目录必须放回来；放回这件事本身有两个确定分支，
// 这里把它们钉住（#269：从前放回失败是 `_ = os.Rename(...)` 的丢弃写，界面只说「参数有误」）。
func TestRestoreRollbackBackupPutsOldVersionBack(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "demo")
	backup := dst + ".rollback-20261001120000"
	writeUnder(t, filepath.Join(dst, "half-written.py"), "broken")
	writeUnder(t, filepath.Join(backup, "plugin.py"), "old-version")

	if err := restoreRollbackBackup(dst, backup); err != nil {
		t.Fatalf("放回成功却回了错误: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dst, "plugin.py"))
	if err != nil || string(b) != "old-version" {
		t.Errorf("旧版本没回到原位: err=%v content=%q", err, b)
	}
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("放回是改名，临时备份目录不该还在: err=%v", err)
	}
}

func TestRestoreRollbackBackupWithoutPriorDirIsNotALoss(t *testing.T) {
	// 新装插件首次回滚失败：磁盘上原本就没有该目录，清空即回到初态，
	// 不能报成「旧版本也丢了」——那会让人去找一份根本不存在的备份。
	root := t.TempDir()
	dst := filepath.Join(root, "demo")
	writeUnder(t, filepath.Join(dst, "half.py"), "x")

	if err := restoreRollbackBackup(dst, dst+".rollback-20261001120000"); err != nil {
		t.Fatalf("没有旧目录时不该报错: %v", err)
	}
	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("写了一半的新目录应被清掉: err=%v", err)
	}
}

// 放回失败的那条分支要真把备份留着：源码层钉住「丢弃的写」已消失、RemoveAll 已改成有条件。
func TestRollbackKeepsBackupWhenRestoreFails(t *testing.T) {
	raw, err := os.ReadFile("releases.go")
	if err != nil {
		t.Fatalf("读 releases.go: %v", err)
	}
	src := string(raw)
	if n := strings.Count(src, "_ = os.Rename(backup, dst)"); n != 0 {
		t.Errorf("仍有 %d 处丢弃的放回失败——旧目录会被随后的 RemoveAll 一起删掉", n)
	}
	if !strings.Contains(src, "if !keepBackupOnFailure {") {
		t.Error("RemoveAll(backup) 未改成有条件：放回失败时那份唯一完好的旧代码会被删掉")
	}
	if n := strings.Count(src, "defer os.RemoveAll(backup)"); n != 0 {
		t.Errorf("仍有 %d 处无条件 defer RemoveAll(backup)", n)
	}
}
