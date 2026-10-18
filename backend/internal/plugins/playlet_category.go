package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// playlet-category：ffprobe 探测子目录里若干集视频时长，全低于阈值则判为短剧，
// 整个子目录移动到「短剧」分类目录下。从 Python external-http 移植为 Go builtin。

// probeDuration 调 ffprobe 取容器时长（秒）；returncode≠0 或输出不可解析视为该视频探测失败。
func probeDuration(ctx context.Context, video string) (float64, error) {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		video)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, ffprobeError(err, stderr.String())
	}
	s := strings.TrimSpace(stdout.String())
	if s == "" {
		return 0, nil // 与 Python float("" or 0.0)=0.0 对齐
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe unparseable: %q", s)
	}
	return f, nil
}

// sampleEvenly 从 n 个文件里均匀间隔取 k 个（n<=k 时全取）。Python 用 random.sample，
// 这里改确定性抽样：更可测、可复现，对短剧识别无实质影响。
func sampleEvenly(files []string, k int) []string {
	n := len(files)
	if n <= k {
		return files
	}
	out := make([]string, 0, k)
	step := float64(n) / float64(k)
	for i := 0; i < k; i++ {
		idx := int(float64(i) * step)
		if idx >= n {
			idx = n - 1
		}
		out = append(out, files[idx])
	}
	return out
}

// isShortDrama 返回 *bool：nil=非视频目录(空)，false=非短剧，true=短剧。
// 全部抽样探测失败视为非短剧（ffprobe 缺失时不会误判为短剧）。
func isShortDrama(ctx context.Context, folder string, exts map[string]bool, sampleCount, maxSeconds int, rt Runtime) *bool {
	media := listMediaFiles(folder, exts)
	if len(media) == 0 {
		return nil
	}
	var durations []float64
	for _, video := range sampleEvenly(media, sampleCount) {
		d, err := probeDuration(ctx, video)
		if err != nil {
			rt.Log(logWarning, "playlet.probe.error", err.Error(), map[string]any{"file": video})
			continue
		}
		durations = append(durations, d)
	}
	if len(durations) == 0 {
		f := false
		return &f
	}
	short := true
	for _, d := range durations {
		if d > float64(maxSeconds) {
			short = false
			break
		}
	}
	return &short
}

// moveDir 整目录移动：同卷 os.Rename 原子；跨卷失败 fallback copyDir + RemoveAll。
func moveDir(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyDir(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

type playletCategoryPlugin struct {
	mu      sync.Mutex
	running bool
	status  map[string]any
}

func NewPlayletCategoryPlugin() Plugin {
	return &playletCategoryPlugin{status: playletFreshStatus()}
}

func playletFreshStatus() map[string]any {
	return map[string]any{"running": false, "done": false, "scanned": 0, "moved": 0, "errors": 0, "error": nil}
}

func (p *playletCategoryPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "ptp-playlet-category",
		Name:        "短剧自动分类",
		Version:     "1.0.0",
		APIVersion:  "1",
		AppVersion:  ">=0.1.0",
		Author:      "PTPatronus Community",
		Description: "定时扫描源目录，用 ffprobe 探测每个子目录里若干集视频时长；若均小于阈值（默认 8 分钟）则判定为短剧，把整个子目录移动到「短剧」分类目录下。需要宿主机已安装 ffprobe。",
		License:     "MIT",
		Keywords:    []string{"media", "drama", "category", "ffprobe"},
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"log:write", "notice:write"},
		Actions: []ActionSpec{
			{Name: "scan", Label: "扫描并分类短剧", Description: "后台扫描源目录，把短剧子目录移动到分类目录。立即返回，进度用 status 查询。"},
			{Name: "status", Label: "查询扫描状态", Description: "返回后台扫描的运行状态与计数。"},
		},
		ConfigSchema: []ConfigField{
			{Key: "source_path", Label: "源目录", Type: "text", Group: "扫描", Help: "对其下每个子目录做短剧判定。"},
			{Key: "category_dir", Label: "分类目录名", Type: "text", Group: "扫描", Default: "短剧"},
			{Key: "episode_duration", Label: "单集最长（分钟）", Type: "text", Group: "扫描", Default: "8", Help: "小于等于该时长才视为短剧。"},
			{Key: "sample_count", Label: "抽样集数", Type: "number", Group: "扫描", Default: 3, Help: "每个子目录最多探测的视频数量。"},
			{Key: "media_ext", Label: "视频扩展名", Type: "text", Group: "扫描", Default: defaultVideoExts},
			{Key: "cron", Label: "定时表达式", Type: "text", Group: "调度", Default: "", Help: "留空则不自动扫描，可手动触发。示例：0 4 * * *。"},
		},
		Schedule: []ScheduleSpec{
			{ID: "scan", Label: "定时分类短剧", Action: "scan", Cron: "", CronConfigKey: "cron"},
		},
		Contributes: &ContributionSpec{
			Commands: []CommandContribution{{ID: "ptp-playlet-category.scan", Label: "短剧自动分类：立即扫描", Action: "scan"}},
			Menus:    []MenuContribution{{Location: "plugins/actions", Command: "ptp-playlet-category.scan"}},
		},
		ResourceAccess: &ResourceAccessSpec{
			ExecBinaries: []string{"ffprobe"},
			FsPaths:      []string{"用户配置的源目录（读取并移动短剧子目录）"},
		},
	}
}

func (p *playletCategoryPlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"source_path":      "",
		"category_dir":     "短剧",
		"episode_duration": "8",
		"sample_count":     3,
		"media_ext":        defaultVideoExts,
		"cron":             "",
	}
}

