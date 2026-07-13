package filemanager

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Local 本机文件系统后端：直接读写后端进程所在机器的磁盘目录。
// 与 webdav/alist/smb/agent 不同，它不经网络也不经 agent 转发，因此根路径（BasePath）
// 是服务端本机的一个物理绝对目录（如 Windows D:\Media 或 Linux /mnt/nas）。
//
// 安全：rootDir 把所有操作钳制在其下，resolve 做两重防御——path.Clean 抹掉 ../，
// filepath.Rel 再校验结果未逃出 rootDir。注意 os 调用会跟随符号链接，故 rootDir 下
// 若存在指向外部的软链仍可被访问；本机存储仅应由管理员配置可信目录（API 路由已限管理员）。
type Local struct {
	rootDir string // 已 Abs 规范化的物理根目录
}

// NewLocal 校验根目录存在且为目录，构造本机后端。
func NewLocal(basePath string) (*Local, error) {
	if strings.TrimSpace(basePath) == "" {
		return nil, fmt.Errorf("本机存储必须填写根目录")
	}
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("解析本机根目录失败: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("本机根目录不可访问: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("本机根路径不是目录: %s", abs)
	}
	return &Local{rootDir: abs}, nil
}

func (l *Local) Type() string { return "local" }

// resolve 把虚拟路径 p（以 / 开头，前端约定）映射到本机物理路径并钳制在 rootDir 下。
// 返回的 full 直接喂给 os.* 使用。虚拟/物理分离是 Windows 必需：物理根 D:\Media 不能
// 直接当 / 开头的虚拟路径回传前端，故对前端始终以 / 为虚拟根。
func (l *Local) resolve(p string) (string, error) {
	cleaned := path.Clean("/" + strings.TrimLeft(strings.ReplaceAll(p, "\\", "/"), "/"))
	full := filepath.Join(l.rootDir, filepath.FromSlash(cleaned))
	// filepath.Join 会清理 ../，但可能把根顶掉后逃到 rootDir 之外（如 rootDir 在子目录时），
	// 用 Rel 纯字符串层再校验一次：相对路径不得以 ".." 开头。
	rel, err := filepath.Rel(l.rootDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root: %s", p)
	}
	return full, nil
}

// List 列目录。返回的 Entry.Path 是虚拟路径（/开头），与 agent/webdav 后端格式一致，
// 前端 enter/up 依赖该约定。
func (l *Local) List(ctx context.Context, p string) ([]Entry, error) {
	full, err := l.resolve(p)
	if err != nil {
		return nil, err
	}
	dirEntries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	parent := normalizePath(p)
	out := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		modTime := time.Time{}
		size := int64(0)
		if info, err := de.Info(); err == nil {
			modTime = info.ModTime()
			size = info.Size()
		}
		out = append(out, Entry{
			Name:    de.Name(),
			Path:    parent + "/" + de.Name(),
			IsDir:   de.IsDir(),
			Size:    size,
			ModTime: modTime,
		})
	}
	return out, nil
}

// Upload 写入文件（父目录不存在则创建）。
func (l *Local) Upload(ctx context.Context, p string, r io.Reader) error {
	full, err := l.resolve(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// Download 打开文件供调用方读取（*os.File 即 io.ReadCloser，由上层关闭）。
func (l *Local) Download(ctx context.Context, p string) (io.ReadCloser, error) {
	full, err := l.resolve(p)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

// Delete 删除文件或目录（递归）。
func (l *Local) Delete(ctx context.Context, p string) error {
	full, err := l.resolve(p)
	if err != nil {
		return err
	}
	return os.RemoveAll(full)
}

// Mkdir 建目录（含父级）。
func (l *Local) Mkdir(ctx context.Context, p string) error {
	full, err := l.resolve(p)
	if err != nil {
		return err
	}
	return os.MkdirAll(full, 0o755)
}

// Rename 同目录改名：path 的 basename 换成 newName（filepath.Base 钳制防注入路径）。
func (l *Local) Rename(ctx context.Context, p, newName string) error {
	full, err := l.resolve(p)
	if err != nil {
		return err
	}
	dst := filepath.Join(filepath.Dir(full), filepath.Base(filepath.FromSlash(newName)))
	return os.Rename(full, dst)
}

// Move 移动到 dstDir（保留文件名）；os.Rename 跨设备失败时降级 copy+delete。
func (l *Local) Move(ctx context.Context, src, dstDir string) error {
	srcFull, err := l.resolve(src)
	if err != nil {
		return err
	}
	dstFull, err := l.resolve(dstDir)
	if err != nil {
		return err
	}
	dst := filepath.Join(dstFull, filepath.Base(srcFull))
	if err := os.Rename(srcFull, dst); err == nil {
		return nil
	}
	if err := copyPath(srcFull, dst); err != nil {
		return err
	}
	return os.RemoveAll(srcFull)
}

// Copy 复制到 dstDir（保留文件名）；文件 io.Copy，目录递归。
func (l *Local) Copy(ctx context.Context, src, dstDir string) error {
	srcFull, err := l.resolve(src)
	if err != nil {
		return err
	}
	dstFull, err := l.resolve(dstDir)
	if err != nil {
		return err
	}
	return copyPath(srcFull, filepath.Join(dstFull, filepath.Base(srcFull)))
}

// copyPath 递归复制文件或目录（文件 copyFileLocal，目录 filepath.Walk 遍历）。
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst, info.Mode())
	}
	return copyFileLocal(src, dst)
}

// copyDir 递归复制目录树，保留目录权限位。
func copyDir(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(dst, mode); err != nil {
		return err
	}
	return filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(target, fi.Mode())
		}
		return copyFileLocal(p, target)
	})
}

// copyFileLocal 逐字节复制单文件（建父目录，保留读权限）。
func copyFileLocal(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
