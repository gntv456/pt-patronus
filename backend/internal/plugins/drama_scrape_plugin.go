package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------- 插件主体 ----------------

type dramaScrapePlugin struct {
	mu      sync.Mutex
	running bool
	status  map[string]any
}

func NewDramaScrapePlugin() Plugin {
	return &dramaScrapePlugin{status: dramaFreshStatus()}
}

func dramaFreshStatus() map[string]any {
	return map[string]any{"running": false, "done": false, "ok": 0, "miss": 0, "exists": 0, "skipped": 0, "errors": 0, "error": nil}
}

func dramaBase(cfg map[string]any) string {
	if b := strings.TrimSpace(pluginString(cfg["cunguangji_base"])); b != "" {
		return b
	}
	return "https://ptang.top"
}

func (p *dramaScrapePlugin) Manifest() Manifest {
	return Manifest{
		ID:          "ptp-drama-scrape",
		Name:        "短剧刮削",
		Version:     "1.0.0",
		APIVersion:  "1",
		AppVersion:  ">=0.1.0",
		Author:      "PTPatronus Community",
		Description: "【短剧专用·寸光集源】监控/扫描短剧目录，按目录名到寸光集（ptang.top）检索元数据与封面，写入 Kodi 兼容的 tvshow.nfo 和 poster.jpg；可选把视频整理到目标目录。",
		License:     "MIT",
		Keywords:    []string{"media", "drama", "scrape", "nfo", "cunguangji", "duanju", "playlet"},
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "log:write", "notice:write", "kv:read", "kv:write"},
		Actions: []ActionSpec{
			{Name: "scan", Label: "扫描刮削短剧", Description: "后台扫描源目录，为每个短剧目录写入 tvshow.nfo 与 poster.jpg。立即返回，进度用 status 查询。"},
			{Name: "status", Label: "查询扫描状态", Description: "返回后台扫描的运行状态与计数。"},
			{Name: "scrape_one", Label: "刮削单个", Description: "input.name 指定剧名，返回寸光集检索结果（不写盘）。", InputSchema: []ConfigField{{Key: "name", Label: "剧名", Type: "text", Required: true}}},
		},
		ConfigSchema: []ConfigField{
			{Key: "source_dirs", Label: "源目录", Type: "textarea", Group: "扫描", Help: "每行一个目录，对其下每个含视频的子目录按目录名刮削。"},
			{Key: "target_dir", Label: "目标目录（可选）", Type: "text", Group: "扫描", Help: "设置后按 transfer_type 把视频+元数据整理到该目录下。"},
			{Key: "transfer_type", Label: "整理方式", Type: "select", Group: "扫描", Default: "none", Options: []ConfigOption{{Label: "不整理", Value: "none"}, {Label: "硬链接", Value: "link"}, {Label: "复制", Value: "copy"}, {Label: "移动", Value: "move"}}},
			{Key: "overwrite", Label: "覆盖已有元数据", Type: "switch", Group: "扫描", Default: false},
			{Key: "exclude_keywords", Label: "排除关键字", Type: "textarea", Group: "扫描", Help: "目录名含任一关键字则跳过，每行一个。"},
			{Key: "media_ext", Label: "视频扩展名", Type: "text", Group: "扫描", Default: defaultVideoExts},
			{Key: "cunguangji_base", Label: "寸光集地址", Type: "text", Group: "刮削源", Default: "https://ptang.top"},
			{Key: "cunguangji_key", Label: "寸光集 API Key", Type: "password", Group: "刮削源", Required: true, Help: "在寸光集（ptang.top）获取你自己的 API Key 后填入；插件不预置任何 Key。"},
			{Key: "cron", Label: "定时表达式", Type: "text", Group: "调度", Default: "", Help: "留空则不自动扫描，可手动触发。示例：*/30 * * * *。"},
		},
		Schedule: []ScheduleSpec{
			{ID: "scan", Label: "定时刮削短剧", Action: "scan", Cron: "", CronConfigKey: "cron"},
		},
		Contributes: &ContributionSpec{
			Commands: []CommandContribution{{ID: "ptp-drama-scrape.scan", Label: "短剧刮削：立即扫描", Action: "scan"}},
			Menus:    []MenuContribution{{Location: "plugins/actions", Command: "ptp-drama-scrape.scan"}},
		},
		ResourceAccess: &ResourceAccessSpec{
			NetHosts: []string{"ptang.top（寸光集 API）"},
			FsPaths:  []string{"用户配置的短剧监控/目标目录"},
		},
	}
}