func (p *playletCategoryPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (p *playletCategoryPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "scan":
		return p.startScan(rt), nil
	case "status":
		return p.snapshotStatus(), nil
	default:
		return nil, unknownActionErr(action)
	}
}

// ---------------- 异步 worker ----------------

func (p *playletCategoryPlugin) snapshotStatus() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.copyStatusLocked()
}

func (p *playletCategoryPlugin) copyStatusLocked() map[string]any {
	out := make(map[string]any, len(p.status))
	for k, v := range p.status {
		out[k] = v
	}
	return out
}

func (p *playletCategoryPlugin) setStatus(scanned, moved, errCount int) {
	p.mu.Lock()
	p.status["scanned"] = scanned
	p.status["moved"] = moved
	p.status["errors"] = errCount
	p.mu.Unlock()
}

func (p *playletCategoryPlugin) startScan(rt Runtime) map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return map[string]any{"started": false, "already_running": true, "status": p.copyStatusLocked()}
	}
	p.status = map[string]any{
		"running":    true,
		"done":       false,
		"started_at": time.Now().Format("2006-01-02 15:04:05"),
		"scanned":    0, "moved": 0, "errors": 0, "error": nil,
	}
	p.running = true
	cfg := rt.Config()
	go p.scanWorker(rt, cfg)
	return map[string]any{"started": true, "status": p.copyStatusLocked()}
}

type playletScanResult struct {
	scanned, moved, nonShort, skipped int
	errors                            []string
}

func (p *playletCategoryPlugin) scanWorker(rt Runtime, cfg map[string]any) {
	defer func() {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
	}()
	ctx := context.Background()
	res, err := p.doScan(ctx, rt, cfg)
	if err != nil {
		p.mu.Lock()
		p.status["running"] = false
		p.status["done"] = true
		p.status["error"] = err.Error()
		p.mu.Unlock()
		rt.Log(logError, "playlet.scan.fatal", err.Error(), nil)
		_ = rt.Notice(ctx, NoticeInput{Title: "短剧自动分类", Body: "扫描失败：" + err.Error(), Level: "warning"})
		return
	}
	p.mu.Lock()
	p.status["running"] = false
	p.status["done"] = true
	p.status["finished_at"] = time.Now().Format("2006-01-02 15:04:05")
	p.mu.Unlock()
	_ = rt.Notice(ctx, NoticeInput{
		Title: "短剧自动分类",
		Body:  fmt.Sprintf("扫描 %d 个目录，移动 %d 个短剧目录。", res.scanned, res.moved),
		Level: "info",
	})
}

func (p *playletCategoryPlugin) doScan(ctx context.Context, rt Runtime, cfg map[string]any) (playletScanResult, error) {
	source := strings.TrimSpace(pluginString(cfg["source_path"]))
	categoryDir := strings.TrimSpace(pluginString(cfg["category_dir"]))
	if categoryDir == "" {
		categoryDir = "短剧"
	}
	maxMinutes := pluginInt(cfg["episode_duration"], 8)
	if maxMinutes <= 0 {
		maxMinutes = 8
	}
	sampleCount := pluginInt(cfg["sample_count"], 3)
	if sampleCount < 1 {
		sampleCount = 1
	}
	exts := parseVideoExts(pluginString(cfg["media_ext"]))
	if source == "" {
		return playletScanResult{}, fmt.Errorf("source_path 未配置")
	}
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		return playletScanResult{}, fmt.Errorf("source_path 未配置或不存在")
	}
	destRoot := filepath.Join(source, categoryDir)
	var res playletScanResult
	entries, err := os.ReadDir(source)
	if err != nil {
		return res, err
	}
	for _, e := range entries { // os.ReadDir 已按文件名排序
		if !e.IsDir() || e.Name() == categoryDir {
			continue
		}
		name := e.Name()
		child := filepath.Join(source, name)
		res.scanned++
		verdict := isShortDrama(ctx, child, exts, sampleCount, maxMinutes*60, rt)
		switch {
		case verdict == nil:
			res.skipped++
		case !*verdict:
			res.nonShort++
		default:
			target := filepath.Join(destRoot, name)
			if err := os.MkdirAll(destRoot, 0o755); err != nil {
				res.errors = append(res.errors, fmt.Sprintf("%s: %s", name, err.Error()))
				p.setStatus(res.scanned, res.moved, len(res.errors))
				continue
			}
			if _, statErr := os.Stat(target); statErr == nil {
				res.errors = append(res.errors, fmt.Sprintf("%s: 目标已存在 %s", name, target))
				p.setStatus(res.scanned, res.moved, len(res.errors))
				continue
			}
			if err := moveDir(child, target); err != nil {
				res.errors = append(res.errors, fmt.Sprintf("%s: %s", name, err.Error()))
				p.setStatus(res.scanned, res.moved, len(res.errors))
				continue
			}
			res.moved++
			rt.Log(logInfo, "playlet.moved", name, map[string]any{"from": child, "to": target})
		}
		p.setStatus(res.scanned, res.moved, len(res.errors))
	}
	return res, nil
}

// ffprobeError 分清两种「拿不到时长」：宿主机根本没装 ffmpeg（可操作：装或停用插件），
// 与这一次探测失败（原因在 stderr——`ffprobe -v error` 的诊断信息全走 stderr，
// 从前只留 exit status 1，用户与日志都不知道是文件不存在、格式不支持还是超时被杀）。
func ffprobeError(err error, stderr string) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("未找到 ffprobe：该插件需要宿主机安装 ffmpeg，请安装后重试，或在插件页停用「短剧分类」")
	}
	if msg := strings.TrimSpace(stderr); msg != "" {
		return fmt.Errorf("ffprobe: %v: %s", err, msg)
	}
	return fmt.Errorf("ffprobe: %v", err)
}
