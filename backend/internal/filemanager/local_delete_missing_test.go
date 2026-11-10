// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filemanager

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestLocalDeleteMissingPathIsNotSuccess 删除一个磁盘上已经不存在的路径：
// os.RemoveAll 对 ENOENT 返回 nil，于是界面把「那行早就被别的程序删掉了」报成「已删除」，
// 列表刷新后那条记录还在。空操作必须能红。
func TestLocalDeleteMissingPathIsNotSuccess(t *testing.T) {
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	derr := l.Delete(context.Background(), "/gone.txt")
	if derr == nil {
		t.Fatalf("删不存在的文件不得返回成功")
	}
	if !errors.Is(derr, fs.ErrNotExist) {
		t.Errorf("Delete err = %v, want errors.Is(fs.ErrNotExist)", derr)
	}
}

// TestLocalDeleteStillRemovesRealFile 正控：加了存在性判定不能把正常删除一起掐掉。
func TestLocalDeleteStillRemovesRealFile(t *testing.T) {
	root := t.TempDir()
	l, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := l.Delete(context.Background(), "/keep.txt"); err != nil {
		t.Fatalf("删除真实存在的文件应成功，got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "keep.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("文件应已被删除，stat = %v", err)
	}
}
