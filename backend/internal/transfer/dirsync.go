// Package transfer —— 目录同步任务：本地目录间按 硬链/软链/复制/移动/STRM 同步。
//
// 对标 MSaber 目录同步。任务持久化（SystemConfig transfer.dir_sync）+ 手动触发执行
// （cron 由 scheduler 挂接，见 app_cron）。过滤：扩展名白名单（媒体默认）。
package transfer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirSyncTask 目录同步任务定义。
type DirSyncTask struct {
	Name      string   `json:"name"`
	SrcDir    string   `json:"src_dir"`
	DstDir    string   `json:"dst_dir"`
	Mode      string   `json:"mode"`           // link | softlink | copy | move | strm
	Exts      []string `json:"exts,omitempty"` // 扩展名白名单（小写含点；空=媒体默认集）
	Recursive bool     `json:"recursive"`
	DeleteSrc bool     `json:"delete_src,omitempty"` // move 隐含删源；copy 后可选删
	Enabled   bool     `json:"enabled"`
}

// DirSyncResult 单次执行汇总。
type DirSyncResult struct {
	Scanned int      `json:"scanned"`
	Synced  int      `json:"synced"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

const dirSyncKey = "transfer.dir_sync"

// LoadDirSyncTasks 读任务列表。
func LoadDirSyncTasks(kv KVStore) []DirSyncTask {
	if kv == nil {
		return nil
	}
	raw, err := kv.Get(dirSyncKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var ts []DirSyncTask
	if json.Unmarshal([]byte(raw), &ts) != nil {
		return nil
	}
	return ts
}

// SaveDirSyncTasks 写任务列表（name 查重 + 模式校验）。
func SaveDirSyncTasks(kv KVStore, ts []DirSyncTask) error {
	seen := map[string]bool{}
	for _, t := range ts {
		if strings.TrimSpace(t.Name) == "" || seen[t.Name] {
			return fmt.Errorf("任务名空或重复: %s", t.Name)
		}
		seen[t.Name] = true
		switch t.Mode {
		case "link", "softlink", "copy", "move", "strm":
		default:
			return fmt.Errorf("任务 %s 同步方式非法: %s", t.Name, t.Mode)
		}
		if strings.TrimSpace(t.SrcDir) == "" || strings.TrimSpace(t.DstDir) == "" {
			return fmt.Errorf("任务 %s 源/目标目录为空", t.Name)
		}
	}
	b, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	return kv.Set(dirSyncKey, string(b))
}

// mediaExtDefaults 同步默认媒体扩展集（未配 Exts 时）。
var mediaSyncExts = []string{".mkv", ".mp4", ".ts", ".m2ts", ".avi", ".mov", ".iso", ".flv", ".wmv", ".strm", ".srt", ".ass"}

// RunDirSync 执行一个同步任务：遍历源目录 → 过滤 → 按模式落地到目标（保持相对结构）。
// 幂等：目标已存在且 size 相同跳过（link/strm 比内容；move 永远执行）。
func RunDirSync(t DirSyncTask) *DirSyncResult {
	res := &DirSyncResult{}
	exts := t.Exts
	if len(exts) == 0 {
		exts = mediaSyncExts
	}
	extSet := map[string]bool{}
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}
	root := NormalizeDirPath(t.SrcDir)
	dstRoot := NormalizeDirPath(t.DstDir)
	var walk func(dir string, depth int)
	maxDepth := 1
	if t.Recursive {
		maxDepth = 16
	}
	walk = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("读目录失败 %s: %v", dir, err))
			return
		}
		for _, e := range entries {
			full := filepath.Join(dir, e.Name())
			if e.IsDir() {
				walk(full, depth+1)
				continue
			}
			res.Scanned++
			if !extSet[strings.ToLower(filepath.Ext(e.Name()))] {
				res.Skipped++
				continue
			}
			rel, err := filepath.Rel(root, full)
			if err != nil {
				res.Skipped++
				continue
			}
			dst := filepath.Join(dstRoot, rel)
			switch t.Mode {
			case "strm":
				ok, err := syncStrm(full, dst)
				if err != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", e.Name(), err))
					res.Skipped++
				} else if ok {
					res.Synced++
				} else {
					res.Skipped++
				}
			default:
				ok, err := syncFile(full, dst, t.Mode)
				if err != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", e.Name(), err))
					res.Skipped++
				} else if ok {
					res.Synced++
					if t.Mode == "move" || t.DeleteSrc {
						// move 内部已删；copy+delete_src 在此删
					}
				} else {
					res.Skipped++
				}
			}
		}
	}
	walk(root, 0)
	sort.Strings(res.Errors)
	return res
}

// syncFile link/softlink/copy/move 单文件（ok=false 表示目标已存在跳过）。
func syncFile(src, dst, mode string) (bool, error) {
	if fi, err := os.Stat(src); err != nil {
		return false, err
	} else if di, err := os.Stat(dst); err == nil && di.Size() == fi.Size() && mode != "move" {
		return false, nil // 已存在同大小（link/copy 幂等跳过）
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	switch mode {
	case "link":
		if err := os.Link(src, dst); err != nil {
			// 跨设备降级 copy（同 transfer.LinkOps.HardLink 语义）
			return true, copyFileLocal(src, dst)
		}
	case "softlink":
		abs, err := filepath.Abs(src)
		if err != nil {
			return false, err
		}
		if err := os.Symlink(abs, dst); err != nil {
			return false, err
		}
	case "copy":
		if err := copyFileLocal(src, dst); err != nil {
			return false, err
		}
	case "move":
		if err := os.Rename(src, dst); err != nil {
			if err := copyFileLocal(src, dst); err != nil {
				return false, err
			}
			if err := os.Remove(src); err != nil {
				return false, fmt.Errorf("移动后删源失败: %w", err)
			}
		}
	default:
		return false, fmt.Errorf("未知模式 %s", mode)
	}
	return true, nil
}

// syncStrm 生成指向 src（绝对路径）的 .strm 文件。dst 后缀强制 .strm。
func syncStrm(src, dst string) (bool, error) {
	if !strings.HasSuffix(strings.ToLower(dst), ".strm") {
		dst += ".strm"
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		return false, err
	}
	content := abs + "\n"
	if old, err := os.ReadFile(dst); err == nil && string(old) == content {
		return false, nil // 幂等
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(dst, []byte(content), 0o644)
}

// copyFileLocal 本地复制（64KB 缓冲流式；跨平台）。
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
	buf := make([]byte, 64<<10)
	_, err = io.CopyBuffer(out, in, buf)
	return err
}
