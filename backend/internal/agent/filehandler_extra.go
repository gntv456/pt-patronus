// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package agent

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// filehandler_extra.go — agent 端扩展文件操作（readfile/writefile/archive/extract/chmod/dirsize）。
// 与 filehandler.go 同包，独立成文件避免超 400 行。压缩/解压独立实现（agent 不 import filemanager）。

const agentMaxReadSize = 10 * 1024 * 1024

// resolveDstPath 钳制 Dst 字段（archive/extract 目标）到 base 下并校验白名单。
func (a *agentConn) resolveDstPath(env Envelope) string {
	full := safePath(env.Cmd.Base, env.Cmd.Dst)
	if !a.allowed(full) {
		return ""
	}
	return full
}

// cmdReadFile 读取文件内容（≤10MB，小操作响应回传 Content）。
func (a *agentConn) cmdReadFile(env Envelope) {
	full := a.resolve(env)
	if full == "" {
		_ = a.writeResp(env.ID, false, "path denied by ALLOWED_ROOTS", nil)
		return
	}
	info, err := os.Stat(filepath.FromSlash(full))
	if err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	if info.IsDir() {
		_ = a.writeResp(env.ID, false, "不能读取目录内容", nil)
		return
	}
	if info.Size() > agentMaxReadSize {
		_ = a.writeResp(env.ID, false, "文件过大，超过 10MB 限制", nil)
		return
	}
	data, err := os.ReadFile(filepath.FromSlash(full))
	if err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	_ = a.writeJSON(Envelope{Type: TypeResp, ID: env.ID, Resp: &Response{OK: true, Content: string(data)}})
}

// cmdWriteFile 写入文件内容（原子写）。
func (a *agentConn) cmdWriteFile(env Envelope) {
	full := a.resolve(env)
	if full == "" {
		_ = a.writeResp(env.ID, false, "path denied by ALLOWED_ROOTS", nil)
		return
	}
	tmp := filepath.FromSlash(full) + ".agent-tmp"
	if err := os.WriteFile(tmp, []byte(env.Cmd.Content), 0o644); err != nil {
		os.Remove(tmp)
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	if err := os.Rename(tmp, filepath.FromSlash(full)); err != nil {
		os.Remove(tmp)
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	_ = a.writeResp(env.ID, true, "", nil)
}

// cmdArchive 压缩 Path 为 Dst（按扩展名选 zip/tar.gz）。
func (a *agentConn) cmdArchive(env Envelope) {
	srcFull := a.resolve(env)
	if srcFull == "" {
		_ = a.writeResp(env.ID, false, "src path denied", nil)
		return
	}
	dstFull := a.resolveDstPath(env)
	if dstFull == "" {
		_ = a.writeResp(env.ID, false, "dst path denied", nil)
		return
	}
	if agentSameOrUnder(srcFull, dstFull) {
		_ = a.writeResp(env.ID, false, "目标压缩包不能位于源内部", nil)
		return
	}
	low := strings.ToLower(dstFull)
	var err error
	switch {
	case strings.HasSuffix(low, ".zip"):
		err = agentZip(srcFull, dstFull)
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"):
		err = agentTarGz(srcFull, dstFull)
	default:
		err = agentTarGz(srcFull, dstFull+".tar.gz")
	}
	if err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	_ = a.writeResp(env.ID, true, "", nil)
}

// cmdExtract 解压 Path 到 Dst 目录。
func (a *agentConn) cmdExtract(env Envelope) {
	srcFull := a.resolve(env)
	if srcFull == "" {
		_ = a.writeResp(env.ID, false, "src path denied", nil)
		return
	}
	dstFull := a.resolveDstPath(env)
	if dstFull == "" {
		_ = a.writeResp(env.ID, false, "dst path denied", nil)
		return
	}
	if err := os.MkdirAll(filepath.FromSlash(dstFull), 0o755); err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	low := strings.ToLower(srcFull)
	var err error
	switch {
	case strings.HasSuffix(low, ".zip"):
		err = agentUnzip(srcFull, dstFull)
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"):
		err = agentUntarGz(srcFull, dstFull)
	default:
		err = errUnsupportedFmt
	}
	if err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	_ = a.writeResp(env.ID, true, "", nil)
}

// cmdChmod 修改权限。
func (a *agentConn) cmdChmod(env Envelope) {
	full := a.resolve(env)
	if full == "" {
		_ = a.writeResp(env.ID, false, "path denied by ALLOWED_ROOTS", nil)
		return
	}
	fm := os.FileMode(env.Cmd.Mode)
	var err error
	if env.Cmd.Recursive {
		err = filepath.Walk(filepath.FromSlash(full), func(p string, info os.FileInfo, werr error) error {
			if werr != nil {
				return werr
			}
			return os.Chmod(p, fm)
		})
	} else {
		err = os.Chmod(filepath.FromSlash(full), fm)
	}
	if err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	_ = a.writeResp(env.ID, true, "", nil)
}

// cmdDirSize 计算目录递归大小。
func (a *agentConn) cmdDirSize(env Envelope) {
	full := a.resolve(env)
	if full == "" {
		_ = a.writeResp(env.ID, false, "path denied by ALLOWED_ROOTS", nil)
		return
	}
	info, err := os.Stat(filepath.FromSlash(full))
	if err != nil {
		_ = a.writeResp(env.ID, false, err.Error(), nil)
		return
	}
	var total int64
	if info.IsDir() {
		_ = filepath.Walk(filepath.FromSlash(full), func(_ string, fi os.FileInfo, werr error) error {
			if werr != nil {
				return werr
			}
			if !fi.IsDir() {
				total += fi.Size()
			}
			return nil
		})
	} else {
		total = info.Size()
	}
	_ = a.writeJSON(Envelope{Type: TypeResp, ID: env.ID, Resp: &Response{OK: true, Size: total}})
}

var errUnsupportedFmt = &simpleErr{"不支持的压缩格式（仅 .zip / .tar.gz / .tgz）"}

type simpleErr struct{ msg string }

func (e *simpleErr) Error() string { return e.msg }

// agentSameOrUnder 判断 dst 是否等于 src 或在 src 子树内。
func agentSameOrUnder(src, dst string) bool {
	cs := filepath.Clean(src)
	cd := filepath.Clean(dst)
	if cs == cd {
		return true
	}
	return strings.HasPrefix(cd, cs+string(filepath.Separator))
}

// —— 压缩/解压实现 ——

func agentZip(src, dst string) error {
	f, err := os.Create(filepath.FromSlash(dst))
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	return filepath.Walk(filepath.FromSlash(src), func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Dir(filepath.FromSlash(src)), path)
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		if fi.IsDir() {
			h.Name += "/"
		} else {
			h.Method = zip.Deflate
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rf, err := os.Open(path)
		if err != nil {
			return err
		}
		defer rf.Close()
		_, err = io.Copy(w, rf)
		return err
	})
}

func agentTarGz(src, dst string) error {
	f, err := os.Create(filepath.FromSlash(dst))
	if err != nil {
		return err
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	return filepath.Walk(filepath.FromSlash(src), func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		h, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Dir(filepath.FromSlash(src)), path)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rf, err := os.Open(path)
		if err != nil {
			return err
		}
		defer rf.Close()
		_, err = io.Copy(tw, rf)
		return err
	})
}

