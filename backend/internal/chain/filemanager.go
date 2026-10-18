package chain

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

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

// vpath 规范化虚拟路径（后端约定以 "/" 分隔）：反斜杠先归一，再 Clean 掉 ./尾部斜杠。
func vpath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" {
		return "/"
	}
	return path.Clean(p)
}

// vdir/vbase 虚拟路径的父目录与末段名（vpath 先归一，避免尾斜杠/./造成比较错位）。
func vdir(p string) string  { return path.Dir(vpath(p)) }
func vbase(p string) string { return path.Base(vpath(p)) }

// ensureTargetFree 改名/移动/复制/压缩前确认目标位未被占用。
// 这些操作在各后端都是「静默覆盖」语义：本机后端直接 os.Rename——POSIX 的 rename(2) 与
// Windows 的 MoveFileEx(MOVEFILE_REPLACE_EXISTING) 都会替换既有文件（实测改名/移动撞名后
// 返回成功、目标内容消失）；复制走截断写入；WebDAV 请求还带 Overwrite: T。
// 而用户在文件管理器点改名/移动时从不是想销毁另一个文件，同名覆盖还不可逆。
// 列不出目标目录即中止——确认不了无冲突还执行覆盖操作，等于把这道检查当装饰。
// 刻意不做大小写模糊匹配：Linux 上 movie.mkv→Movie.mkv 是正当改名，误拒反而挡路。
func ensureTargetFree(ctx context.Context, b filemanager.StorageBackend, srcPath, dstDir, dstName string) error {
	dstDir = vpath(dstDir)
	if dstDir == vdir(srcPath) && dstName == vbase(srcPath) {
		return nil // 目标位就是源本身（原地改名 / 移回本目录）
	}
	entries, err := b.List(ctx, dstDir)
	if err != nil {
		return fmt.Errorf("无法确认目标目录 %s 是否已有同名条目（%v），已中止以免静默覆盖", dstDir, err)
	}
	for _, e := range entries {
		if e.Name == dstName {
			return fmt.Errorf("目标目录 %s 下已存在同名条目「%s」，已拒绝覆盖", dstDir, dstName)
		}
	}
	return nil
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

// Rename 同目录改名。
func (c *FileManagerChain) Rename(ctx context.Context, id uint, path, newName string) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureTargetFree(ctx, b, path, vdir(path), newName); err != nil {
		return err
	}
	return b.Rename(ctx, path, newName)
}

// Move 移到 dstDir（保留文件名）。
func (c *FileManagerChain) Move(ctx context.Context, id uint, src, dstDir string) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureTargetFree(ctx, b, src, dstDir, vbase(src)); err != nil {
		return err
	}
	return b.Move(ctx, src, dstDir)
}

// Copy 复制到 dstDir（保留文件名）。
func (c *FileManagerChain) Copy(ctx context.Context, id uint, src, dstDir string) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureTargetFree(ctx, b, src, dstDir, vbase(src)); err != nil {
		return err
	}
	return b.Copy(ctx, src, dstDir)
}

// Test 探测存储连通性：列配置根成功即视为连通。
// 复用 List，零接口改动；各后端构造时已注入 BasePath，path="/" 即探测配置根。
func (c *FileManagerChain) Test(ctx context.Context, id uint) error {
	b, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	_, err = b.List(ctx, "/")
	return err
}
