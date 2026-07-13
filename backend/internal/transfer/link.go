package transfer

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// LinkOps 抽象本地文件操作（v1 仅本地 FS；Phase 3 扩 StorageBackend 跨后端时替换）。
// 所有方法保证目标父目录存在。
type LinkOps struct{}

// HardLink 硬链 src→dst；跨设备/不支持硬链时降级为 copy（保种优先，但绝不静默丢文件）。
func (LinkOps) HardLink(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := os.Link(src, dst); err != nil {
		// 跨设备(EXDEV)/跨盘(Windows)硬链失败 → 降级 copy（双份占盘但仍可用）。
		slog.Debug("hardlink failed, fallback to copy", "src", src, "dst", dst, "err", err)
		return copyFile(src, dst)
	}
	return nil
}

// SoftLink 软链 src→dst；目标已存在则先删（避免 symlink 失败）。
func (LinkOps) SoftLink(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if _, err := os.Lstat(dst); err == nil {
		_ = os.Remove(dst)
	}
	return os.Symlink(src, dst)
}

// Move 移动 src→dst；跨设备自动 copy+delete。
func (LinkOps) Move(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else {
		// 跨设备 rename 失败 → copy + remove 源（断种，仅 move 模式触发；UI 二次确认）。
		slog.Debug("rename failed, fallback to copy+delete", "src", src, "dst", dst, "err", err)
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// Copy 复制 src→dst。
func (LinkOps) Copy(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return copyFile(src, dst)
}

// Apply 按 mode 执行一次文件整理。
func (LinkOps) Apply(mode Mode, src, dst string) error {
	switch mode {
	case ModeLink:
		return LinkOps{}.HardLink(src, dst)
	case ModeSoftLink:
		return LinkOps{}.SoftLink(src, dst)
	case ModeMove:
		return LinkOps{}.Move(src, dst)
	case ModeCopy:
		return LinkOps{}.Copy(src, dst)
	default:
		return LinkOps{}.HardLink(src, dst)
	}
}

// copyFile 逐字节复制（兼容跨设备/跨盘）。
func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()
	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer df.Close()
	if _, err := io.Copy(df, sf); err != nil {
		return err
	}
	if si, err := os.Stat(src); err == nil {
		_ = os.Chmod(dst, si.Mode())
	}
	return nil
}

// pathExists 本地路径是否存在。
func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
