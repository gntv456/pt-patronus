// filemanager_extra.go — chain 层扩展能力转发（在线编辑/压缩/解压/chmod/目录大小）。
// 各方法先取 backend，再做可选能力类型断言；未实现返回 ErrUnsupported。

package chain

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/filemanager"
)

// ReadFile 读取文件内容（在线编辑/预览）。
func (c *FileManagerChain) ReadFile(ctx context.Context, id uint, path string) ([]byte, error) {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	r, ok := b.(filemanager.FileReader)
	if !ok {
		return nil, filemanager.ErrUnsupported
	}
	return r.ReadFile(ctx, path)
}

// WriteFile 写入文件内容（在线编辑保存）。
func (c *FileManagerChain) WriteFile(ctx context.Context, id uint, path string, data []byte) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	w, ok := b.(filemanager.FileWriter)
	if !ok {
		return filemanager.ErrUnsupported
	}
	return w.WriteFile(ctx, path, data)
}

// Archive 压缩。
func (c *FileManagerChain) Archive(ctx context.Context, id uint, src, dst string) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	a, ok := b.(filemanager.Archiver)
	if !ok {
		return filemanager.ErrUnsupported
	}
	// 压缩产物是截断写入：同名 dst 会被就地毁掉（可能是另一份归档），先确认目标位空着。
	if err := ensureTargetFree(ctx, b, src, vdir(dst), vbase(dst)); err != nil {
		return err
	}
	return a.Archive(ctx, src, dst)
}

// Extract 解压。
func (c *FileManagerChain) Extract(ctx context.Context, id uint, src, dstDir string) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	e, ok := b.(filemanager.Extractor)
	if !ok {
		return filemanager.ErrUnsupported
	}
	return e.Extract(ctx, src, dstDir)
}

// Chmod 修改权限。
func (c *FileManagerChain) Chmod(ctx context.Context, id uint, path string, mode uint32, recursive bool) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	cm, ok := b.(filemanager.Chmoder)
	if !ok {
		return filemanager.ErrUnsupported
	}
	return cm.Chmod(ctx, path, mode, recursive)
}

// DirSize 计算目录大小。
func (c *FileManagerChain) DirSize(ctx context.Context, id uint, path string) (int64, error) {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	ds, ok := b.(filemanager.DirSizer)
	if !ok {
		return 0, filemanager.ErrUnsupported
	}
	return ds.DirSize(ctx, path)
}