func (p *dramaScrapePlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"source_dirs":      "",
		"target_dir":       "",
		"transfer_type":    "none",
		"overwrite":        false,
		"exclude_keywords": "",
		"media_ext":        defaultVideoExts,
		"cunguangji_base":  "https://ptang.top",
		"cunguangji_key":   "",
		"cron":             "",
	}
}

func (p *dramaScrapePlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (p *dramaScrapePlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "scrape_one":
		cfg := rt.Config()
		base := dramaBase(cfg)
		key := strings.TrimSpace(pluginString(cfg["cunguangji_key"]))
		name := dramaCleanName(pluginString(input["name"]))
		if name == "" {
			return nil, fmt.Errorf("input.name is required")
		}
		items, err := xgSearch(ctx, base, key, name)
		if err != nil {
			return nil, err
		}
		results := make([]map[string]any, 0, len(items))
		for _, it := range items {
			if len(results) >= 10 {
				break
			}
			results = append(results, map[string]any{
				"id":       it.ID,
				"title":    it.Title,
				"altTitle": it.AltTitle,
				"year":     it.ReleaseYear,
				"rating":   it.Rating,
				"cover":    resolveDramaURL(base, it.Cover),
			})
		}
		return map[string]any{"name": name, "results": results}, nil
	case "scan":
		return p.startScan(rt), nil
	case "status":
		return p.snapshotStatus(), nil
	default:
		return nil, fmt.Errorf("unknown action %s", action)
	}
}

// ---------------- 异步 worker ----------------

func (p *dramaScrapePlugin) snapshotStatus() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.copyStatusLocked()
}

func (p *dramaScrapePlugin) copyStatusLocked() map[string]any {
	out := make(map[string]any, len(p.status))
	for k, v := range p.status {
		out[k] = v
	}
	return out
}

func (p *dramaScrapePlugin) setCounts(ok, miss, exists, skipped, errCount int) {
	p.mu.Lock()
	p.status["ok"] = ok
	p.status["miss"] = miss
	p.status["exists"] = exists
	p.status["skipped"] = skipped
	p.status["errors"] = errCount
	p.mu.Unlock()
}

func (p *dramaScrapePlugin) startScan(rt Runtime) map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return map[string]any{"started": false, "already_running": true, "status": p.copyStatusLocked()}
	}
	p.status = map[string]any{
		"running":    true,
		"done":       false,
		"started_at": time.Now().Format("2006-01-02 15:04:05"),
		"ok":         0, "miss": 0, "exists": 0, "skipped": 0, "errors": 0, "error": nil,
	}
	p.running = true
	cfg := rt.Config()
	go p.scanWorker(rt, cfg)
	return map[string]any{"started": true, "status": p.copyStatusLocked()}
}

type dramaScanResult struct {
	ok, miss, exists, skipped int
	errors                    []string
}

func (p *dramaScrapePlugin) scanWorker(rt Runtime, cfg map[string]any) {
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
		rt.Log(logError, "drama.scan.fatal", err.Error(), nil)
		_ = rt.Notice(ctx, NoticeInput{Title: "短剧刮削", Body: "扫描失败：" + err.Error(), Level: "warning"})
		return
	}
	p.mu.Lock()
	p.status["running"] = false
	p.status["done"] = true
	p.status["finished_at"] = time.Now().Format("2006-01-02 15:04:05")
	p.mu.Unlock()
	_ = rt.Notice(ctx, NoticeInput{
		Title: "短剧刮削",
		Body:  fmt.Sprintf("完成：成功 %d，未匹配 %d，已存在 %d，失败 %d。", res.ok, res.miss, res.exists, len(res.errors)),
		Level: "info",
	})
}

