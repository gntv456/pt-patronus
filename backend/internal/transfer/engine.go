package transfer

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/media"
)

// Recognizer 抽象媒体识别（chain.MediaChain 实现），避免 transfer→chain 循环。
type Recognizer interface {
	Search(ctx context.Context, keyword string, year int, filter string) ([]media.MediaInfo, error)
	Detail(ctx context.Context, source, id string) (*media.MediaInfo, error)
}

// MediaLink 是 DownloadMediaLink 的解耦投影（避免 transfer→models 依赖）。
type MediaLink struct {
	Source, SourceID, Title string
	Year                    int
	Type                    string
}

// LinkStore 抽象 DownloadMediaLinkOper.GetByHash（chain 适配）。
type LinkStore interface {
	GetByHash(hash string) (*MediaLink, error)
}

// FileInput 是一个待整理文件（绝对路径 + 大小）。
type FileInput struct {
	Path string
	Size int64
}

// ProcessInput 整理一个完成种子的入参（chain 解析下载器后填充）。
type ProcessInput struct {
	DownloaderID uint
	Hash         string
	Name         string // 种子名（无 DownloadMediaLink 时的搜索回退关键字之一）
	SavePath     string // 下载保存目录
	Category     string
	Files        []FileInput
	Manual       bool       // 手动整理（chain 已保证放行，引擎不重复过滤）
	MediaHint    *MediaLink // 推送时标签编码的媒体身份（Path A：秒级识别，免搜索）
}

// Result 是一次整理的结果（chain 据此写 TransferHistory + 通知）。
type Result struct {
	Success   bool
	ErrMsg    string
	Src       string // 源（首个视频）
	DestDir   string // 目标根目录（电影=片目录，剧集=剧根目录）
	Mode      Mode
	Files     []string // 已整理的目标文件完整路径
	NFO       []string // 已写的 NFO/图（相对 DestDir 或剧根）
	Media     *media.MediaInfo
	Type      string // movie | tv
	Category  string // 自动分类名（库子目录；空=未分类）
	Replaced  []string // 洗版被替换删除的旧文件（空=非洗版或首版）
	Skipped   bool   // 整理被跳过（如洗版判定画质未提升）——record 不写失败/不通知
	SkipReason string
	Seasons   string
	Episodes  string
}

// Engine 整理流水线引擎。所有依赖经接口/函数注入。
type Engine struct {
	recognize  Recognizer
	links      LinkStore
	kv         KVStore
	fetchImage ImageFetcher
	nfoWrite   NFOWriter
	linkOps    LinkOps
}

// NewEngine 构造。nfoWrite 为 nil 时用 media.NFO。
func NewEngine(rec Recognizer, links LinkStore, kv KVStore, fetch ImageFetcher, nfoWrite NFOWriter) *Engine {
	if nfoWrite == nil {
		nfoWrite = media.NFO
	}
	return &Engine{recognize: rec, links: links, kv: kv, fetchImage: fetch, nfoWrite: nfoWrite, linkOps: LinkOps{}}
}

// Process 执行整理：过滤视频 → 识别 → 重命名 → 硬链/移动 → 刮削 → 返回结果。
func (e *Engine) Process(ctx context.Context, in ProcessInput) *Result {
	cfg := Load(e.kv)
	res := &Result{Mode: cfg.Mode, Src: firstVideoPath(in.Files)}

	// 1. 过滤视频文件
	videos := filterVideos(in.Files)
	if len(videos) == 0 {
		res.ErrMsg = "no video files to organize"
		return res
	}

	// 2. 识别
	mi := e.recognizeMedia(ctx, in, videos)
	if mi == nil {
		res.ErrMsg = "media not recognized: " + searchKeyword(in, videos)
		return res
	}
	res.Media = mi

	// 2.5 自动分类（按媒体类型/国家/年份规则解析库子目录名）。
	category := ResolveCategory(LoadCategories(e.kv), mi)
	res.Category = category

	// 3. 按类型整理
	if mi.Type == media.TypeTV {
		e.organizeTV(ctx, cfg, in, mi, videos, category, res)
	} else {
		e.organizeMovie(ctx, cfg, in, mi, videos, category, res)
	}
	if res.ErrMsg != "" {
		return res
	}
	res.Success = true
	res.Type = string(mi.Type)
	return res
}

