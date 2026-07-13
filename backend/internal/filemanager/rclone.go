package filemanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

// Rclone 后端：通过调用 rclone 二进制的子进程 CLI（lsjson/mkdir/purge/deletefile/copyto）
// 访问任意 rclone remote。参考 MP-2 的 rclone.py（同样用 subprocess CLI，而非 rc API/FUSE）。
//
// 字段映射（与 manager.buildBackend 约定）：
//   - binary        = f.URL        rclone 二进制路径（/usr/bin/rclone、rclone.exe，或空=用 PATH 里的 rclone）
//   - remote        = f.Username   rclone remote 名（如 myremote，不含尾随冒号）
//   - confContent   = password     已解密的 rclone.conf INI 原文（含 [myremote] 段）
//   - rootDir       = f.BasePath   remote 内的根子路径（虚拟根），把所有操作钳制其下
//
// 安全：每次操作把 confContent 写入仅本进程可读的临时文件（0600），用 --config 指定后立即删除，
// 避免把含凭据的 rclone.conf 落盘到固定路径。resolve 经 resolvePath 钳制 ../ 越界（与 smb/webdav 一致）。
type Rclone struct {
	binary      string
	remote      string
	confContent string
	rootDir     string
	// execFunc 执行一条 rclone 命令并返回 stdout；默认走真实 exec，测试可覆盖以注入桩。
	execFunc func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewRclone 构造。构造期不启动任何子进程（仅 exec.LookPath 校验二进制，LookPath 是 PATH 搜索非子进程），
// 因此可在 Manager 锁内安全调用。
func NewRclone(binary, remote, confContent, basePath string) (*Rclone, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		// 留空 → 运行时用 PATH 中的 rclone；LookPath 推迟到 run 时（错误在操作时报）。
		binary = "rclone"
	} else {
		if _, err := exec.LookPath(binary); err != nil {
			return nil, fmt.Errorf("rclone 二进制不可用: %w", err)
		}
	}
	remote = strings.TrimSpace(remote)
	remote = strings.TrimRight(remote, ":") // 容错：用户多写了尾随冒号
	if remote == "" {
		return nil, fmt.Errorf("rclone 必须填写 remote 名")
	}
	if strings.ContainsAny(remote, "/\\:") {
		return nil, fmt.Errorf("rclone remote 名不能包含 / \\ 或冒号: %s", remote)
	}
	return &Rclone{
		binary:      binary,
		remote:      remote,
		confContent: confContent,
		rootDir:     strings.TrimSpace(basePath),
		execFunc:    defaultRcloneExec,
	}, nil
}

func (r *Rclone) Type() string { return "rclone" }

// resolve 把虚拟 /-路径（前端约定）映射为 rclone 期望的 `remote:path` 形式。
// 经 resolvePath 把 ../ 与反斜杠变体钳制在 rootDir 下（rootDir 为空则以 / 为边界）。
// 返回形如 "myremote:/Media/Movies"。
func (r *Rclone) resolve(p string) string {
	clamped := resolvePath(r.rootDir, p)
	return r.remote + ":" + clamped
}

// defaultRcloneExec 默认执行器：跑 exec.CommandContext，捕获 stdout/stderr，非零退出返回含 stderr 的错误。
func defaultRcloneExec(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rclone %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// run 写临时 conf、注入 --config、调用 execFunc；ctx 无 deadline 时套 10min 硬上限（大文件拷贝）。
func (r *Rclone) run(ctx context.Context, args ...string) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
	}
	// confContent 写入一次性临时文件（0600），defer 删除，绝不落盘固定路径。
	tmpConf, err := os.CreateTemp("", "ptpatronus-rclone-conf-*")
	if err != nil {
		return nil, fmt.Errorf("rclone 创建配置临时文件失败: %w", err)
	}
	tmpConfPath := tmpConf.Name()
	defer os.Remove(tmpConfPath)
	if r.confContent != "" {
		if _, werr := tmpConf.WriteString(r.confContent); werr != nil {
			tmpConf.Close()
			return nil, fmt.Errorf("rclone 写配置临时文件失败: %w", werr)
		}
	}
	if cerr := tmpConf.Close(); cerr != nil {
		return nil, fmt.Errorf("rclone 关闭配置临时文件失败: %w", cerr)
	}
	full := append([]string{"--config", tmpConfPath}, args...)
	return r.execFunc(ctx, r.binary, full...)
}

// rcloneItem 对应 rclone lsjson 输出的单条（Name/IsDir/Size/ModTime，ModTime 为 RFC3339）。
type rcloneItem struct {
	Name    string `json:"Name"`
	IsDir   bool   `json:"IsDir"`
	Size    int64  `json:"Size"`
	ModTime string `json:"ModTime"`
}

