package filemanager

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocal_NewValidatesRoot(t *testing.T) {
	if _, err := NewLocal(""); err == nil {
		t.Fatal("空根目录应报错")
	}
	if _, err := NewLocal("   "); err == nil {
		t.Fatal("空白根目录应报错")
	}
	if _, err := NewLocal(filepath.Join(os.TempDir(), "ptpatronus-nonexistent-xyz-zzz")); err == nil {
		t.Fatal("不存在的目录应报错")
	}
	tmp, _ := os.CreateTemp("", "ptpatronus-notdir-*")
	tmp.Close()
	defer os.Remove(tmp.Name())
	if _, err := NewLocal(tmp.Name()); err == nil {
		t.Fatal("非目录根路径应报错")
	}
}

func TestLocal_CRUD(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644)
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("world"), 0o644)

	b, err := NewLocal(root)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	ctx := context.Background()

	// List 根：返回虚拟路径 /name
	entries, err := b.List(ctx, "/")
	if err != nil {
		t.Fatalf("List /: %v", err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
		if e.Path != "/"+e.Name {
			t.Errorf("entry path = %q, want /%s", e.Path, e.Name)
		}
	}
	if !names["a.txt"] || !names["sub"] {
		t.Fatalf("List 缺项: %v", names)
	}

	// Upload → Download 往返
	if err := b.Upload(ctx, "/sub/c.txt", bytes.NewReader([]byte("uploaded"))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	rc, err := b.Download(ctx, "/sub/c.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(rc); err != nil {
		t.Fatalf("read: %v", err)
	}
	rc.Close()
	if buf.String() != "uploaded" {
		t.Fatalf("download content = %q", buf.String())
	}

	// Mkdir 含父级
	if err := b.Mkdir(ctx, "/newdir/nested"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(root, "newdir", "nested")); err != nil || !fi.IsDir() {
		t.Fatalf("Mkdir 后目录不存在")
	}

	// Delete
	if err := b.Delete(ctx, "/a.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("Delete 后文件仍存在")
	}
}

// TestLocal_PathEscape 路径穿越防御：../ 与反斜杠变体必须被钳制在 root 内，绝不逃出。
func TestLocal_PathEscape(t *testing.T) {
	root := t.TempDir()
	b, err := NewLocal(root)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	// 在 root 上层放一个诱饵文件，若逃出会被读到
	parent := filepath.Dir(root)
	bait := filepath.Join(parent, "ptpatronus-bait.txt")
	os.WriteFile(bait, []byte("secret"), 0o644)
	defer os.Remove(bait)

	escapes := []string{
		"/../../../" + filepath.Base(bait),
		"/..%2f..%2fetc",
		`\..\..\` + filepath.Base(bait),
		"/sub/../../" + filepath.Base(bait),
	}
	for _, p := range escapes {
		full, err := b.resolve(p)
		if err == nil {
			// 解析成功时，结果必须仍在 root 内（rel 不以 .. 开头）
			rel, rerr := filepath.Rel(root, full)
			if rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Fatalf("穿越路径 %q 解析到 root 外: %s (rel=%q)", p, full, rel)
			}
			// 且不能等于诱饵文件
			if full == bait {
				t.Fatalf("穿越路径 %q 命中 root 外诱饵文件", p)
			}
		}
		// 下载穿越路径必须失败或读不到诱饵内容
		rc, derr := b.Download(context.Background(), p)
		if derr == nil {
			buf, _ := io.ReadAll(rc)
			rc.Close()
			if strings.Contains(string(buf), "secret") {
				t.Fatalf("穿越路径 %q 读到了 root 外诱饵内容", p)
			}
		}
	}
}
