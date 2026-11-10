// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filemanager

import (
	"runtime"

	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocal_ReadWriteFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644)
	b, _ := NewLocal(root)
	ctx := context.Background()

	data, err := b.ReadFile(ctx, "/a.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("ReadFile = %q, want hello", data)
	}
	if err := b.WriteFile(ctx, "/a.txt", []byte("world")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, _ = b.ReadFile(ctx, "/a.txt")
	if string(data) != "world" {
		t.Errorf("after WriteFile = %q, want world", data)
	}
	// 读目录应报错
	if _, err := b.ReadFile(ctx, "/"); err == nil {
		t.Error("ReadFile on dir should error")
	}
}

func TestLocal_DirSize(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "f1"), make([]byte, 100), 0o644)
	os.WriteFile(filepath.Join(root, "sub", "f2"), make([]byte, 200), 0o644)
	b, _ := NewLocal(root)
	size, err := b.DirSize(context.Background(), "/")
	if err != nil {
		t.Fatalf("DirSize: %v", err)
	}
	if size != 300 {
		t.Errorf("DirSize = %d, want 300", size)
	}
}

func TestLocal_Chmod(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix ???? Windows ??????")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a"), []byte("x"), 0o644)
	b, _ := NewLocal(root)
	if err := b.Chmod(context.Background(), "/a", 0o600, false); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	info, _ := os.Stat(filepath.Join(root, "a"))
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
	// 根目录应拒绝
	if err := b.Chmod(context.Background(), "/", 0o777, false); err == nil {
		t.Error("Chmod root should reject")
	}
}

func TestLocal_ArchiveExtract(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "src", "a.txt"), []byte("aaa"), 0o644)
	b, _ := NewLocal(root)
	ctx := context.Background()

	// zip
	if err := b.Archive(ctx, "/src", "/out.zip"); err != nil {
		t.Fatalf("Archive zip: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "out.zip")); err != nil {
		t.Fatalf("zip not created: %v", err)
	}
	// 解压到新目录
	if err := b.Extract(ctx, "/out.zip", "/unzipped"); err != nil {
		t.Fatalf("Extract zip: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "unzipped", "src", "a.txt"))
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(data) != "aaa" {
		t.Errorf("extracted content = %q, want aaa", data)
	}

	// tar.gz
	if err := b.Archive(ctx, "/src", "/out.tar.gz"); err != nil {
		t.Fatalf("Archive targz: %v", err)
	}
	if err := b.Extract(ctx, "/out.tar.gz", "/untarred"); err != nil {
		t.Fatalf("Extract targz: %v", err)
	}
	data, _ = os.ReadFile(filepath.Join(root, "untarred", "src", "a.txt"))
	if string(data) != "aaa" {
		t.Errorf("untarred content = %q, want aaa", data)
	}

	// 源在目标内部应拒绝（把整个 root 压到 root 下）
	if err := b.Archive(ctx, "/", "/root.zip"); err == nil {
		t.Error("Archive root into itself should reject")
	}
}

func TestLocal_ExtractZipSlip(t *testing.T) {
	root := t.TempDir()
	// 构造含 ../escape.txt 的恶意 zip
	zipPath := filepath.Join(root, "evil.zip")
	zf, _ := os.Create(zipPath)
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("../escape.txt")
	w.Write([]byte("pwned"))
	zw.Close()
	zf.Close()

	b, _ := NewLocal(root)
	err := b.Extract(context.Background(), "/evil.zip", "/out")
	if err == nil {
		t.Fatal("zip slip should be detected")
	}
}
