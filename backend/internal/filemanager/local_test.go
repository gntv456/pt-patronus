// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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

// TestLocal_RenameMoveCopy 验证本机后端重命名 / 移动 / 复制（文件 + 目录递归）。
func TestLocal_RenameMoveCopy(t *testing.T) {
	dir := t.TempDir()
	b, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := b.Mkdir(ctx, "/sub"); err != nil {
		t.Fatal(err)
	}
	if err := b.Upload(ctx, "/a.txt", strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}

	// Rename：a.txt → a.new.txt
	if err := b.Rename(ctx, "/a.txt", "a.new.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.new.txt")); err != nil {
		t.Fatalf("rename 后新文件不存在: %v", err)
	}

	// Move：a.new.txt → /sub/（保留文件名）
	if err := b.Move(ctx, "/a.new.txt", "/sub"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "a.new.txt")); err != nil {
		t.Fatalf("move 后目标不存在: %v", err)
	}

	// Copy 文件：/sub/a.new.txt → /（保留文件名，源不删）
	if err := b.Copy(ctx, "/sub/a.new.txt", "/"); err != nil {
		t.Fatalf("copy file: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.new.txt"))
	if err != nil {
		t.Fatalf("copy 后根目录副本不存在: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("copy 内容不一致: %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "a.new.txt")); err != nil {
		t.Fatalf("copy 后源文件应保留: %v", err)
	}

	// Copy 目录：/sub → /dest/（递归）
	if err := b.Copy(ctx, "/sub", "/dest"); err != nil {
		t.Fatalf("copy dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dest", "sub", "a.new.txt")); err != nil {
		t.Fatalf("目录复制后子文件不存在: %v", err)
	}
}

