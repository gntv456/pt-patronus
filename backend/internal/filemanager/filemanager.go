// Package filemanager 定义网盘/存储后端抽象（WebDAV / Alist）与数据模型。
package filemanager

import (
	"context"
	"fmt"
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

// ErrUnsupported 表示该后端不支持此操作（如 webdav/alist 不支持 chmod）。
// chain 层对可选能力做类型断言，未实现则返回此错误；前端按后端类型隐藏按钮。
var ErrUnsupported = fmt.Errorf("该存储后端不支持此操作")

// FileReader 读取文件全部内容（用于在线编辑/文本预览）。
type FileReader interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
}

// FileWriter 写入文件全部内容（用于在线编辑保存）。
type FileWriter interface {
	WriteFile(ctx context.Context, path string, data []byte) error
}

// Archiver 压缩：把 src（文件或目录）打包为 dst（.zip/.tar.gz）。
type Archiver interface {
	Archive(ctx context.Context, src, dst string) error
}

// Extractor 解压：把 src（.zip/.tar.gz）解到 dstDir。
type Extractor interface {
	Extract(ctx context.Context, src, dstDir string) error
}

// Chmoder 修改权限/属主（仅 local/agent 支持）。
type Chmoder interface {
	Chmod(ctx context.Context, path string, mode uint32, recursive bool) error
}

// DirSizer 计算目录递归大小。
type DirSizer interface {
	DirSize(ctx context.Context, path string) (int64, error)
}
