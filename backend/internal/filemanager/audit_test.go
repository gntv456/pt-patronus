package filemanager

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// P2-010: local_extra.go unzipTo 缺少解压大小上限
// 与 market_archive.go 的 unzipSafe 不同，unzipTo 使用无限制 io.Copy
// 恶意 zip 可用压缩比极高的文件（如全零）炸毁磁盘
func TestUnzipToNoSizeLimit(t *testing.T) {
	// 验证 unzipTo 函数签名不含 LimitReader（通过源码审计确认）
	// 这里验证功能：正常小文件能解压
	tmp := t.TempDir()
	src := filepath.Join(tmp, "test.txt")
	os.WriteFile(src, []byte("hello"), 0o644)

	zipPath := filepath.Join(tmp, "test.zip")
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("test.txt")
	w.Write([]byte("hello"))
	zw.Close()
	os.WriteFile(zipPath, buf.Bytes(), 0o644)

	dst := filepath.Join(tmp, "out")
	if err := unzipTo(zipPath, dst); err != nil {
		t.Fatalf("unzipTo failed: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dst, "test.txt"))
	if string(data) != "hello" {
		t.Errorf("解压内容不匹配: %q", data)
	}
	t.Log("P2-010 确认: unzipTo 无 io.LimitReader，恶意 zip 可炸盘（对比 unzipSafe 有 32MB/96MB 上限）")
}

// P2-011: unzipTo 的 zip-slip 检测用 strings.HasPrefix
// 比起 isSubpath 的 filepath.Rel 检测更弱
// 虽然当前能拦截 ../，但在 Windows 混合分隔符场景可能绕过
func TestUnzipToZipSlipCheckUsesHasPrefix(t *testing.T) {
	// 验证 zip-slip 被拦截
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "evil.zip")
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("../../../etc/passwd")
	w.Write([]byte("evil"))
	zw.Close()
	os.WriteFile(zipPath, buf.Bytes(), 0o644)

	dst := filepath.Join(tmp, "out")
	err := unzipTo(zipPath, dst)
	if err == nil {
		t.Fatal("zip-slip 应被拦截")
	}
	if !strings.Contains(err.Error(), "slip") {
		t.Errorf("错误应含 'slip': %v", err)
	}
	t.Log("zip-slip 基本拦截有效，但 strings.HasPrefix 比 isSubpath 弱")
}

// P2-012: Local Delete 拒绝根目录但允许深层递归删除
func TestLocalDeleteRejectsRoot(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o644)

	l, err := NewLocal(root)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	// 删除根目录应被拒绝
	err = l.Delete(context.Background(), "/")
	if err == nil {
		t.Fatal("删除根目录应被拒绝")
	}

	// 删除根目录变体
	for _, p := range []string{"", "/", "/.", "/.."} {
		err := l.Delete(context.Background(), p)
		if err == nil {
			t.Errorf("删除根目录变体 %q 应被拒绝", p)
		}
	}
}

// P3-013: sanitizeNewName 允许含路径分隔符的名称被 Base 钳制
// 但 "a/b" 会被 Base 处理为 "b"，静默丢掉目录部分（容错但不告警）
func TestSanitizeNewNameStripsPath(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"newname.txt", "newname.txt"},
		{"subdir/file.txt", "file.txt"},   // Base 钳制，静默丢 subdir
		{"../escape", "escape"},           // Base 钳制
		{"  spaces  ", "spaces"},          // TrimSpace
	}
	for _, c := range cases {
		got, err := sanitizeNewName(c.input)
		if err != nil {
			t.Errorf("sanitizeNewName(%q) error: %v", c.input, err)
			continue
		}
		if got != c.want {
			t.Errorf("sanitizeNewName(%q) = %q, want %q", c.input, got, c.want)
		}
	}
	// 非法名
	for _, bad := range []string{"", ".", ".."} {
		if _, err := sanitizeNewName(bad); err == nil {
			t.Errorf("sanitizeNewName(%q) 应返回错误", bad)
		}
	}
}

// P3-014: resolvePath 对空 base 的行为
// base 为空时，../etc/passwd 不被约束，直接返回 /etc/passwd
// 这对远程后端（agent/webdav）可能是期望行为，但对 local 不使用 resolvePath
func TestResolvePathEmptyBaseNoConstraint(t *testing.T) {
	got := resolvePath("", "../../../etc/passwd")
	if got != "/etc/passwd" {
		t.Errorf("空 base 时应为 /etc/passwd, got %q", got)
	}
	// 这意味着如果远程后端 basePath 配置为空，路径穿越不受约束
	t.Log("P3-014: 空 basePath 时远程文件操作无路径约束（依赖远程端自身限制）")
}
