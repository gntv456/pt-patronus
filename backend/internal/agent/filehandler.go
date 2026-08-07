package agent

import (
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// handleCmd 分发服务端文件命令。
func (a *agentConn) handleCmd(env Envelope) {
	if env.Cmd == nil {
		return
	}
	switch env.Cmd.Op {
	case "list":
		a.cmdList(env)
	case "mkdir":
		a.cmdMkdir(env)
	case "delete":
		a.cmdDelete(env)
	case "rename":
		a.cmdRename(env)
	case "move":
		a.cmdMove(env)
	case "copy":
		a.cmdCopy(env)
	case "download":
		a.cmdDownload(env)
	case "upload":
		a.cmdUpload(env)
	case "readfile":
		a.cmdReadFile(env)
	case "writefile":
		a.cmdWriteFile(env)
	case "archive":
		a.cmdArchive(env)
	case "extract":
		a.cmdExtract(env)
	case "chmod":
		a.cmdChmod(env)
	case "dirsize":
		a.cmdDirSize(env)
	default:
		_ = a.writeResp(env.ID, false, "unknown op: "+env.Cmd.Op, nil)
	}
}

// writeResp 写小操作响应（ok/err/entries）。
func (a *agentConn) writeResp(id string, ok bool, errMsg string, entries []Entry) error {
	return a.writeJSON(Envelope{Type: TypeResp, ID: id, Resp: &Response{OK: ok, Err: errMsg, Entries: entries}})
}

// resolve 把命令路径钳制到 base 下并校验 ALLOWED_ROOTS 白名单；被拒返回 ""。
func (a *agentConn) resolve(env Envelope) string {
	full := safePath(env.Cmd.Base, env.Cmd.Path)
	if !a.allowed(full) {
		return ""
	}
	return full
}

// allowed 校验路径是否在 ALLOWED_ROOTS 白名单内（空白名单=不限，仅靠 base 钳制）。
func (a *agentConn) allowed(full string) bool {
	if len(a.cfg.AllowedRoots) == 0 {
		return true
	}
	for _, root := range a.cfg.AllowedRoots {
		r := strings.TrimRight(root, "/")
		if r == "" {
			continue
		}
		if full == r || strings.HasPrefix(full, r+"/") {
			return true
		}
	}
	return false
}

func (a *agentConn) cmdList(env Envelope) {
	full := a.resolve(env)
	if full == "" {
		_ = a.writeResp(env.ID, false, "path denied by ALLOWED_ROOTS", nil)
		return
	}
	dirEntries, err := os.ReadDir(filepath.FromSlash(full))
	if err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	parent := normalizePath(full)
	entries := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		modTime := time.Time{}
		size := int64(0)
		if info, err := de.Info(); err == nil {
			modTime = info.ModTime()
			size = info.Size()
		}
		entries = append(entries, Entry{
			Name:    de.Name(),
			Path:    parent + "/" + de.Name(),
			IsDir:   de.IsDir(),
			Size:    size,
			ModTime: modTime,
		})
	}
	_ = a.writeResp(env.ID, true, "", entries)
}

func (a *agentConn) cmdMkdir(env Envelope) {
	full := a.resolve(env)
	if full == "" {
		_ = a.writeResp(env.ID, false, "path denied by ALLOWED_ROOTS", nil)
		return
	}
	if err := os.MkdirAll(filepath.FromSlash(full), 0755); err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	_ = a.writeResp(env.ID, true, "", nil)
}

func (a *agentConn) cmdDelete(env Envelope) {
	full := a.resolve(env)
	if full == "" {
		_ = a.writeResp(env.ID, false, "path denied by ALLOWED_ROOTS", nil)
		return
	}
	if err := os.RemoveAll(filepath.FromSlash(full)); err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	_ = a.writeResp(env.ID, true, "", nil)
}

// cmdRename 同目录改名（NewName 经 filepath.Base 钳制防注入路径）。
func (a *agentConn) cmdRename(env Envelope) {
	srcFull := a.resolve(env)
	if srcFull == "" {
		_ = a.writeResp(env.ID, false, "path denied by ALLOWED_ROOTS", nil)
		return
	}
	newName := filepath.Base(filepath.FromSlash(env.Cmd.NewName))
	dst := filepath.Join(filepath.Dir(srcFull), newName)
	if err := os.Rename(srcFull, dst); err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	_ = a.writeResp(env.ID, true, "", nil)
}

// resolveDst 钳制目标目录（cmd.DstDir）到 base 下并校验白名单。
func (a *agentConn) resolveDst(env Envelope) string {
	full := safePath(env.Cmd.Base, env.Cmd.DstDir)
	if !a.allowed(full) {
		return ""
	}
	return full
}

