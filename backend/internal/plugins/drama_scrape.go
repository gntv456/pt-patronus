package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// drama-scrape / playlet-category 共享的视频扩展名与小工具。两个短剧插件从 Python
// external-http 移植为 Go builtin：编译期编入二进制，前端「已安装」天然出现，不再依赖市场源。
const defaultVideoExts = ".mp4,.mkv,.avi,.ts,.mov,.wmv,.flv,.m4v,.rmvb,.mpg,.mpeg,.webm"

const dramaUA = "PTPatronusPlugin/1.0 (drama-scrape)"

// parseVideoExts 把 "mp4,.mkv,avi" 之类解析为带点小写扩展名集合；空值用默认列表。
func parseVideoExts(value string) map[string]bool {
	if strings.TrimSpace(value) == "" {
		value = defaultVideoExts
	}
	m := map[string]bool{}
	for _, p := range strings.Split(value, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, ".") {
			p = "." + p
		}
		m[p] = true
	}
	return m
}

// listMediaFiles 递归遍历 folder，返回扩展名命中的文件绝对路径（对标 Python os.walk 过滤）。
func listMediaFiles(folder string, exts map[string]bool) []string {
	var out []string
	_ = filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if exts[strings.ToLower(filepath.Ext(d.Name()))] {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// copyFileData 复制单文件并保留权限位（link/move 跨卷失败时的 fallback）。
func copyFileData(src, dst string) error {
	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, si.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func matchesAny(keywords []string, s string) bool {
	for _, k := range keywords {
		if k != "" && strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// ---------------- 寸光集（ptang.top）client ----------------
// 与 internal/media/xingguangji.go 同源（/api/explore），但 drama-scrape 走寸光集 ptang.top
// 需要 Bearer API key 鉴权，且要保留 totalEpisodes/region/casts 全字段写进 NFO，故独立实现。

type xgItem struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	AltTitle      string  `json:"altTitle"`
	Cover         string  `json:"cover"`
	Description   string  `json:"description"`
	ReleaseYear   int     `json:"releaseYear"`
	Rating        float64 `json:"rating"`
	Tags          string  `json:"tags"`
	Region        string  `json:"region"`
	TotalEpisodes int     `json:"totalEpisodes"`
	Status        string  `json:"status"`
}

type xgCast struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Avatar string `json:"avatar"`
}

type xgDetailItem struct {
	xgItem
	Casts []xgCast `json:"casts"`
}

func resolveDramaURL(base, u string) string {
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if strings.HasPrefix(u, "/") {
		return strings.TrimRight(base, "/") + u
	}
	return u
}

// xgGet 发 GET 并解码到 out；硬 20s deadline（context.WithTimeout 取消 in-flight 请求）。
func xgGet(ctx context.Context, base, key, path string, params map[string]string, out any) error {
	u := strings.TrimRight(base, "/") + path
	if len(params) > 0 {
		v := url.Values{}
		for k, val := range params {
			v.Set(k, val)
		}
		u += "?" + v.Encode()
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", dramaUA)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cunguangji %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func xgSearch(ctx context.Context, base, key, name string) ([]xgItem, error) {
	var r struct {
		Items []xgItem `json:"items"`
	}
	if err := xgGet(ctx, base, key, "/api/explore", map[string]string{"q": name, "page": "1", "pageSize": "10"}, &r); err != nil {
		return nil, err
	}
	return r.Items, nil
}

func xgDetail(ctx context.Context, base, key, idStr string) (*xgDetailItem, error) {
	var d xgDetailItem
	if err := xgGet(ctx, base, key, "/api/explore/"+url.PathEscape(idStr), nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// ---------------- 名称清洗 + 匹配（逐字照 Python）----------------

var (
	dramaBracketRE = regexp.MustCompile(`【.*?】|\[.*?\]|\(.*?\)|（.*?）|「.*?」`)
	dramaNoiseRE   = regexp.MustCompile(`(?i)(S\d{1,2}E?\d{0,3}|[Ee][Pp]?\d{1,3}|\d{3,4}[pP]|[48][Kk]|HEVC|H\.?26[45]|h264|AVC|10bit|GB|fps|X264|[Xx]26[45]|AAC|FLAC|WEB-DL|BluRay|BDRip|mp4|mkv)`)
	dramaSepRE     = regexp.MustCompile(`[._]+`)
	dramaSpaceRE   = regexp.MustCompile(`\s+`)
)

func dramaCleanName(raw string) string {
	s := dramaBracketRE.ReplaceAllString(raw, " ")
	s = dramaNoiseRE.ReplaceAllString(s, " ")
	s = dramaSepRE.ReplaceAllString(s, " ")
	s = dramaSpaceRE.ReplaceAllString(s, " ")
	return strings.Trim(s, " -—")
}

// dramaPickMatch 评分选最佳：100 精确 / 50 包含 / 10 兜底；空结果返回 nil。
func dramaPickMatch(name string, items []xgItem) *xgItem {
	if len(items) == 0 {
		return nil
	}
	target := strings.ToLower(name)
	var best *xgItem
	bestScore := -1
	for i := range items {
		it := &items[i]
		title := strings.ToLower(it.Title)
		alt := strings.ToLower(it.AltTitle)
		score := -1
		switch {
		case target != "" && (target == title || target == alt):
			score = 100
		case target != "" && (strings.Contains(title, target) || strings.Contains(alt, target) || strings.Contains(target, title)):
			score = 50
		case title != "":
			score = 10
		}
		if score > bestScore {
			best = it
			bestScore = score
		}
	}
	return best
}

// ---------------- tvshow.nfo（1:1 复刻 Python ET 输出）----------------

func nfoEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func nfoLine(b *strings.Builder, tag, val string) {
	fmt.Fprintf(b, "  <%s>%s</%s>\n", tag, nfoEscape(val), tag)
}

func nfoSplitTags(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// writeTVShowNFO 写 Kodi/Jellyfin 兼容的 tvshow.nfo；字段与条件逐项对齐 Python write_tvshow_nfo。
func writeTVShowNFO(path string, d *xgDetailItem) error {
	var b strings.Builder
	b.WriteString("<?xml version='1.0' encoding='utf-8'?>\n<tvshow>\n")
	nfoLine(&b, "title", d.Title)
	if d.AltTitle != "" {
		nfoLine(&b, "originaltitle", d.AltTitle)
	}
	nfoLine(&b, "sorttitle", d.Title)
	if d.Description != "" {
		nfoLine(&b, "plot", d.Description)
	}
	if d.ReleaseYear != 0 {
		nfoLine(&b, "year", strconv.Itoa(d.ReleaseYear))
	}
	for _, g := range nfoSplitTags(d.Tags) {
		nfoLine(&b, "genre", g)
	}
	if d.Region != "" {
		nfoLine(&b, "country", d.Region)
	}
	nfoLine(&b, "studio", "寸光集")
	if d.Rating > 0 {
		b.WriteString("  <ratings>\n")
		b.WriteString(`    <rating name="cunguangji" max="10" default="true">` + "\n")
		fmt.Fprintf(&b, "      <value>%s</value>\n", strconv.FormatFloat(d.Rating, 'f', -1, 64))
		b.WriteString("      <votes>0</votes>\n")
		b.WriteString("    </rating>\n  </ratings>\n")
	}
	fmt.Fprintf(&b, `  <uniqueid type="cunguangji" default="true">%s</uniqueid>`+"\n", nfoEscape(strconv.Itoa(d.ID)))
	for i, c := range d.Casts {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		b.WriteString("  <actor>\n")
		fmt.Fprintf(&b, "    <name>%s</name>\n", nfoEscape(c.Name))
		if c.Role != "" {
			fmt.Fprintf(&b, "    <role>%s</role>\n", nfoEscape(c.Role))
		}
		fmt.Fprintf(&b, "    <order>%d</order>\n", i)
		b.WriteString("  </actor>\n")
	}
	b.WriteString("  <season>1</season>\n")
	if d.TotalEpisodes != 0 {
		nfoLine(&b, "episode", strconv.Itoa(d.TotalEpisodes))
	}
	b.WriteString("</tvshow>")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ---------------- 图片下载 + 文件整理 ----------------

func downloadDramaImage(ctx context.Context, imgURL, dest string) error {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", dramaUA)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("image status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

// transferDramaFile：link=硬链接(失败 fallback copy) / copy / move(Rename 失败 fallback copy+delete)。
func transferDramaFile(src, dst, mode string) error {
	switch mode {
	case "link":
		if err := os.Link(src, dst); err != nil {
			return copyFileData(src, dst)
		}
		return nil
	case "copy":
		return copyFileData(src, dst)
	case "move":
		if err := os.Rename(src, dst); err == nil {
			return nil
		}
		if err := copyFileData(src, dst); err != nil {
			return err
		}
		return os.Remove(src)
	}
	return nil
}

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
	errors                     []string
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
