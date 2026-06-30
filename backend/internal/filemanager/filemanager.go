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
type StorageBackend interface {
	Type() string
	List(ctx context.Context, path string) ([]Entry, error)
	Upload(ctx context.Context, path string, r io.Reader) error
	Download(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Mkdir(ctx context.Context, path string) error
}
