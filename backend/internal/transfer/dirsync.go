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
	"time"
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
	// IntervalMinutes 定时执行间隔（分钟）；0=仅手动触发，>=5 才参与 cron 调度。
	IntervalMinutes int   `json:"interval_minutes,omitempty"`
	LastRunAt       int64 `json:"last_run_at,omitempty"` // 上次定时执行的 unix 秒（手动触发不回写）
}

// DirSyncResult 单次执行汇总。
type DirSyncResult struct {
	Scanned int      `json:"scanned"`
	Synced  int      `json:"synced"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

// DirSyncRun 一次定时 tick 中某任务的执行记录（供日志/通知）。
type DirSyncRun struct {
	Name   string
	Result *DirSyncResult // nil = 因失联前置条件未执行
	Err    error
}

const dirSyncKey = "transfer.dir_sync"

// LoadDirSyncTasks 读任务列表。err 非 nil = 配置读不出来或已损坏——调用方必须
// 与「空清单」区分渲染（此前两者都静默变成「没有任务」：cron 整轮不跑、界面空列表）。
func LoadDirSyncTasks(kv KVStore) ([]DirSyncTask, error) {
	if kv == nil {
		return nil, nil
	}
	raw, err := kv.Get(dirSyncKey)
	if err != nil {
		return nil, fmt.Errorf("读取目录同步任务失败: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var ts []DirSyncTask
	if err := json.Unmarshal([]byte(raw), &ts); err != nil {
		return nil, fmt.Errorf("目录同步任务配置损坏: %w", err)
	}
	return ts, nil
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

// UpdateDirSyncLastRun 仅回写单任务的 LastRunAt（四轮 P2 修复：此前 cron tick 整表
// 读-改-写回 Save，tick 执行期间（任务可跑数分钟）用户经 API 增删/编辑任务会被
// 收尾的整表 Save 用读时快照覆盖，静默丢编辑）。按 name 定位，任务被用户删除则跳过。
func UpdateDirSyncLastRun(kv KVStore, name string, lastRunAt int64) error {
	tasks, err := LoadDirSyncTasks(kv)
	if err != nil {
		return err
	}
	for i := range tasks {
		if tasks[i].Name == name {
			tasks[i].LastRunAt = lastRunAt
			return SaveDirSyncTasks(kv, tasks)
		}
	}
	return nil
}

// minDirSyncInterval 定时执行的最小间隔（分钟）：过低会高频扫盘/硬链，失去定时意义。
const minDirSyncInterval = 5

// RunDueDirSyncs 定时 tick：执行所有「启用 + interval≥5 + 已到期」的任务并回写 LastRunAt。
// 由 scheduler 的 SkipIfStillRunning 防重叠；单个任务失败不阻断其它任务。
// 返回每个任务的执行记录（跳过的不在返回值里）；err 非 nil = 清单读不出来，
// 本轮整轮跳过——调用方必须与「没有到期任务」区分（跑没跑与没事可做不能同形）。
func RunDueDirSyncs(kv KVStore, now time.Time) ([]DirSyncRun, error) {
	tasks, err := LoadDirSyncTasks(kv)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, nil
	}
	nowUnix := now.Unix()
	var runs []DirSyncRun
	for i := range tasks {
		t := tasks[i]
		if !t.Enabled || t.IntervalMinutes < minDirSyncInterval {
			continue
		}
		if t.LastRunAt > 0 && nowUnix-t.LastRunAt < int64(t.IntervalMinutes)*60 {
			continue // 未到期
		}
		run := DirSyncRun{Name: t.Name}
		func() {
			defer func() {
				if r := recover(); r != nil {
					run.Err = fmt.Errorf("panic: %v", r)
				}
			}()
			run.Result = RunDirSync(t)
		}()
		// 四轮 P2 修复：单任务回写 LastRunAt，不再整表 Save（防覆盖用户并发编辑）。
		if err := UpdateDirSyncLastRun(kv, t.Name, nowUnix); err != nil {
			run.Err = fmt.Errorf("回写 last_run_at 失败: %w", err)
		}
		runs = append(runs, run)
	}
	return runs, nil
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
					// move 模式源文件已在 MoveTo 内删；copy+delete_src 的删除由 MoveTo 调用方处理
					res.Synced++
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
	// 四轮 P2 修复：tmp+rename 原子写（直写崩溃/与 STRM 生成链路并发写同一文件会留半行）。
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return true, nil
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
