package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLocal_SymlinkEscape_Rejected 验证 root 内指向外部的符号链接不会越界。
// 在支持符号链接的平台（Unix 或有权限的 Windows）创建 root/link -> /tmp（外部），
// resolve 应拒绝。Windows 无符号链接权限时 skip（CI Linux 可跑）。
func TestLocal_SymlinkEscape_Rejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir() // root 外的目录
	// 在 root 内创建一个文件供诱饵
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("external-secret"), 0o644)

	// 尝试创建符号链接 root/escape -> outside
	linkPath := filepath.Join(root, "escape")
	err := os.Symlink(outside, linkPath)
	if err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("Windows 符号链接需要开发模式或管理员权限，跳过（CI Linux 验证）")
		}
		t.Fatalf("创建符号链接失败: %v", err)
	}

	b, err := NewLocal(root)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	// 通过符号链接尝试读取外部文件
	_, rerr := b.resolve("/escape/secret.txt")
	if rerr == nil {
		t.Fatal("通过符号链接解析到外部路径应被拒绝")
	}
	if !strings.Contains(rerr.Error(), "symlink escapes root") {
		t.Fatalf("错误应含 'symlink escapes root'，got: %v", rerr)
	}

	// Download 也应失败（不读到外部内容）
	rc, derr := b.Download(context.Background(), "/escape/secret.txt")
	if derr == nil {
		rc.Close()
		t.Fatal("通过符号链接下载外部文件应失败")
	}
}

// TestLocal_SymlinkInsideRoot_Allowed root 内部正常路径不受 EvalSymlinks 影响。
func TestLocal_SymlinkInsideRoot_Allowed(t *testing.T) {
	root := t.TempDir()
	// root 内正常子目录
	subdir := filepath.Join(root, "sub")
	os.MkdirAll(subdir, 0o755)
	os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("ok"), 0o644)

	b, err := NewLocal(root)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	full, rerr := b.resolve("/sub/file.txt")
	if rerr != nil {
		t.Fatalf("root 内正常路径不应被拒: %v", rerr)
	}
	if !strings.HasSuffix(full, filepath.Join("sub", "file.txt")) {
		t.Fatalf("解析路径不对: %s", full)
	}
}
