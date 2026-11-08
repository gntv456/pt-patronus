package filemanager

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// local_extra.go — Local 后端的扩展能力（在线编辑/压缩/解压/chmod/目录大小）。
// 与 local.go 同包，独立成文件避免超 400 行。所有方法均经 resolve 钳制路径 + 拒绝操作根目录。

// maxReadSize 限制读取文件内容的大小，防止内存爆。
const maxReadSize = 10 * 1024 * 1024

// ReadFile 读取文件全部内容（在线编辑/文本预览用）。
func (l *Local) ReadFile(ctx context.Context, p string) ([]byte, error) {
	full, err := l.resolve(p)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("不能读取目录内容")
	}
	if info.Size() > maxReadSize {
		return nil, fmt.Errorf("文件过大（%d 字节），超过 %d 限制", info.Size(), maxReadSize)
	}
	return os.ReadFile(full)
}

// WriteFile 写入文件全部内容（在线编辑保存）。原子写避免断电损坏。
func (l *Local) WriteFile(ctx context.Context, p string, data []byte) error {
	full, err := l.resolve(p)
	if err != nil {
		return err
	}
	info, err := os.Stat(full)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if info != nil && info.IsDir() {
		return fmt.Errorf("不能覆盖目录")
	}
	tmp := full + ".ptpatronus-tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, full)
}

// Archive 压缩 src（文件或目录）为 dst（按扩展名选 zip 或 tar.gz）。
func (l *Local) Archive(ctx context.Context, src, dst string) error {
	srcFull, err := l.resolve(src)
	if err != nil {
		return err
	}
	dstFull, err := l.resolve(dst)
	if err != nil {
		return err
	}
	if isSameOrUnder(srcFull, dstFull) {
		return fmt.Errorf("目标压缩包不能位于源内部")
	}
	low := strings.ToLower(dstFull)
	switch {
	case strings.HasSuffix(low, ".zip"):
		return zipPath(srcFull, dstFull)
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"):
		return tarGzPath(srcFull, dstFull)
	default:
		return tarGzPath(srcFull, dstFull+".tar.gz")
	}
}

// Extract 解压 src（.zip/.tar.gz）到 dstDir。
func (l *Local) Extract(ctx context.Context, src, dstDir string) error {
	srcFull, err := l.resolve(src)
	if err != nil {
		return err
	}
	dstFull, err := l.resolve(dstDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dstFull, 0o755); err != nil {
		return err
	}
	low := strings.ToLower(srcFull)
	switch {
	case strings.HasSuffix(low, ".zip"):
		return unzipTo(srcFull, dstFull)
	case strings.HasSuffix(low, ".tar.gz"), strings.HasSuffix(low, ".tgz"):
		return untarGzTo(srcFull, dstFull)
	default:
		return fmt.Errorf("不支持的压缩格式（仅 .zip / .tar.gz / .tgz）")
	}
}

// Chmod 修改文件/目录权限位。recursive=true 时递归。
func (l *Local) Chmod(ctx context.Context, p string, mode uint32, recursive bool) error {
	full, err := l.resolve(p)
	if err != nil {
		return err
	}
	if filepath.Clean(full) == filepath.Clean(l.rootDir) {
		return fmt.Errorf("不允许修改存储根目录权限")
	}
	fm := os.FileMode(mode)
	if !recursive {
		return os.Chmod(full, fm)
	}
	return filepath.Walk(full, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, fm)
	})
}

// DirSize 递归计算目录总大小（字节）。
func (l *Local) DirSize(ctx context.Context, p string) (int64, error) {
	full, err := l.resolve(p)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}
	var total int64
	err = filepath.Walk(full, func(_ string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total, err
}

// —— 压缩/解压内部实现 ——

func zipPath(src, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Dir(src), path)
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

func tarGzPath(src, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		h, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Dir(src), path)
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

// maxExtractEntrySize limits single-file decompress to prevent zip-bomb disk exhaustion.
// maxExtractTotalSize limits the cumulative extracted total (aligned with market archive limits).
const (
	maxExtractEntrySize int64 = 100 << 20 // 100MB per file
	maxExtractTotalSize int64 = 500 << 20 // 500MB total
)

func unzipTo(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	var total int64
	for _, f := range r.File {
		target := filepath.Join(dst, filepath.Clean(f.Name))
		if !isSubpath(dst, target) {
			return fmt.Errorf("zip slip detected: unsafe path %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if int64(f.UncompressedSize64) > maxExtractEntrySize {
			return fmt.Errorf("zip entry %q too large (%d bytes, max %d)", f.Name, f.UncompressedSize64, maxExtractEntrySize)
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
		n, copyErr := io.Copy(out, io.LimitReader(rc, maxExtractEntrySize+1))
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if n > maxExtractEntrySize {
			return fmt.Errorf("zip entry %q exceeds decompress limit", f.Name)
		}
		if closeErr != nil {
			return closeErr
		}
		total += n
		if total > maxExtractTotalSize {
			return fmt.Errorf("zip archive exceeds total decompress limit (%d bytes)", maxExtractTotalSize)
		}
	}
	return nil
}

func untarGzTo(src, dst string) error {
	f, err := os.Open(src)
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
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.Clean(h.Name))
		if !isSubpath(dst, target) {
			return fmt.Errorf("tar slip detected: unsafe path %s", h.Name)
		}
		if h.Size > maxExtractEntrySize {
			return fmt.Errorf("tar entry %q too large (%d bytes, max %d)", h.Name, h.Size, maxExtractEntrySize)
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
			n, copyErr := io.Copy(out, io.LimitReader(tr, maxExtractEntrySize+1))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if n > maxExtractEntrySize {
				return fmt.Errorf("tar entry %q exceeds decompress limit", h.Name)
			}
			if closeErr != nil {
				return closeErr
			}
			total += n
			if total > maxExtractTotalSize {
				return fmt.Errorf("tar archive exceeds total decompress limit (%d bytes)", maxExtractTotalSize)
			}
		}
	}
	return nil
}