// recognizeMedia 优先按标签/DownloadMediaLink 秒级识别（Path A），否则按文件名搜索（Path B）。
func (e *Engine) recognizeMedia(ctx context.Context, in ProcessInput, videos []FileInput) *media.MediaInfo {
	// Path A：推送时标签编码的媒体身份（最可靠）。
	if in.MediaHint != nil && in.MediaHint.Source != "" {
		if mi, err := e.recognize.Detail(ctx, in.MediaHint.Source, in.MediaHint.SourceID); err == nil && mi != nil {
			return mi
		}
	}
	// Path A'：下载推送时落库的 DownloadMediaLink（兜底）。
	if e.links != nil && in.Hash != "" {
		if link, err := e.links.GetByHash(in.Hash); err == nil && link != nil && link.Source != "" {
			if mi, err := e.recognize.Detail(ctx, link.Source, link.SourceID); err == nil && mi != nil {
				return mi
			}
		}
	}
	// Path B：文件名解析 → 搜索 → Detail 富化。
	meta := ParseMeta(firstVideoName(videos))
	filter := "video"
	if meta.IsTV {
		filter = "tv"
	}
	results, err := e.recognize.Search(ctx, meta.Title, meta.Year, filter)
	if err != nil || len(results) == 0 {
		// 回退：用种子名再搜一次（文件名可能是乱码/英文片名）。
		results, err = e.recognize.Search(ctx, searchKeyword(in, videos), 0, filter)
		if err != nil || len(results) == 0 {
			return nil
		}
	}
	top := results[0]
	if mi, err := e.recognize.Detail(ctx, top.Source, top.SourceID); err == nil && mi != nil {
		return mi
	}
	return &top
}

// organizeMovie 整理电影：取最大视频文件 → 重命名 → 进库 → 写 movie.nfo + 海报。
func (e *Engine) organizeMovie(ctx context.Context, cfg Config, in ProcessInput, mi *media.MediaInfo, videos []FileInput, category string, res *Result) {
	main := pickLargest(videos)
	meta := ParseMeta(filepath.Base(main.Path))
	name := renderName(cfg.MovieRename, namingCtxFrom(mi, meta))
	ext := filepath.Ext(main.Path)
	destFile, destDir := destPathForMovie(cfg, cfg.MovieDir, category, name, ext)

	// 目标库目录已有版本时的处理：洗版升级 / 强制覆盖 / 跳过。
	if existing := firstVideoInDir(destDir); existing != "" {
		switch {
		case cfg.BestVersion && qualityScore(main.Path) > qualityScore(existing):
			// 新版画质更高 → 删旧链新（硬链模式仅去库链接，不毁做种源）。
			rem, err := removeMediaFile(existing)
			if err != nil {
				res.ErrMsg = "remove old version failed: " + err.Error()
				return
			}
			res.Replaced = []string{rem}
		case cfg.OverwriteFiles && existing == destFile:
			// 同名强制覆盖：先删再链。
			if _, err := removeMediaFile(existing); err != nil {
				res.ErrMsg = "remove existing failed: " + err.Error()
				return
			}
		case cfg.BestVersion:
			// 洗版但新版未更优 → 正常跳过（非失败）。
			res.Skipped = true
			res.SkipReason = "best_version: new not better than existing"
			return
		default:
			res.ErrMsg = "target exists: " + existing
			return
		}
	}
	if err := e.linkOps.Apply(cfg.Mode, main.Path, destFile); err != nil {
		res.ErrMsg = "link failed: " + err.Error()
		return
	}
	res.Files = []string{destFile}
	res.DestDir = destDir
	res.Src = main.Path
	nfo := writeScrape(ctx, destDir, "movie.nfo", mi, cfg.Scrape, e.nfoWrite, e.fetchImage)
	res.NFO = nfo
}