// cmdMove 移到 DstDir（保留文件名；os.Rename 跨设备失败时降级 copy+delete）。
func (a *agentConn) cmdMove(env Envelope) {
	srcFull := a.resolve(env)
	if srcFull == "" {
		_ = a.writeResp(env.ID, false, "src path denied by ALLOWED_ROOTS", nil)
		return
	}
	dstDirFull := a.resolveDst(env)
	if dstDirFull == "" {
		_ = a.writeResp(env.ID, false, "dst path denied by ALLOWED_ROOTS", nil)
		return
	}
	dst := filepath.Join(dstDirFull, filepath.Base(srcFull))
	if err := os.Rename(srcFull, dst); err == nil {
		_ = a.writeResp(env.ID, true, "", nil)
		return
	}
	if err := agentCopyPath(srcFull, dst); err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	if err := os.RemoveAll(srcFull); err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	_ = a.writeResp(env.ID, true, "", nil)
}

// cmdCopy 复制到 DstDir（保留文件名）。
func (a *agentConn) cmdCopy(env Envelope) {
	srcFull := a.resolve(env)
	if srcFull == "" {
		_ = a.writeResp(env.ID, false, "src path denied by ALLOWED_ROOTS", nil)
		return
	}
	dstDirFull := a.resolveDst(env)
	if dstDirFull == "" {
		_ = a.writeResp(env.ID, false, "dst path denied by ALLOWED_ROOTS", nil)
		return
	}
	dst := filepath.Join(dstDirFull, filepath.Base(srcFull))
	if err := agentCopyPath(srcFull, dst); err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	_ = a.writeResp(env.ID, true, "", nil)
}

// agentCopyPath 递归复制文件或目录（文件 io.Copy，目录 filepath.Walk）。
func agentCopyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return agentCopyDir(src, dst, info.Mode())
	}
	return agentCopyFile(src, dst)
}

// agentCopyDir 递归复制目录树，保留目录权限位。
func agentCopyDir(src, dst string, mode os.FileMode) error {
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
		return agentCopyFile(p, target)
	})
}

// agentCopyFile 逐字节复制单文件（建父目录）。
func agentCopyFile(src, dst string) error {
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

// cmdDownload 流式回传文件：32KB 块 base64 编码发 data 帧，最后 Last:true。
func (a *agentConn) cmdDownload(env Envelope) {
	full := a.resolve(env)
	if full == "" {
		_ = a.writeResp(env.ID, false, "path denied by ALLOWED_ROOTS", nil)
		return
	}
	f, err := os.Open(filepath.FromSlash(full))
	if err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	defer f.Close()
	buf := make([]byte, 32*1024)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			enc := base64.StdEncoding.EncodeToString(buf[:n])
			if werr := a.writeJSON(Envelope{Type: TypeData, ID: env.ID, Chunk: &Chunk{Data: enc}}); werr != nil {
				return
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = a.writeJSON(Envelope{Type: TypeError, ID: env.ID, Error: rerr.Error()})
			return
		}
	}
	_ = a.writeJSON(Envelope{Type: TypeData, ID: env.ID, Chunk: &Chunk{Last: true}})
}

// cmdUpload 打开目标文件等待后续 data 帧（handleData 写盘）；Last 时 finishUpload 回 resp。
func (a *agentConn) cmdUpload(env Envelope) {
	full := a.resolve(env)
	if full == "" {
		_ = a.writeResp(env.ID, false, "path denied by ALLOWED_ROOTS", nil)
		return
	}
	f, err := os.Create(filepath.FromSlash(full))
	if err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	a.uploadMu.Lock()
	a.uploads[env.ID] = f
	a.uploadMu.Unlock()
}

// handleData 接收 upload 的 data 帧写盘；Last 时完成。
func (a *agentConn) handleData(env Envelope) {
	if env.Chunk == nil {
		return
	}
	a.uploadMu.Lock()
	f, ok := a.uploads[env.ID]
	a.uploadMu.Unlock()
	if !ok {
		return
	}
	if data, err := base64.StdEncoding.DecodeString(env.Chunk.Data); err == nil && len(data) > 0 {
		if _, werr := f.Write(data); werr != nil {
			a.finishUpload(env.ID, false)
			return
		}
	}
	if env.Chunk.Last {
		a.finishUpload(env.ID, true)
	}
}

// finishUpload 关闭上传文件并回 resp；ok=false 时删除半成品。
func (a *agentConn) finishUpload(id string, ok bool) {
	a.uploadMu.Lock()
	f, existed := a.uploads[id]
	if existed {
		delete(a.uploads, id)
	}
	a.uploadMu.Unlock()
	if !existed || f == nil {
		return
	}
	name := f.Name()
	f.Close()
	if !ok {
		_ = os.Remove(name)
		_ = a.writeResp(id, false, "upload 写入失败", nil)
		return
	}
	_ = a.writeResp(id, true, "", nil)
}

// cancelUpload server 取消 upload（error 帧）：关闭并删除半成品文件。
func (a *agentConn) cancelUpload(id string) {
	if id == "" {
		return
	}
	a.uploadMu.Lock()
	f, existed := a.uploads[id]
	if existed {
		delete(a.uploads, id)
	}
	a.uploadMu.Unlock()
	if existed && f != nil {
		name := f.Name()
		f.Close()
		_ = os.Remove(name)
	}
}

// normalizePath 规范化为以 "/" 开头、无尾斜杠的绝对路径（独立于 filemanager.normalizePath）。
func normalizePath(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}