// agent 解压上限（四轮 P1 修复：与本地后端 filemanager/local_extra.go 的
// maxExtractEntrySize/maxExtractTotalSize 对齐——此前 agent 端无任何限制，恶意/
// 损坏的压缩包（zip bomb）可写满 agent 主机磁盘）。
const (
	agentMaxExtractEntry int64 = 100 << 20 // 单文件 100MB
	agentMaxExtractTotal int64 = 500 << 20 // 累计 500MB
)

// agentExtractTracker 累计解压总量守卫。
type agentExtractTracker struct{ total int64 }

func (t *agentExtractTracker) copy(out io.Writer, rc io.Reader) (int64, error) {
	n, err := io.Copy(out, io.LimitReader(rc, agentMaxExtractEntry+1))
	if n > agentMaxExtractEntry {
		return n, &simpleErr{"解压失败：单文件超过 100MB 上限"}
	}
	t.total += n
	if t.total > agentMaxExtractTotal {
		return n, &simpleErr{"解压失败：总解压量超过 500MB 上限"}
	}
	return n, err
}

func agentUnzip(src, dst string) error {
	r, err := zip.OpenReader(filepath.FromSlash(src))
	if err != nil {
		return err
	}
	defer r.Close()
	tracker := &agentExtractTracker{}
	for _, f := range r.File {
		target := filepath.Join(filepath.FromSlash(dst), f.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(filepath.Separator), filepath.Clean(filepath.FromSlash(dst))+string(filepath.Separator)) {
			return &simpleErr{"zip slip 检测：非法路径 " + f.Name}
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		if _, cerr := tracker.copy(out, rc); cerr != nil {
			rc.Close()
			out.Close()
			return cerr
		}
		rc.Close()
		out.Close()
	}
	return nil
}

func agentUntarGz(src, dst string) error {
	f, err := os.Open(filepath.FromSlash(src))
	if err != nil {
		return err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	tracker := &agentExtractTracker{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(filepath.FromSlash(dst), h.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(filepath.Separator), filepath.Clean(filepath.FromSlash(dst))+string(filepath.Separator)) {
			return &simpleErr{"tar slip 检测：非法路径 " + h.Name}
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(h.Mode)|0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(h.Mode))
			if err != nil {
				return err
			}
			if _, cerr := tracker.copy(out, tr); cerr != nil {
				out.Close()
				return cerr
			}
			out.Close()
		}
	}
	return nil
}
