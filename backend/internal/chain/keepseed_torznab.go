package chain

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/keepseed"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/torznab"
)

// reportTorznabFailure 上报 Torznab 搜索失败：slog.Warn + 站内通知（按天去重防刷屏）。
// 此前仅 slog.Debug：Jackett/Prowlarr 端点坏掉（API Key 错/服务未起）时搜索永远返回空，
// 用户只看到「没有候选」，无从发现 Torznab 源已失效。
func (c *KeepSeedChain) reportTorznabFailure(ctx context.Context, err error) {
	slog.Warn("torznab search failed", "err", err)
	if c.notice == nil {
		return
	}
	_ = c.notice.Notice(ctx, notice.Input{
		Source: notice.SourceReseedFail,
		Key:    "torznab",
		Title:  "辅种 Torznab 搜索失败",
		Body:   "已配置的 Torznab（Jackett/Prowlarr）端点搜索失败：" + err.Error() + "；辅种候选将缺失，请检查端点地址与 API Key",
		Level:  "warning",
	}, true)
}

// mediaIDHints 合并标题提取 + DownloadMediaLink（按 infohash）。
func (c *KeepSeedChain) mediaIDHints(infoHash, name string) keepseed.IDHints {
	ids := keepseed.IDHints{
		ImdbID: torznab.ExtractImdbID(name),
		TmdbID: keepseed.ExtractTmdbID(name),
	}
	if c.db == nil || infoHash == "" {
		return ids
	}
	var link models.DownloadMediaLink
	// hash 大小写不敏感：下载器可能返回大小写混合
	err := c.db.Where("LOWER(download_hash) = ?", strings.ToLower(infoHash)).First(&link).Error
	if err != nil {
		return ids
	}
	if link.Source == "tmdb" && link.SourceID != "" {
		ids.TmdbID = link.SourceID
	}
	return ids
}

// listTorrentFileNames 尝试从下载器取文件路径列表（类型断言 ListFiles）。
func listTorrentFileNames(ctx context.Context, dl reseedClient, hash string) []string {
	type fileLister interface {
		ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error)
	}
	fl, ok := dl.(fileLister)
	if !ok || hash == "" {
		return nil
	}
	files, err := fl.ListFiles(ctx, hash)
	if err != nil || len(files) == 0 {
		return nil
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		if f.Name != "" {
			out = append(out, f.Name)
		}
	}
	return out
}

// torznabCandidateSite 生成 candidate_site 去重键（DB size:64）。
// 用 FNV64 压缩 Guid/Link，避免长 URL 截断碰撞与字段溢出。
func torznabCandidateSite(r torznab.Result) string {
	raw := strings.TrimSpace(r.Guid)
	if raw == "" {
		raw = strings.TrimSpace(r.Link)
	}
	if raw == "" {
		raw = strings.TrimSpace(r.Title) + "|" + strconv.FormatInt(r.Size, 10)
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(raw))
	key := fmt.Sprintf("%016x", h.Sum64())
	idx := strings.TrimSpace(r.Indexer)
	if idx == "" {
		return "tz:" + key // 19 字符
	}
	// 索引名压缩到 20，总长 ≤ 3+20+1+16 = 40
	runes := []rune(idx)
	if len(runes) > 20 {
		idx = string(runes[:20])
	}
	return "tz/" + idx + ":" + key
}

// findTorznabCandidates 用 Jackett/Prowlarr Torznab 端点搜候选，再按 keepseed.Score 评分。
// 标题中若含 IMDb ID（tt...），优先带 imdbid 查询；结果 IMDb 一致时抬升到 ≥90。
// SiteID=0：添加时走下载器直链 URL（Torznab/indexer 下载链），不经站点 cookie。
func (c *KeepSeedChain) findTorznabCandidates(ctx context.Context, name string, size int64, ids keepseed.IDHints) []keepseed.Candidate {
	baseURL, apiKey, err := c.GetTorznab()
	if err != nil || strings.TrimSpace(baseURL) == "" {
		return nil
	}
	cli := &torznab.Client{BaseURL: baseURL, APIKey: apiKey}
	imdb := ids.ImdbID
	if imdb == "" {
		imdb = torznab.ExtractImdbID(name)
	}
	tmdb := keepseed.NormalizeTmdbID(ids.TmdbID)
	if tmdb == "" {
		tmdb = keepseed.ExtractTmdbID(name)
	}
	results, err := cli.Search(ctx, name, imdb, tmdb)
	if err != nil {
		c.reportTorznabFailure(ctx, err)
		return nil
	}
	out := make([]keepseed.Candidate, 0, len(results))
	for _, r := range results {
		score := keepseed.ScoreWith(name, r.Title, size, r.Size, ids, r.ImdbID, r.TmdbID)
		if score < 60 {
			continue
		}
		// CandidateSite 必须唯一：同 Indexer 多条结果不能共用 "Torznab"，否则 upsert 互相覆盖
		siteName := torznabCandidateSite(r)
		sizeStr := ""
		if r.Size > 0 {
			sizeStr = strconv.FormatInt(r.Size, 10)
		}
		out = append(out, keepseed.Candidate{
			SiteID:   0,
			SiteName: siteName,
			Torrent: site.Torrent{
				Title:       r.Title,
				DownloadURL: r.Link,
				DetailURL:   r.Guid,
				Size:        sizeStr,
				Seeders:     r.Seeders,
				SiteName:    siteName,
			},
			Score: score,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// ingestTorznabForDownloader 对下载器做种补搜 Torznab 候选并落库（IYUU 路径补充 / 与 reseedLocal 共用逻辑）。
func (c *KeepSeedChain) ingestTorznabForDownloader(ctx context.Context, downloaderID uint, torrents []downloader.Torrent, setting models.CrossSeedSetting) []models.CrossSeedTask {
	baseURL, _, err := c.GetTorznab()
	if err != nil || strings.TrimSpace(baseURL) == "" {
		return nil
	}
	const minScore = 75
	const maxPerSrc = 5
	created := make([]models.CrossSeedTask, 0)
	for _, t := range torrents {
		infoHash := torrentInfoHash(t)
		if infoHash == "" || t.SavePath == "" {
			continue
		}
		ids := c.mediaIDHints(infoHash, t.Name)
		cands := c.findTorznabCandidates(ctx, t.Name, t.Size, ids)
		n := 0
		for _, cand := range cands {
			if cand.Score < minScore {
				continue
			}
			url := cand.Torrent.DownloadURL
			if url == "" {
				url = cand.Torrent.DetailURL
			}
			if url == "" {
				continue
			}
			save := setting.SavePath
			if save == "" {
				save = t.SavePath
			}
			task, ok := c.upsertCrossSeedTask(downloaderID, cand.SiteID, infoHash, t.Name, cand.SiteName, url, save, cand.Score)
			if ok {
				created = append(created, task)
				n++
				if n >= maxPerSrc {
					break
				}
			}
		}
	}
	if len(created) > 0 {
		slog.Info("reseed torznab supplement", "downloader", downloaderID, "candidates", len(created))
	}
	return created
}
