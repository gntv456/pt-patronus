package dirwatch

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// ProcessFunc 处理一批发现的媒体文件（由 TransferChain.ProcessDiscoveredFiles 实现）。
type ProcessFunc func(ctx context.Context, srcDir string, files []transfer.FileInput)

// Watcher 监视一组目录，发现媒体文件即调 ProcessFunc。
type Watcher struct {
	cfg      Config
	process  ProcessFunc
	debounce time.Duration
	mu       sync.Mutex
	pending  map[string]*time.Timer // dir → debounce 定时器
	recent   map[string]time.Time   // 首文件路径 → 上次处理时间（TTL 去重）
}

// New 构造。debounce 取 cfg.DebounceMs，<=0 用 2s。
func New(cfg Config, process ProcessFunc) *Watcher {
	d := time.Duration(cfg.DebounceMs) * time.Millisecond
	if d <= 0 {
		d = 2 * time.Second
	}
	return &Watcher{cfg: cfg, process: process, debounce: d, pending: map[string]*time.Timer{}, recent: map[string]time.Time{}}
}

// Run 启动监视（阻塞）。compatibility 模式或指定轮询间隔时走轮询；否则 fsnotify。
func (w *Watcher) Run(ctx context.Context) {
	if w.cfg.MonitorMode == "compatibility" || w.cfg.PollIntervalSec > 0 {
		w.runPoll(ctx)
		return
	}
	w.runFSNotify(ctx)
}

// runFSNotify fsnotify 事件驱动 + debounce（默认路径）。
func (w *Watcher) runFSNotify(ctx context.Context) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("dirwatch: fsnotify 不可用，回退轮询", "err", err)
		w.runPoll(ctx)
		return
	}
	defer fw.Close()
	for _, d := range w.cfg.Dirs {
		if d.Path == "" {
			continue
		}
		if err := fw.Add(d.Path); err != nil {
			slog.Warn("dirwatch: 监视目录失败", "dir", d.Path, "err", err)
			continue
		}
		slog.Info("dirwatch: 监视中", "dir", d.Path, "mode", "fsnotify")
	}
	// 启动时各目录先扫一遍（catch-up 已存在的文件）。
	for _, d := range w.cfg.Dirs {
		if d.Path != "" {
			dir := d.Path
			go w.scanAndProcess(ctx, dir)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-fw.Events:
			if !ok {
				return
			}
			// Create/Write → 归并到所在目录 debounce。
			if ev.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				w.schedule(ctx, filepath.Dir(ev.Name))
			}
		case err, ok := <-fw.Errors:
			if !ok {
				return
			}
			slog.Warn("dirwatch: watcher error", "err", err)
		}
	}
}

// runPoll 定时全量扫描（compatibility / Windows flooding / 远程盘兜底）。
func (w *Watcher) runPoll(ctx context.Context) {
	interval := time.Duration(w.cfg.PollIntervalSec) * time.Second
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	var wg sync.WaitGroup
	for _, d := range w.cfg.Dirs {
		if d.Path == "" {
			continue
		}
		slog.Info("dirwatch: 监视中", "dir", d.Path, "mode", "poll", "interval", interval)
		wg.Add(1)
		safego.Go("watcher:107", func() {
			dir := d.Path
			defer wg.Done()
			t := time.NewTicker(interval)
			defer t.Stop()
			w.scanAndProcess(ctx, dir)
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					w.scanAndProcess(ctx, dir)
				}
			}
		})
	}
	<-ctx.Done()
	wg.Wait()
}

// schedule 同一目录的连续事件归并，静置 debounce 后扫一次（应对批量拷贝 flooding）。
func (w *Watcher) schedule(ctx context.Context, dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.pending[dir]; ok {
		t.Stop()
	}
	w.pending[dir] = time.AfterFunc(w.debounce, func() { w.scanAndProcess(ctx, dir) })
}

var tempExts = map[string]bool{
	".part": true, ".tmp": true, ".aria2": true, ".downloading": true,
	".!qb": true, ".!ut": true, ".crdownload": true,
}

func isTemp(p string) bool {
	lp := strings.ToLower(p)
	for ext := range tempExts {
		if strings.HasSuffix(lp, ext) {
			return true
		}
	}
	return false
}

// scanAndProcess 扫目录组媒体文件（跳临时扩展名）→ TTL 去重 → 调 process。
// 视频判定用 transfer.IsVideoPath（与可整理范围同一事实源，防清单漂移致恒失败）。
func (w *Watcher) scanAndProcess(ctx context.Context, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var files []transfer.FileInput
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if isTemp(p) || !transfer.IsVideoPath(p) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, transfer.FileInput{Path: p, Size: fi.Size()})
	}
	if len(files) == 0 {
		return
	}
	first := files[0].Path
	w.mu.Lock()
	// TTL 淘汰：recent 只增不减会随监视时间无限膨胀（每目录首文件路径常驻内存），
	// 顺手清掉已过期项（低频扫描路径，遍历代价可忽略）。
	now := time.Now()
	for k, ts := range w.recent {
		if now.Sub(ts) >= recentTTL {
			delete(w.recent, k)
		}
	}
	if t, ok := w.recent[first]; ok && now.Sub(t) < recentTTL {
		w.mu.Unlock()
		return // 短期内已处理
	}
	w.recent[first] = now
	w.mu.Unlock()
	w.process(ctx, dir, files)
}

// recentTTL 同一首文件的处理去重窗口。
const recentTTL = 10 * time.Second
