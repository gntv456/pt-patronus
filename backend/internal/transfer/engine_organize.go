package transfer

// 整理执行核心：电影/剧集的识别→硬链/移动→命名落位。从 engine.go 迁出。

import (
	"context"
	"os"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/media"
)

// organizeMovie 整理电影：取最大视频文件 → 重命名 → 进库 → 写 movie.nfo + 海报。
func (e *Engine) organizeMovie(ctx context.Context, cfg Config, in ProcessInput, mi *media.MediaInfo, videos []FileInput, category string, res *Result) {
	main := pickLargest(videos)
	meta := ParseMeta(filepath.Base(main.Path))
	name := renderName(cfg.MovieRename, namingCtxFrom(mi, meta))
	ext := filepath.Ext(main.Path)
	// 电影目录名仅 Title (Year)（与剧集 showFolderName 一致）：1080P/2160P 同片归同目录，洗版可识别旧版。
	movieFolder := showFolderName(mi.Title, mi.Year)
	destFile, destDir := destPathForMovie(cfg, cfg.MovieDir, category, movieFolder, name, ext)

	// 目标库目录已有版本时的处理：洗版升级 / 强制覆盖 / 跳过。
	if existing := firstVideoInDir(destDir); existing != "" {
		switch {
		case cfg.BestVersion && qualityScore(main.Path) > qualityScore(existing):
			// 新版画质更高 → 先链新到临时名→校验成功→删旧→重命名为最终名。
			// 保证硬链失败时旧库文件不丢，洗版可重试。
			if err := replaceViaTemp(cfg.Mode, main.Path, destFile, existing, &res.Replaced); err != nil {
				res.ErrMsg = "best_version replace failed: " + err.Error()
				return
			}
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
	var missingEps []string // 单集 link 失败时累计集号，ErrMsg 反映缺集
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
			// 单集 link 失败累计到 missingEps：ErrMsg 非空 → Success=false，通知/历史反映缺集。
			if se := meta.SeasonEpisode; se != "" {
				missingEps = append(missingEps, se)
			}
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
	// 单集 link 失败时让 ErrMsg 非空（Success=false），通知/历史反映缺集；已成功的集仍在 res.Files。
	if len(missingEps) > 0 {
		res.ErrMsg = "缺失 " + strings.Join(missingEps, ",")
	}
	if len(eps) > 0 {
		res.Episodes = joinNonEmpty(eps, " ")
		if s := seasonOf(eps); s != "" {
			res.Seasons = s
		}
	}
}

// namingCtxFrom 由识别结果（标题/年份）+ 文件 meta（季集/分辨率/组）合成命名上下文。
