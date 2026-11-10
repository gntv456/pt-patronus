// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/filemanager"
	"gorm.io/gorm"
)

// newLocalFileChain 接真实 sqlite + 真实本机存储目录，返回 chain/根目录/存储 id。
// 走 Manager.buildBackend 的 local 分支，保证测的是文件管理器 UI 点「改名/移动/复制」的同一入口。
func newLocalFileChain(t *testing.T) (*FileManagerChain, string, uint) {
	t.Helper()
	root := t.TempDir()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/fm.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.FileStorage{}); err != nil {
		t.Fatal(err)
	}
	fsOp := oper.NewFileStorageOper(db, security.NewCrypto("seed"))
	rec, err := fsOp.Create(oper.FileStorageInput{Name: "本机", Type: "local", BasePath: root})
	if err != nil {
		t.Fatal(err)
	}
	ch := NewFileManagerChain(Base{}, filemanager.NewManager(fsOp))
	return ch, root, rec.ID
}

func writeAt(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readAt(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func existsAt(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// TestFileManagerRenameCollidingTargetRefuses 改名撞上同名条目必须拒绝，且两边文件都还在原样。
// 回归：修复前直接 os.Rename，改名撞名返回成功、目标文件内容被换掉——
// 摘掉检查跑本测试实测 err==nil（Windows 的 MoveFileEx 带 REPLACE_EXISTING，POSIX rename(2) 同样替换）。
func TestFileManagerRenameCollidingTargetRefuses(t *testing.T) {
	ch, root, id := newLocalFileChain(t)
	writeAt(t, root, "keep.mkv", "IMPORTANT")
	writeAt(t, root, "junk.mkv", "junk")

	err := ch.Rename(context.Background(), id, "/junk.mkv", "keep.mkv")
	if err == nil {
		t.Fatal("改名撞上同名条目应拒绝")
	}
	if !strings.Contains(err.Error(), "已存在同名条目") {
		t.Fatalf("错误应说明同名冲突，got %v", err)
	}
	if got := readAt(t, root, "keep.mkv"); got != "IMPORTANT" {
		t.Fatalf("目标文件被覆盖（数据丢失）：content=%q", got)
	}
	if !existsAt(root, "junk.mkv") {
		t.Fatal("被拒绝的改名不得动到源文件")
	}

	// 正控：目标位空着时改名照常可用。
	if err := ch.Rename(context.Background(), id, "/junk.mkv", "renamed.mkv"); err != nil {
		t.Fatalf("无冲突改名应成功: %v", err)
	}
	if existsAt(root, "junk.mkv") || readAt(t, root, "renamed.mkv") != "junk" {
		t.Fatal("改名后源消失、目标带内容")
	}
	// 正控：改回自己的名字（同名但就是源本身）不该被误判成冲突。
	if err := ch.Rename(context.Background(), id, "/renamed.mkv", "renamed.mkv"); err != nil {
		t.Fatalf("原地改名（同名即源自身）应放行: %v", err)
	}
	// 正控：Linux 上改大小写是正当操作，不能被模糊匹配挡掉——但目标与源是同一文件时放行。
	if err := ch.Rename(context.Background(), id, "/renamed.mkv", "Renamed.mkv"); err != nil {
		t.Fatalf("改大小写应放行: %v", err)
	}
	if !existsAt(root, "Renamed.mkv") {
		t.Fatal("大小写改名后文件应在")
	}
}

// TestFileManagerMoveCollidingTargetRefuses 跨目录移动撞上同名条目同样拒绝（摘掉检查后实测返回成功且目标内容被换掉）。
func TestFileManagerMoveCollidingTargetRefuses(t *testing.T) {
	ch, root, id := newLocalFileChain(t)
	writeAt(t, root, "a/x.mkv", "junk-a")
	writeAt(t, root, "b/x.mkv", "IMPORTANT-b")

	err := ch.Move(context.Background(), id, "/a/x.mkv", "/b")
	if err == nil || !strings.Contains(err.Error(), "已存在同名条目") {
		t.Fatalf("移动到同名目标应拒绝，got %v", err)
	}
	if got := readAt(t, root, "b/x.mkv"); got != "IMPORTANT-b" {
		t.Fatalf("目标目录里的同名文件被覆盖：content=%q", got)
	}
	if !existsAt(root, "a/x.mkv") {
		t.Fatal("被拒绝的移动不得搬走源")
	}

	// 正控：目标目录没占用时移动照常。
	writeAt(t, root, "c/readme.txt", "hi")
	if err := ch.Move(context.Background(), id, "/c/readme.txt", "/b"); err != nil {
		t.Fatalf("无冲突移动应成功: %v", err)
	}
	if existsAt(root, "c/readme.txt") || readAt(t, root, "b/readme.txt") != "hi" {
		t.Fatal("移动后源应消失、目标应在")
	}
	// 正控：移回自己所在的目录（同名条目就是源本身）不该被误判成同名冲突。
	// 后端对「原地移动」另有自己的语义（no-op 或拒绝），这里只要求不是检查在挡路。
	if err := ch.Move(context.Background(), id, "/b/readme.txt", "/b"); err != nil &&
		strings.Contains(err.Error(), "已存在同名条目") {
		t.Fatalf("移回本目录被误判成同名冲突: %v", err)
	}
	// 正控：目标目录写法带多余斜杠时，「源就在该目录」仍识别为原地移动，不误报同名冲突。
	if err := ch.Move(context.Background(), id, "/b/readme.txt", "/b//"); err != nil &&
		strings.Contains(err.Error(), "已存在同名条目") {
		t.Fatalf("目标目录写法差异造成同名误判: %v", err)
	}

	// 反查：写法差异也不能让冲突漏检——/a/x.mkv 移到 "/b//" 同样要撞名。
	if err := ch.Move(context.Background(), id, "/a/x.mkv", "/b//"); err == nil ||
		!strings.Contains(err.Error(), "已存在同名条目") {
		t.Fatalf("带多余斜杠的目标目录仍应检出同名冲突，got %v", err)
	}
}

// TestFileManagerCopyCollidingTargetRefuses 复制撞上同名条目拒绝（copyFileLocal 用 os.Create 截断目标）。
func TestFileManagerCopyCollidingTargetRefuses(t *testing.T) {
	ch, root, id := newLocalFileChain(t)
	writeAt(t, root, "a/src.mkv", "junk")
	writeAt(t, root, "b/src.mkv", "IMPORTANT")

	err := ch.Copy(context.Background(), id, "/a/src.mkv", "/b")
	if err == nil || !strings.Contains(err.Error(), "已存在同名条目") {
		t.Fatalf("复制到同名目标应拒绝，got %v", err)
	}
	if got := readAt(t, root, "b/src.mkv"); got != "IMPORTANT" {
		t.Fatalf("同名目标被截断覆盖：content=%q", got)
	}
	if !existsAt(root, "a/src.mkv") {
		t.Fatal("被拒绝的复制不该动源")
	}

	// 正控：复制到不冲突的目录照常生成副本，源保留。
	_ = os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	if err := ch.Copy(context.Background(), id, "/a/src.mkv", "/sub"); err != nil {
		t.Fatalf("无冲突复制应成功: %v", err)
	}
	if !existsAt(root, "a/src.mkv") || readAt(t, root, "sub/src.mkv") != "junk" {
		t.Fatal("复制后源与副本都应在")
	}
}

// TestFileManagerArchiveCollidingTargetRefuses 压缩产物同名会被就地截断（可能是另一份归档）。
func TestFileManagerArchiveCollidingTargetRefuses(t *testing.T) {
	ch, root, id := newLocalFileChain(t)
	writeAt(t, root, "movie.mkv", "MKV")
	writeAt(t, root, "movie.mkv.zip", "OLD-ARCHIVE")

	err := ch.Archive(context.Background(), id, "/movie.mkv", "/movie.mkv.zip")
	if err == nil || !strings.Contains(err.Error(), "已存在同名条目") {
		t.Fatalf("压缩撞上同名产物应拒绝，got %v", err)
	}
	if got := readAt(t, root, "movie.mkv.zip"); got != "OLD-ARCHIVE" {
		t.Fatalf("既有归档被覆盖：content=%q", got)
	}

	// 正控：换个不冲突的产物名照常压缩成功。
	if err := ch.Archive(context.Background(), id, "/movie.mkv", "/out.zip"); err != nil {
		t.Fatalf("无冲突压缩应成功: %v", err)
	}
	if !existsAt(root, "out.zip") {
		t.Fatal("压缩产物应生成")
	}
}

// failingListBackend 列目录总是失败的后端（远端存储掉线/权限异常）。
type failingListBackend struct{ filemanager.StorageBackend }

func (failingListBackend) List(context.Context, string) ([]filemanager.Entry, error) {
	return nil, context.DeadlineExceeded
}

// TestEnsureTargetFreeFailsClosed 目标目录列不出来时必须中止：
// 无法确认没有同名冲突还执行覆盖类操作，等于把这道检查当装饰。
func TestEnsureTargetFreeFailsClosed(t *testing.T) {
	err := ensureTargetFree(context.Background(), failingListBackend{}, "/a/x.mkv", "/b", "x.mkv")
	if err == nil {
		t.Fatal("列目录失败时应中止")
	}
	if !strings.Contains(err.Error(), "已中止") {
		t.Fatalf("错误应说明中止原因，got %v", err)
	}
	// 目标位就是源本身时无需列表，仍放行（原地改名不被健康问题挡住）。
	if err := ensureTargetFree(context.Background(), failingListBackend{}, "/a/x.mkv", "/a", "x.mkv"); err != nil {
		t.Fatalf("原地改名应放行，got %v", err)
	}
}
