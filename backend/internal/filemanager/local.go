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
	rootDir  string // 已 Abs 规范化的物理根目录
	rootReal string // EvalSymlinks(rootDir)；Windows 上 TEMP 常为 8.3 短名（RUNNER~1），
	// 与子路径 EvalSymlinks 展开出的长名字符串不同，若只用 rootDir 比较会整树误判逃逸
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
	rootReal := abs
	if real, e := filepath.EvalSymlinks(abs); e == nil {
		rootReal = real
	}
	return &Local{rootDir: abs, rootReal: rootReal}, nil
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
	// 符号链接越界防御：EvalSymlinks 解析 full 路径中的所有软链到真实物理路径，
	// 再校验真实路径仍在根之下。root 内指向外部的 symlink 会被拦截。
	// 根同时比对 rootDir 与 rootReal：Windows 上两者可能是同一目录的不同字符串
	// （8.3 短名 vs 长名），只比一个会把合法子路径误判成逃逸（CI TEMP 即短名）。
	if real, e := filepath.EvalSymlinks(full); e == nil {
		if !underRoot(l.rootDir, real) && !underRoot(l.rootReal, real) {
			return "", fmt.Errorf("symlink escapes root: %s -> %s", p, real)
		}
	}
	return full, nil
}

// underRoot 报告 real 是否位于 root 之下（含 root 本身）。
func underRoot(root, real string) bool {
	rel, err := filepath.Rel(root, real)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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

// Delete 删除文件或目录（递归）。拒绝删除存储根目录（path="/" 或空解析为 rootDir）——
// 该操作会清空整个存储，前端误传或 AI 提示注入均可触发，故在 backend 层硬拦。
func (l *Local) Delete(ctx context.Context, p string) error {
	full, err := l.resolve(p)
	if err != nil {
		return err
	}
	if full == l.rootDir {
		return fmt.Errorf("不允许删除存储根目录")
	}
	// os.RemoveAll 对「路径本来就不存在」返回 nil：删一条早被别的程序/另一个标签页
	// 删掉的记录会被界面报成「已删除」，而那行还留在列表里。先判存在性，让空操作能红。
	if _, err := os.Lstat(full); err != nil {
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

// Rename 同目录改名：path 换成 newName（sanitizeNewName 校验防注入路径/非法名）。
func (l *Local) Rename(ctx context.Context, p, newName string) error {
	full, err := l.resolve(p)
	if err != nil {
		return err
	}
	if filepath.Clean(full) == filepath.Clean(l.rootDir) {
		return fmt.Errorf("不允许重命名存储根目录")
	}
	name, err := sanitizeNewName(newName)
	if err != nil {
		return err
	}
	dst := filepath.Join(filepath.Dir(full), name)
	return os.Rename(full, dst)
}

// Move 移动到 dstDir（保留文件名）；os.Rename 跨设备失败时降级 copy+delete。
//
// 必须在 copy+delete 前拦截 dst==src：否则 copyPath 对「复制到自身」返回成功后，
// Move 会 RemoveAll(src) 删源；以及 dst 落在 src 子树（目录移入自身后代）会让
// copyDir Walk 撞刚建的 dst 子树产生嵌套垃圾——两者都致数据损坏。
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
	if isSameOrUnder(srcFull, dst) {
		return fmt.Errorf("目标路径与源相同或位于源内部，拒绝移动")
	}
	if err := os.Rename(srcFull, dst); err == nil {
		return nil
	}
	if err := copyPath(srcFull, dst); err != nil {
		return err
	}
	return os.RemoveAll(srcFull)
}

// Copy 复制到 dstDir（保留文件名）；文件 io.Copy，目录递归。
// 拦截 dst==src（防 copyFileLocal(src,src) 先 os.Create 截断源致内容清空）与
// dst 落在 src 子树（目录复制入自身后代）。
func (l *Local) Copy(ctx context.Context, src, dstDir string) error {
	srcFull, err := l.resolve(src)
	if err != nil {
		return err
	}
	dstFull, err := l.resolve(dstDir)
	if err != nil {
		return err
	}
	dst := filepath.Join(dstFull, filepath.Base(srcFull))
	if isSameOrUnder(srcFull, dst) {
		return fmt.Errorf("目标路径与源相同或位于源内部，拒绝复制")
	}
	return copyPath(srcFull, dst)
}

// isSameOrUnder 判断 dst 是否等于 src 或落在 src 子树内。
// 用于拦截「复制/移动到自身」(防 copyFileLocal(src,src) 截断清空) 与
// 「目录移入自身后代」(防 copyDir Walk 撞刚建的 dst 子树)。
func isSameOrUnder(src, dst string) bool {
	cs := filepath.Clean(src)
	cd := filepath.Clean(dst)
	if cs == cd {
		return true
	}
	return strings.HasPrefix(cd, cs+string(filepath.Separator))
}

// sanitizeNewName 校验并钳制新文件名：去首尾空白，再用 filepath.Base 钳为纯文件名。
// 防 "a/b" 把文件移进子目录、防 "../x" 上溯越界（与 local 原先的 filepath.Base 行为一致：
// 容错但安全）。仅拒绝空 / "." / ".."（Base 后仍非法）。供各后端 Rename 统一采用。
func sanitizeNewName(newName string) (string, error) {
	name := strings.TrimSpace(newName)
	if name == "" {
		return "", fmt.Errorf("非法新文件名: %q", newName)
	}
	name = filepath.Base(filepath.FromSlash(name))
	if name == "." || name == ".." {
		return "", fmt.Errorf("非法新文件名: %q", newName)
	}
	return name, nil
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

// copyFileLocal 逐字节复制单文件（建父目录，保留源权限位）。
//
// 关键：out.Close() 的错误不可被 defer 吞掉——NFS/SMB/网络挂载下写错误常延迟到
// close（刷缓冲）才报；若吞掉，io.Copy 返回 nil 会让调用方（Move 的 copy+delete 降级）
// 误以为成功并 RemoveAll 源，造成「目标残缺/空、源已删」的不可逆数据丢失。
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
	_, copyErr := io.Copy(out, in) // 先取拷贝错误
	if cerr := out.Close(); copyErr == nil {
		copyErr = cerr // 拷贝无错时改用 close 错误（含延迟的写失败）
	}
	if copyErr != nil {
		return copyErr
	}
	// 保留源权限位（os.Create 受 umask 落地 ~0666，会丢失可执行/只读位）。
	if info, err := os.Stat(src); err == nil {
		_ = os.Chmod(dst, info.Mode())
	}
	return nil
}