// organizeTV 整理剧集：每集重命名 SxxExx 进季目录；剧根写 tvshow.nfo + 海报（一次）。
func (e *Engine) organizeTV(ctx context.Context, cfg Config, in ProcessInput, mi *media.MediaInfo, videos []FileInput, category string, res *Result) {
	showFolder := showFolderName(mi.Title, mi.Year)
	dir := libraryDir(cfg, cfg.TVDir, "tv", category)
	showDir := filepath.Join(dir, showFolder)
	res.DestDir = showDir

	// 按文件名排序，保证集序稳定
	sorted := make([]FileInput, len(videos))
	copy(sorted, videos)
	sort.SliceStable(sorted, func(i, j int) bool {
		return filepath.Base(sorted[i].Path) < filepath.Base(sorted[j].Path)
	})

	var eps []string
	showMetaWritten := false
	for _, f := range sorted {
		meta := ParseMeta(filepath.Base(f.Path))
		name := renderName(cfg.TVRename, namingCtxFrom(mi, meta))
		ext := filepath.Ext(f.Path)
		seasonFolder := seasonFolderName(meta.Season)
		seasonDir := filepath.Join(showDir, seasonFolder)
		destFile := filepath.Join(seasonDir, name+ext)

		// 同集已有版本：洗版升级（删旧）/ 强制覆盖 / 跳过该集。
		if existing := findEpisodeInDir(seasonDir, meta.SeasonEpisode); existing != "" {
			switch {
			case cfg.BestVersion && qualityScore(f.Path) > qualityScore(existing):
				if rem, err := removeMediaFile(existing); err == nil {
					res.Replaced = append(res.Replaced, rem)
				} else {
					slog.Warn("transfer: remove old episode failed", "old", existing, "err", err)
					continue
				}
			case cfg.OverwriteFiles && existing == destFile:
				if _, err := removeMediaFile(existing); err != nil {
					slog.Warn("transfer: remove existing episode failed", "old", existing, "err", err)
					continue
				}
			case cfg.BestVersion:
				slog.Info("transfer: best_version episode not better, skip", "dst", destFile)
				continue
			default:
				slog.Warn("transfer: target episode exists, skip", "dst", destFile)
				continue
			}
		}
		if err := e.linkOps.Apply(cfg.Mode, f.Path, destFile); err != nil {
			slog.Warn("transfer: link episode failed", "src", f.Path, "err", err)
			continue
		}
		res.Files = append(res.Files, destFile)
		// 单集 NFO（与集文件同名 .nfo，<episodedetails>）
		epNfo := strings.TrimSuffix(destFile, filepath.Ext(destFile)) + ".nfo"
		if shouldWrite(epNfo, cfg.Scrape) {
			if err := os.WriteFile(epNfo, []byte(media.EpisodeNFO(mi, meta.Season, meta.Episode)), 0o644); err == nil {
				res.NFO = append(res.NFO, filepath.Base(epNfo))
			} else {
				slog.Debug("write episode nfo failed", "path", epNfo, "err", err)
			}
		}
		if meta.SeasonEpisode != "" {
			eps = append(eps, meta.SeasonEpisode)
		}
		if res.Src == "" {
			res.Src = f.Path
		}

		// 剧根 NFO + 海报只在第一个成功集后写一次（MISSING_ONLY 下幂等）。
		if !showMetaWritten {
			nfo := writeScrape(ctx, showDir, "tvshow.nfo", mi, cfg.Scrape, e.nfoWrite, e.fetchImage)
			res.NFO = append(res.NFO, nfo...)
			showMetaWritten = true
		}
	}
	if len(res.Files) == 0 {
		res.ErrMsg = "no episode organized (all targets exist or link failed)"
		return
	}
	if len(eps) > 0 {
		res.Episodes = joinNonEmpty(eps, " ")
		if s := seasonOf(eps); s != "" {
			res.Seasons = s
		}
	}
}

// namingCtxFrom 由识别结果（标题/年份）+ 文件 meta（季集/分辨率/组）合成命名上下文。
func namingCtxFrom(mi *media.MediaInfo, meta Meta) namingCtx {
	return namingCtx{
		Title: mi.Title, Year: mi.Year,
		Resolution: meta.Resolution, ReleaseGroup: meta.ReleaseGroup,
		Season: meta.Season, Episode: meta.Episode, SeasonEpisode: meta.SeasonEpisode,
	}
}

// filterVideos 筛视频文件、去样本。
func filterVideos(files []FileInput) []FileInput {
	var out []FileInput
	for _, f := range files {
		if !isVideoFile(f.Path) || isSample(f.Path) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// pickLargest 取体积最大的视频（电影主文件）。
func pickLargest(videos []FileInput) FileInput {
	best := videos[0]
	for _, f := range videos[1:] {
		if f.Size > best.Size {
			best = f
		}
	}
	return best
}

func firstVideoPath(files []FileInput) string {
	for _, f := range files {
		if isVideoFile(f.Path) {
			return f.Path
		}
	}
	if len(files) > 0 {
		return files[0].Path
	}
	return ""
}

// FirstVideoPath 返回首个视频文件路径（与 Result.Src 一致），供调用方在整理前去重。
func FirstVideoPath(files []FileInput) string { return firstVideoPath(files) }

func firstVideoName(videos []FileInput) string {
	if len(videos) > 0 {
		return filepath.Base(videos[0].Path)
	}
	return ""
}

// searchKeyword 无 link 时的搜索关键字：优先清洗后的种子名。
func searchKeyword(in ProcessInput, videos []FileInput) string {
	if in.Name != "" {
		return ParseMeta(in.Name).Title
	}
	if len(videos) > 0 {
		return ParseMeta(filepath.Base(videos[0].Path)).Title
	}
	return ""
}

// seasonOf 从 "S01E02" 集合里取首个季号展示 "S01"。
func seasonOf(eps []string) string {
	for _, e := range eps {
		if len(e) >= 3 && e[0] == 'S' {
			return e[:3]
		}
	}
	return ""
}

func joinNonEmpty(ss []string, sep string) string {
	var out []string
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return ""
	}
	if len(out) == 1 {
		return out[0]
	}
	return out[0] // 多集只记首个，避免历史字段过长
}