func (p *dramaScrapePlugin) doScan(ctx context.Context, rt Runtime, cfg map[string]any) (dramaScanResult, error) {
	base := dramaBase(cfg)
	key := strings.TrimSpace(pluginString(cfg["cunguangji_key"]))
	targetDir := strings.TrimSpace(pluginString(cfg["target_dir"]))
	mode := pluginString(cfg["transfer_type"])
	if mode == "" {
		mode = "none"
	}
	overwrite := asBool(cfg["overwrite"])
	exts := parseVideoExts(pluginString(cfg["media_ext"]))
	roots := splitLines(pluginString(cfg["source_dirs"]))
	excludes := splitLines(pluginString(cfg["exclude_keywords"]))
	if len(roots) == 0 {
		return dramaScanResult{}, fmt.Errorf("source_dirs 未配置")
	}
	var res dramaScanResult
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			res.errors = append(res.errors, "not a dir: "+root)
			p.setCounts(res.ok, res.miss, res.exists, res.skipped, len(res.errors))
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			res.errors = append(res.errors, "read dir "+root+": "+err.Error())
			p.setCounts(res.ok, res.miss, res.exists, res.skipped, len(res.errors))
			continue
		}
		for _, e := range entries { // os.ReadDir 已按文件名排序
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			child := filepath.Join(root, name)
			if matchesAny(excludes, name) {
				res.skipped++
				p.setCounts(res.ok, res.miss, res.exists, res.skipped, len(res.errors))
				continue
			}
			if len(listMediaFiles(child, exts)) == 0 {
				continue
			}
			r, err := p.scrapeFolder(ctx, child, base, key, targetDir, mode, overwrite, exts, rt)
			if err != nil {
				res.errors = append(res.errors, fmt.Sprintf("%s: %s", name, err.Error()))
				rt.Log(logError, "drama.scrape.error", err.Error(), map[string]any{"dir": name})
				p.setCounts(res.ok, res.miss, res.exists, res.skipped, len(res.errors))
				continue
			}
			switch r["status"] {
			case "ok":
				res.ok++
			case "miss":
				res.miss++
			case "exists":
				res.exists++
			default:
				res.skipped++
			}
			p.setCounts(res.ok, res.miss, res.exists, res.skipped, len(res.errors))
		}
	}
	return res, nil
}

func (p *dramaScrapePlugin) scrapeFolder(ctx context.Context, folder, base, key, targetDir, mode string, overwrite bool, exts map[string]bool, rt Runtime) (map[string]any, error) {
	name := dramaCleanName(filepath.Base(folder))
	if name == "" {
		return map[string]any{"status": "skip", "reason": "empty name"}, nil
	}
	items, err := xgSearch(ctx, base, key, name)
	if err != nil {
		return nil, err
	}
	match := dramaPickMatch(name, items)
	if match == nil {
		return map[string]any{"status": "miss", "name": name}, nil
	}
	var d xgDetailItem
	if detail, derr := xgDetail(ctx, base, key, strconv.Itoa(match.ID)); derr == nil && detail != nil && detail.Title != "" {
		d = *detail
	} else {
		d = xgDetailItem{xgItem: *match} // fallback：详情失败时用搜索结果（无 casts）
	}

	dest := folder
	if targetDir != "" {
		dest = filepath.Join(targetDir, name)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, err
	}
	nfoPath := filepath.Join(dest, "tvshow.nfo")
	if _, statErr := os.Stat(nfoPath); statErr == nil && !overwrite && targetDir == "" {
		return map[string]any{"status": "exists", "name": name}, nil
	}
	if err := writeTVShowNFO(nfoPath, &d); err != nil {
		return nil, err
	}
	result := map[string]any{"status": "ok", "name": name, "title": d.Title, "id": d.ID, "nfo": nfoPath, "poster": nil}
	cover := resolveDramaURL(base, d.Cover)
	if cover == "" {
		cover = resolveDramaURL(base, match.Cover)
	}
	if cover != "" {
		posterPath := filepath.Join(dest, "poster.jpg")
		if err := downloadDramaImage(ctx, cover, posterPath); err == nil {
			result["poster"] = posterPath
		} else {
			rt.Log(logWarning, "drama.poster.error", err.Error(), map[string]any{"name": name, "cover": cover})
		}
	}
	if targetDir != "" && mode != "none" {
		for _, video := range listMediaFiles(folder, exts) {
			if err := transferDramaFile(video, filepath.Join(dest, filepath.Base(video)), mode); err != nil {
				rt.Log(logWarning, "drama.transfer.error", err.Error(), map[string]any{"file": video})
			}
		}
	}
	return result, nil
}
