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
	// 删源失败时：dst 已存在且与 src 等大（copy 已完整落地）→ 视为幂等成功。
	// 否则整理会无限重试且卡在 target-exists，不如让上层跳过重试。
	if err := os.Remove(src); err != nil {
		if sameSize(src, dst) {
			slog.Warn("transfer: move remove source failed but dst complete, treat as idempotent success", "src", src, "err", err)
			return nil
		}
		return fmt.Errorf("remove source: %w", err)
	}
	return nil
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

// replaceViaTemp 实现「先链新到临时名→校验成功→删旧→重命名为最终名」的洗版原子替换。
// 任何步骤失败都保证旧库文件不丢、不留残临时文件：
//  1. Apply 到 dst 同目录的临时名 dst.new（链失败直接返回，旧文件未动）；
//  2. 删除旧库文件（删失败回滚：清理临时文件后返回）；
//  3. rename 临时名为 dst（rename 同目录原子，极少失败；若失败临时文件仍在，已记 Replaced）。
//
// 成功删除的旧路径追加到 *replaced（洗版通知/历史用）。
func replaceViaTemp(mode Mode, src, dst, old string, replaced *[]string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmp := dst + ".new"
	// 若上轮失败遗留同名 tmp，先清。
	if _, err := os.Lstat(tmp); err == nil {
		_ = os.Remove(tmp)
	}
	if err := (LinkOps{}).Apply(mode, src, tmp); err != nil {
		return fmt.Errorf("link temp: %w", err)
	}
	// 删旧失败 → 清理临时文件后返回（旧库文件未丢）。
	if _, err := removeMediaFile(old); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("remove old: %w", err)
	}
	if replaced != nil {
		*replaced = append(*replaced, old)
	}
	// rename 同目录原子。失败时临时文件仍在（但不影响做种源/旧库已让位）。
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}

// copyFile 逐字节复制（兼容跨设备/跨盘）。
// 安全保证：
//   - dst 已存在且与 src 同 inode（如已是硬链到同一文件）→ 跳过，避免 O_TRUNC 自毁源；
//   - io.Copy 失败时 os.Remove(dst) 清理残文件，避免半写文件被当成完成。
func copyFile(src, dst string) error {
	// dst 已存在且与 src 同 inode → 不再截断，视为已完成（幂等成功）。
	if sameInode(src, dst) {
		return nil
	}
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()
	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(df, sf); err != nil {
		_ = df.Close()
		// 清理半写残文件，避免下次被当成完整目标。
		_ = os.Remove(dst)
		return err
	}
	if err := df.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	if si, err := os.Stat(src); err == nil {
		_ = os.Chmod(dst, si.Mode())
	}
	return nil
}

// sameInode 报告 src 与 dst 是否同一 inode（同盘硬链到自身/同名）。任一 stat 失败返 false。
func sameInode(src, dst string) bool {
	ss, err := os.Stat(src)
	if err != nil {
		return false
	}
	ds, err := os.Stat(dst)
	if err != nil {
		return false
	}
	return os.SameFile(ss, ds)
}

// sameSize 报告 src 与 dst 当前大小是否相等（copy 落地后 dst 应等大）。任一 stat 失败返 false。
func sameSize(src, dst string) bool {
	ss, err := os.Stat(src)
	if err != nil {
		return false
	}
	ds, err := os.Stat(dst)
	if err != nil {
		return false
	}
	return ss.Size() == ds.Size()
}

// pathExists 本地路径是否存在。
func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