// List 用 lsjson 列目录。Entry.Path 取虚拟路径 parent+"/"+Name（与 local/smb 一致，含 rootDir 前缀）。
func (r *Rclone) List(ctx context.Context, p string) ([]Entry, error) {
	out, err := r.run(ctx, "lsjson", r.resolve(p))
	if err != nil {
		return nil, err
	}
	var items []rcloneItem
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("rclone lsjson 解析失败: %w", err)
	}
	parent := normalizePath(resolvePath(r.rootDir, p))
	entries := make([]Entry, 0, len(items))
	for _, it := range items {
		entries = append(entries, Entry{
			Name:    it.Name,
			Path:    parent + "/" + it.Name,
			IsDir:   it.IsDir,
			Size:    it.Size,
			ModTime: parseRcloneTime(it.ModTime),
		})
	}
	return entries, nil
}

// Upload 先把 reader 落到本地临时文件，再 rclone copyto 本地→远端（copyto 会自动建父目录）。
func (r *Rclone) Upload(ctx context.Context, p string, rd io.Reader) error {
	tmp, err := os.CreateTemp("", "ptpatronus-rclone-up-*")
	if err != nil {
		return fmt.Errorf("rclone 创建上传临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // 任一步失败都清理本地临时文件
	if _, err := io.Copy(tmp, rd); err != nil {
		tmp.Close()
		return fmt.Errorf("rclone 写入上传缓冲失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("rclone 关闭上传缓冲失败: %w", err)
	}
	_, err = r.run(ctx, "copyto", tmpPath, r.resolve(p))
	return err
}

// Download 先 rclone copyto 远端→本地临时文件，再打开供调用方读取；Close 时删除临时文件。
func (r *Rclone) Download(ctx context.Context, p string) (io.ReadCloser, error) {
	tmp, err := os.CreateTemp("", "ptpatronus-rclone-dl-*")
	if err != nil {
		return nil, fmt.Errorf("rclone 创建下载临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close() // 仅占名，由 rclone 写入
	if _, err := r.run(ctx, "copyto", r.resolve(p), tmpPath); err != nil {
		os.Remove(tmpPath)
		return nil, err
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("rclone 打开下载结果失败: %w", err)
	}
	return &tempFileReadCloser{File: f, path: tmpPath}, nil
}

// tempFileReadCloser 包装 *os.File，Close 时一并删除底层临时文件。
type tempFileReadCloser struct {
	*os.File
	path   string
	closed bool
}

func (t *tempFileReadCloser) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	err := t.File.Close()
	_ = os.Remove(t.path)
	return err
}

// isDirPath 判断 clamped（虚拟 /-路径，已钳制在 rootDir 下）是否为目录。
// 通过 lsjson 父目录并按 basename 匹配 IsDir（参考 MP-2 get_item）。根目录恒为目录。
func (r *Rclone) isDirPath(ctx context.Context, clamped string) (isDir, found bool, err error) {
	if clamped == "" || clamped == "/" {
		return true, true, nil
	}
	parent := normalizePath(path.Dir(clamped))
	name := path.Base(clamped)
	out, err := r.run(ctx, "lsjson", r.remote+":"+parent)
	if err != nil {
		return false, false, err
	}
	var items []rcloneItem
	if err := json.Unmarshal(out, &items); err != nil {
		return false, false, fmt.Errorf("rclone lsjson 解析失败: %w", err)
	}
	for _, it := range items {
		if it.Name == name {
			return it.IsDir, true, nil
		}
	}
	return false, false, nil
}

// Delete 先探测类型：目录用 purge（递归），文件用 deletefile；不存在视为已删除（幂等，与 webdav 404 处理一致）。
func (r *Rclone) Delete(ctx context.Context, p string) error {
	clamped := resolvePath(r.rootDir, p)
	rp := r.remote + ":" + clamped
	isDir, found, err := r.isDirPath(ctx, clamped)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if isDir {
		_, err = r.run(ctx, "purge", rp)
	} else {
		_, err = r.run(ctx, "deletefile", rp)
	}
	return err
}

// Mkdir 用 rclone mkdir 建目录。
func (r *Rclone) Mkdir(ctx context.Context, p string) error {
	_, err := r.run(ctx, "mkdir", r.resolve(p))
	return err
}

// parseRcloneTime 解析 rclone lsjson 的 ModTime（RFC3339，可能带小数秒）。
// Go time.Parse 即便 layout 无小数位也能容忍输入带小数秒，故单一 RFC3339 layout 足够；失败返回零值。
func parseRcloneTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
