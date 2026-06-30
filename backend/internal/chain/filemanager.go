package chain

import (
	"context"
	"io"

	"github.com/hxpt/ptpatronus/internal/filemanager"
)

// FileManagerChain 编排文件存储操作。
type FileManagerChain struct {
	Base
	mgr *filemanager.Manager
}

// NewFileManagerChain 构造。
func NewFileManagerChain(b Base, mgr *filemanager.Manager) *FileManagerChain {
	return &FileManagerChain{Base: b, mgr: mgr}
}

// List 列目录。
func (c *FileManagerChain) List(ctx context.Context, id uint, path string) ([]filemanager.Entry, error) {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return b.List(ctx, path)
}

// Upload 上传。
func (c *FileManagerChain) Upload(ctx context.Context, id uint, path string, r io.Reader) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return b.Upload(ctx, path, r)
}

// Download 下载。
func (c *FileManagerChain) Download(ctx context.Context, id uint, path string) (io.ReadCloser, error) {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return b.Download(ctx, path)
}

// Delete 删除。
func (c *FileManagerChain) Delete(ctx context.Context, id uint, path string) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return b.Delete(ctx, path)
}

// Mkdir 建目录。
func (c *FileManagerChain) Mkdir(ctx context.Context, id uint, path string) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return b.Mkdir(ctx, path)
}