// TestLocal_RenameNewNameClamped 验证 newName 含 ../ 时被 filepath.Base 钳制，不逃逸 rootDir。
func TestLocal_RenameNewNameClamped(t *testing.T) {
	dir := t.TempDir()
	b, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := b.Mkdir(ctx, "/sub"); err != nil {
		t.Fatal(err)
	}
	if err := b.Upload(ctx, "/sub/a.txt", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	// "../escape.txt" 经 Base → "escape.txt"，落到 /sub/escape.txt，不逃到 rootDir 之上。
	if err := b.Rename(ctx, "/sub/a.txt", "../../escape.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "escape.txt")); err != nil {
		t.Fatalf("newName 应被钳制到同目录: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.txt")); err == nil {
		t.Fatalf("newName 未被钳制，逃出了 rootDir")
	}
}

// TestParseMultiStatus_DirAndFile 回归 WebDAV 目录/文件识别：collection 标签存在→IsDir=true，
// getcontentlength→Size。防 `Collection != struct{}{}` 恒 false 的老 bug（所有项被当文件，文件夹点不进）。
func TestParseMultiStatus_DirAndFile(t *testing.T) {
	const resp = `<?xml version="1.0" encoding="utf-8"?>
<multistatus xmlns="DAV:">
  <response>
    <href>/dav/Movies/</href>
    <propstat><prop>
      <displayname>Movies</displayname>
      <resourcetype><collection/></resourcetype>
    </prop></propstat>
  </response>
  <response>
    <href>/dav/Movies/a.mkv</href>
    <propstat><prop>
      <displayname>a.mkv</displayname>
      <getcontentlength>4242424</getcontentlength>
      <resourcetype/>
    </prop></propstat>
  </response>
</multistatus>`
	entries, err := parseMultiStatus([]byte(resp), "/dav/Movies/")
	if err != nil {
		t.Fatalf("parseMultiStatus: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%d, want 2", len(entries))
	}
	byName := map[string]Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if d := byName["Movies"]; !d.IsDir {
		t.Errorf("Movies 应识别为目录 (IsDir=true)，got IsDir=%v（WebDAV collection 解析回归）", d.IsDir)
	}
	if f := byName["a.mkv"]; f.IsDir {
		t.Errorf("a.mkv 应识别为文件 (IsDir=false)，got IsDir=%v", f.IsDir)
	}
	if f := byName["a.mkv"]; f.Size != 4242424 {
		t.Errorf("a.mkv size=%d, want 4242424", f.Size)
	}
}

// TestLocal_CopySelf_Rejected 钉死「Copy 文件到源所在目录」必须被拒——否则 dst==src，
// copyFileLocal 的 os.Create 会先把源截断成 0、io.Copy 从空 fd 读 → 文件被清空无报错。
func TestLocal_CopySelf_Rejected(t *testing.T) {
	dir := t.TempDir()
	b, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := b.Upload(ctx, "/a.txt", strings.NewReader("content-12345")); err != nil {
		t.Fatal(err)
	}
	if err := b.Copy(ctx, "/a.txt", "/"); err == nil {
		t.Fatal("Copy 到自身应被拒绝（否则源被截断清空）")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "content-12345" {
		t.Fatalf("源文件被破坏: %q", data)
	}
}

// TestLocal_MoveSelf_Rejected 钉死「Move 到源所在目录」必须被拒——否则跨设备降级时
// copyPath 返回后 Move 执行 RemoveAll(src) 删源。
func TestLocal_MoveSelf_Rejected(t *testing.T) {
	dir := t.TempDir()
	b, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := b.Upload(ctx, "/a.txt", strings.NewReader("keep")); err != nil {
		t.Fatal(err)
	}
	if err := b.Move(ctx, "/a.txt", "/"); err == nil {
		t.Fatal("Move 到自身应被拒绝")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "keep" {
		t.Fatalf("源文件被破坏: %q", data)
	}
}

// TestLocal_DeleteRoot_Rejected 钳制 Delete 根目录（防 HTTP/AI 误删清空整个存储）。
func TestLocal_DeleteRoot_Rejected(t *testing.T) {
	dir := t.TempDir()
	b, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := b.Upload(ctx, "/a.txt", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(ctx, "/"); err == nil {
		t.Fatal("Delete 根目录应被拒绝")
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal("拒绝删除根后文件不应丢失")
	}
}

func TestLocal_RenameRootVariants_Rejected(t *testing.T) {
	dir := t.TempDir()
	backend, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"/", "/.", "/..", `\\.`} {
		if err := backend.Rename(context.Background(), root, "renamed"); err == nil {
			t.Errorf("Rename(%q) should reject storage root", root)
		}
	}
}

// TestLocal_CopyIntoDescendant_Rejected 钉死目录复制/移动到自身后代必须被拒——
// 否则 copyDir Walk 会撞刚 MkdirAll 出的 dst 子树，产生嵌套垃圾/重复拷贝。
func TestLocal_CopyIntoDescendant_Rejected(t *testing.T) {
	dir := t.TempDir()
	b, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := b.Mkdir(ctx, "/d/sub"); err != nil {
		t.Fatal(err)
	}
	if err := b.Upload(ctx, "/d/a.txt", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if err := b.Copy(ctx, "/d", "/d/sub"); err == nil {
		t.Fatal("Copy 到自身后代应被拒绝")
	}
	if err := b.Move(ctx, "/d", "/d/sub"); err == nil {
		t.Fatal("Move 到自身后代应被拒绝")
	}
}

// TestSanitizeNewName 钉死 newName 钳制：含分隔符取 Base（容错但防越界），空/. /.. 拒绝。
func TestSanitizeNewName(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"a.txt", "a.txt", false},
		{"../../escape.txt", "escape.txt", false},
		{"a/b/c", "c", false},
		{".", "", true},
		{"..", "", true},
		{"", "", true},
		{"   ", "", true},
	}
	for _, c := range cases {
		got, err := sanitizeNewName(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("sanitizeNewName(%q) 应报错，got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("sanitizeNewName(%q) 意外错误: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("sanitizeNewName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
