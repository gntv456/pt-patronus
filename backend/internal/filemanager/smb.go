package filemanager

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	smb2 "github.com/cloudsoda/go-smb2"
)

// SMB 后端（SMB2/3，纯 Go github.com/cloudsoda/go-smb2，无 CGO）。
// 字段映射：URL=smb://host[:port]；BasePath=/<share>/<sub>（首段为 share 名，余为 share 内路径）。
// 懒连接：每操作 dial→mount→操作→close，避免缓存连不上的后端。
// 限制：仅支持 SMB2/3，不支持 SMB1 老 NAS。
type SMB struct {
	host     string
	port     int
	username string
	password string
	basePath string
}

// NewSMB 构造。URL 解析出 host/port（默认 445）；basePath 把所有操作约束在该子目录下（防 ../ 越界）。
func NewSMB(rawURL, username, password, basePath string) *SMB {
	s := &SMB{username: username, password: password, basePath: basePath, port: 445}
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil && u.Hostname() != "" {
		s.host = u.Hostname()
		if p := u.Port(); p != "" {
			if n, err := strconv.Atoi(p); err == nil {
				s.port = n
			}
		}
	}
	return s
}

func (s *SMB) Type() string { return "smb" }

// session 建立 TCP+SMB 会话并挂载 share；返回 share 与 cleanup（umount+logoff+cancel）。
func (s *SMB) session(ctx context.Context) (*smb2.Share, func(), error) {
	shareName, _ := splitShare(s.basePath)
	if shareName == "" {
		return nil, nil, fmt.Errorf("smb base_path 必须以 share 名开头，如 /Media")
	}
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("smb 连接 %s 失败: %w", addr, err)
	}
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	d := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: s.username, Password: s.password}}
	sess, err := d.DialConn(dialCtx, conn, addr)
	if err != nil {
		cancel()
		conn.Close()
		return nil, nil, fmt.Errorf("smb 认证失败: %w", err)
	}
	share, err := sess.Mount(shareName)
	if err != nil {
		cancel()
		_ = sess.Logoff()
		return nil, nil, fmt.Errorf("smb 挂载共享 %s 失败: %w", shareName, err)
	}
	cleanup := func() {
		_ = share.Umount()
		_ = sess.Logoff()
		cancel()
	}
	return share, cleanup, nil
}

// rel 把用户路径 p 解析为 share 内相对路径（钳制在 basePath 下；根目录返回 "."）。
func (s *SMB) rel(p string) string {
	full := resolvePath(s.basePath, p)
	_, sub := splitShare(full)
	r := strings.TrimPrefix(sub, "/")
	if r == "" {
		return "."
	}
	return r
}

