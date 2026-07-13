// Package filemanager 定义网盘/存储后端抽象（WebDAV / Alist）与数据模型。
package filemanager

import (
	"context"
	"io"

	"github.com/hxpt/ptpatronus/internal/agentrpc"
)

// Entry 是一个文件/目录条目（= agentrpc.Entry，本地后端 webdav/alist/smb 与 agent 后端共用同一结构）。
type Entry = agentrpc.Entry

// StorageBackend 是存储后端统一接口。
// 路径约定：path/srcPath/dstDir 均为前端虚拟路径（/ 开头，各后端内部钳制在 BasePath 下）。
type StorageBackend interface {
	Type() string
	List(ctx context.Context, path string) ([]Entry, error)
	Upload(ctx context.Context, path string, r io.Reader) error
	Download(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Mkdir(ctx context.Context, path string) error
	// Rename 同目录改名：把 path 的文件名换成 newName（纯文件名，不含 /）。
	Rename(ctx context.Context, path, newName string) error
	// Move 把 srcPath 移到 dstDir 下（保留原文件名；跨目录即移动）。
	Move(ctx context.Context, srcPath, dstDir string) error
	// Copy 把 srcPath 复制到 dstDir 下（保留原文件名）。
	Copy(ctx context.Context, srcPath, dstDir string) error
}
