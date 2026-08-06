package plugins

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	maxPluginEntrySize int64 = 32 << 20 // 单文件解压上限 32MB
	maxPluginTotalSize int64 = 96 << 20 // 全包累计上限 96MB
)

func unzipSafe(raw []byte, dst string) error {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	// zip 炸弹防护：单条目 + 累计总量双上限。
	var total int64
	for _, f := range zr.File {
		target := filepath.Join(dst, filepath.Clean(f.Name))
		if !isSubpath(dst, target) {
			return fmt.Errorf("unsafe zip path %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		// 预检声明大小（UncompressedSize64 可被伪造，下面 LimitReader 双保险）。
		if int64(f.UncompressedSize64) > maxPluginEntrySize {
			return fmt.Errorf("plugin entry %q too large after decompress (%d bytes, max %d)", f.Name, f.UncompressedSize64, maxPluginEntrySize)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.FileInfo().Mode())
		if err != nil {
			rc.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(rc, maxPluginEntrySize+1))
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if n > maxPluginEntrySize {
			return fmt.Errorf("plugin entry %q exceeds per-file decompress limit", f.Name)
		}
		if closeErr != nil {
			return closeErr
		}
		total += n
		if total > maxPluginTotalSize {
			return fmt.Errorf("plugin archive exceeds total decompress limit (%d bytes)", maxPluginTotalSize)
		}
	}
	return nil
}

func findManifest(root string) (string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "plugin.json" {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return "", errors.New("plugin.json not found in archive")
	}
	if len(found) > 1 {
		return "", errors.New("archive contains multiple plugin.json files")
	}
	return found[0], nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if !isSubpath(dst, target) {
			return fmt.Errorf("unsafe copy path %s", target)
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

// installPythonRequirements 把声明的 pip 需求装到 {pluginDir}/.deps（best-effort）。
// pip/pip3 不在 PATH 时静默跳过（宿主没装 pip 就没法装依赖）；pip 存在但安装失败时返回错误，
// 由调用方记录但不阻断安装（插件可能仅靠 stdlib 仍可运行）。--target 装入 .deps 后，进程启动时
// 由 external.go 把 .deps 注入 PYTHONPATH。
func installPythonRequirements(pluginDir string, reqs []string) error {
	pip, err := exec.LookPath("pip")
	if err != nil {
		if pip, err = exec.LookPath("pip3"); err != nil {
			return nil
		}
	}
	depsDir := filepath.Join(pluginDir, ".deps")
	if err := os.MkdirAll(depsDir, 0o755); err != nil {
		return err
	}
	args := append([]string{"install", "--quiet", "--disable-pip-version-check", "--no-warn-script-location", "--target", depsDir}, reqs...)
	cmd := exec.Command(pip, args...)
	cmd.Dir = pluginDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip install %s: %w: %s", strings.Join(reqs, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isSubpath(root, target string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