// List 列目录。
func (s *SMB) List(ctx context.Context, p string) ([]Entry, error) {
	share, cleanup, err := s.session(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	infos, err := share.ReadDir(s.rel(p))
	if err != nil {
		return nil, fmt.Errorf("smb 列目录失败: %w", err)
	}
	parent := normalizePath(resolvePath(s.basePath, p))
	out := make([]Entry, 0, len(infos))
	for _, fi := range infos {
		out = append(out, Entry{
			Name:    fi.Name(),
			Path:    parent + "/" + fi.Name(),
			IsDir:   fi.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		})
	}
	return out, nil
}

// Upload 创建/覆盖文件并写入。
func (s *SMB) Upload(ctx context.Context, p string, r io.Reader) error {
	share, cleanup, err := s.session(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	f, err := share.Create(s.rel(p))
	if err != nil {
		return fmt.Errorf("smb 创建文件失败: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil { // *smb2.File 实现 ReadFrom，io.Copy 高效
		return fmt.Errorf("smb 写入失败: %w", err)
	}
	return nil
}

// Download 打开文件流式读取；Close 时同时释放 SMB 会话。
func (s *SMB) Download(ctx context.Context, p string) (io.ReadCloser, error) {
	share, cleanup, err := s.session(ctx)
	if err != nil {
		return nil, err
	}
	f, err := share.Open(s.rel(p))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("smb 打开文件失败: %w", err)
	}
	return &smbReadCloser{f: f, cleanup: cleanup}, nil
}

type smbReadCloser struct {
	f       *smb2.File
	cleanup func()
	closed  bool
}

func (r *smbReadCloser) Read(p []byte) (int, error) { return r.f.Read(p) }
func (r *smbReadCloser) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	err := r.f.Close()
	r.cleanup()
	return err
}

// Delete 删除文件或目录（目录递归）。
func (s *SMB) Delete(ctx context.Context, p string) error {
	share, cleanup, err := s.session(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	rel := s.rel(p)
	if rel == "." {
		return fmt.Errorf("不允许删除存储根目录")
	}
	if fi, statErr := share.Stat(rel); statErr == nil && fi.IsDir() {
		if err := share.RemoveAll(rel); err != nil {
			return fmt.Errorf("smb 删除目录失败: %w", err)
		}
		return nil
	}
	if err := share.Remove(rel); err != nil {
		return fmt.Errorf("smb 删除失败: %w", err)
	}
	return nil
}

// Mkdir 建目录。
func (s *SMB) Mkdir(ctx context.Context, p string) error {
	share, cleanup, err := s.session(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := share.Mkdir(s.rel(p), 0755); err != nil {
		return fmt.Errorf("smb 建目录失败: %w", err)
	}
	return nil
}

// Rename 同目录改名（go-smb2 Share.Rename）。newName 经 sanitizeNewName 钳制，防
// `name="../x"` 经 path.Join 折叠后上溯逃出 basePath 到 share 根。
func (s *SMB) Rename(ctx context.Context, p, newName string) error {
	share, cleanup, err := s.session(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	oldRel := s.rel(p)
	name, err := sanitizeNewName(newName)
	if err != nil {
		return err
	}
	newRel := path.Join(path.Dir(oldRel), name)
	if err := share.Rename(oldRel, newRel); err != nil {
		return fmt.Errorf("smb 重命名失败: %w", err)
	}
	return nil
}

// Move 跨目录改名（SMB Rename 支持同 share 内跨目录移动）。
func (s *SMB) Move(ctx context.Context, src, dstDir string) error {
	share, cleanup, err := s.session(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	srcRel := s.rel(src)
	dstRel := path.Join(s.rel(dstDir), path.Base(srcRel))
	if err := share.Rename(srcRel, dstRel); err != nil {
		return fmt.Errorf("smb 移动失败: %w", err)
	}
	return nil
}

// Copy 手动读+写（go-smb2 无原生 copy）。源为目录时递归复制（与 local/agent/webdav 一致），
// 避免原先对目录 share.Open 拿目录句柄 io.Copy 失败、且已在目标残留同名空文件的问题。
func (s *SMB) Copy(ctx context.Context, src, dstDir string) error {
	share, cleanup, err := s.session(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	srcRel := s.rel(src)
	dstRel := path.Join(s.rel(dstDir), path.Base(srcRel))
	fi, err := share.Stat(srcRel)
	if err != nil {
		return fmt.Errorf("smb stat 源失败: %w", err)
	}
	if fi.IsDir() {
		return smbCopyTree(share, srcRel, dstRel)
	}
	return smbCopyFile(share, srcRel, dstRel)
}

// smbCopyFile 在同一 share 内复制单文件（手动读+写）。out.Close 的错误不被吞
// （同 local.copyFileLocal：写错误延迟到 close 才报时，吞掉会让上层误判成功）。
func smbCopyFile(share *smb2.Share, src, dst string) error {
	in, err := share.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := share.Create(dst)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	if cerr := out.Close(); copyErr == nil {
		copyErr = cerr
	}
	return copyErr
}

// smbCopyTree 递归复制目录树（同 share 内）。父目录用 Mkdir 建立并容忍已存在。
func smbCopyTree(share *smb2.Share, src, dst string) error {
	_ = share.Mkdir(dst, 0755) // 已存在不报错
	infos, err := share.ReadDir(src)
	if err != nil {
		return err
	}
	for _, fi := range infos {
		sp := path.Join(src, fi.Name())
		dp := path.Join(dst, fi.Name())
		if fi.IsDir() {
			if err := smbCopyTree(share, sp, dp); err != nil {
				return err
			}
		} else if err := smbCopyFile(share, sp, dp); err != nil {
			return err
		}
	}
	return nil
}

// splitShare 把 /share/sub/path 拆成 share 名与 share 内绝对路径。
// /Media/Movies → ("Media", "/Movies")；/Media → ("Media", "/")；""/"/" → ("", "/")。
func splitShare(p string) (share, sub string) {
	p = strings.Trim(path.Clean("/"+strings.TrimLeft(p, "/")), "/")
	if p == "" || p == "." {
		return "", "/"
	}
	parts := strings.SplitN(p, "/", 2)
	share = parts[0]
	if len(parts) > 1 && parts[1] != "" {
		sub = "/" + parts[1]
	} else {
		sub = "/"
	}
	return share, sub
}
