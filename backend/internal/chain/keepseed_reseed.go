package chain

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"gorm.io/gorm"
)

// Reseed 对指定下载器的做种种子做辅种匹配，落 pending 候选任务并返回新增项。
// 优先 IYUU 精确 infohash（配置了 token 时）；未配置 token 时回退本地名称+大小跨站搜索（FindCandidates），
// 降低 IYUU 中心化单点依赖（feature-roadmap §1.2）。
func (c *KeepSeedChain) Reseed(ctx context.Context, downloaderID uint) ([]models.CrossSeedTask, error) {
	token, err := c.cfgOp.Get(IYUUTokenKey)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return c.reseedLocal(ctx, downloaderID)
	}

	dl, err := c.dl.Get(ctx, downloaderID)
	if err != nil {
		return nil, err
	}
	torrents, err := dl.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}

	// 仅取有 save_path 的做种（需复用其路径）；hash → 源信息。
	type src struct{ name, save string }
	srcByHash := map[string]src{}
	hashes := make([]string, 0, len(torrents))
	for _, t := range torrents {
		infoHash := torrentInfoHash(t)
		if infoHash == "" || t.SavePath == "" {
			continue
		}
		srcByHash[infoHash] = src{t.Name, t.SavePath}
		hashes = append(hashes, infoHash)
	}
	if len(hashes) == 0 {
		return nil, nil
	}

	matches, err := c.iyuu.HashIndex(ctx, token, hashes)
	if err != nil {
		// token 已配但 IYUU 不可达时回退本地+Torznab，避免自动辅种整链瘫痪
		slog.Warn("iyuu HashIndex failed, fallback reseedLocal", "err", err, "downloader", downloaderID)
		return c.reseedLocal(ctx, downloaderID)
	}
	iyuuSites, err := c.iyuu.Sites(ctx, token)
	if err != nil {
		return nil, err
	}
	siteBySid := make(map[int]iyuu.SiteInfo, len(iyuuSites))
	for _, s := range iyuuSites {
		siteBySid[s.ID] = s
	}

	localSites, err := c.siteOp.List()
	if err != nil {
		return nil, err
	}
	setting := c.loadSetting(downloaderID)

	created := make([]models.CrossSeedTask, 0)
	for _, m := range matches {
		si, ok := srcByHash[m.Hash]
		if !ok {
			continue
		}
		for _, t := range m.Torrents {
			iyuuSite, ok := siteBySid[t.Sid]
			if !ok {
				continue
			}
			local := matchSiteByHost(localSites, iyuuSite.BaseURL)
			if local == nil {
				continue // 本地未配置该站，跳过
			}
			candURL, ok := c.buildCandidateURL(local, iyuuSite, t.TorrentID)
			if !ok {
				continue // 无可用凭据（MTeam 缺 api_key / 其它站缺 passkey）
			}
			save := setting.SavePath
			if save == "" {
				save = si.save
			}
			// 同 (info_hash, 候选站) 去重 / 复活：
			//   pending|added → 进行中，跳过；
			//   skipped|failed → 复活为 pending（用户改主意 / 自动辅种重试失败任务）；
			//   不存在 → 新建。
			// 不再用 OnConflict{DoNothing}：它冲突时不插入却仍被计入返回值，会返回 ID=0
			// 幽灵候选，且 skipped/failed 行永久阻塞候选重建。
			var existing models.CrossSeedTask
			lookupErr := c.db.Where("info_hash = ? AND candidate_site = ?", m.Hash, iyuuSite.Site).First(&existing).Error
			switch {
			case lookupErr == nil:
				if existing.Status == "pending" || existing.Status == "added" {
					continue // 进行中，去重
				}
				if e := c.db.Model(&existing).Updates(map[string]any{
					"status": "pending", "candidate_url": candURL, "save_path": save, "resolved_at": nil,
				}).Error; e != nil {
					slog.Warn("reseed revive task", "hash", m.Hash, "site", iyuuSite.Site, "err", e)
					continue
				}
				existing.Status = "pending"
				existing.CandidateURL = candURL
				existing.SavePath = save
				existing.ResolvedAt = nil
				created = append(created, existing)
			case errors.Is(lookupErr, gorm.ErrRecordNotFound):
				task := &models.CrossSeedTask{
					DownloaderID:  downloaderID,
					SiteID:        local.ID,
					InfoHash:      m.Hash,
					SourceName:    si.name,
					CandidateSite: iyuuSite.Site,
					CandidateURL:  candURL,
					SavePath:      save,
					Status:        "pending",
				}
				// 并发兜底：唯一索引冲突（另一协程刚建同记录）静默跳过
				if e := c.db.Create(task).Error; e != nil {
					slog.Warn("reseed create task", "hash", m.Hash, "site", iyuuSite.Site, "err", e)
					continue
				}
				created = append(created, *task)
			default:
				slog.Warn("reseed lookup task", "hash", m.Hash, "site", iyuuSite.Site, "err", lookupErr)
			}
		}
	}
	// 补充 Torznab 候选（已配置 Jackett/Prowlarr 时），降低 IYUU 单点依赖
	extra := c.ingestTorznabForDownloader(ctx, downloaderID, torrents, setting)
	created = append(created, extra...)
	slog.Info("reseed done", "downloader", downloaderID, "hashes", len(hashes), "candidates", len(created))
	return created, nil
}

// reseedLocal 本地辅种：对下载器做种按名称+大小跨站搜索（keepseed.Score），高分候选落库。
// 不依赖 IYUU；适合 token 未配置或 IYUU 不可达时的兜底。score≥75 才收录，每源最多 5 候选。
func (c *KeepSeedChain) reseedLocal(ctx context.Context, downloaderID uint) ([]models.CrossSeedTask, error) {
	dl, err := c.dl.Get(ctx, downloaderID)
	if err != nil {
		return nil, err
	}
	torrents, err := dl.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	setting := c.loadSetting(downloaderID)
	created := make([]models.CrossSeedTask, 0)
	const minScore = 75
	const maxPerSrc = 5
	for _, t := range torrents {
		infoHash := torrentInfoHash(t)
		if infoHash == "" || t.SavePath == "" {
			continue
		}
		// 仅处理已完成/做种中的种子
		if t.Progress < 0.999 && t.CompletedAt.IsZero() {
			st := strings.ToLower(t.State)
			if st != "uploading" && st != "stalledup" && st != "forcedup" && st != "seeding" && st != "seed" {
				continue
			}
		}
		ids := c.mediaIDHints(infoHash, t.Name)
		fileNames := listTorrentFileNames(ctx, dl, infoHash)
		cands := c.findCandidatesWith(ctx, t.Name, t.Size, ids, fileNames)
		// 合并 Torznab 候选后按评分重排，再取 top maxPerSrc
		cands = append(cands, c.findTorznabCandidates(ctx, t.Name, t.Size, ids)...)
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
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
	slog.Info("reseed local done", "downloader", downloaderID, "candidates", len(created))
	return created, nil
}

// upsertCrossSeedTask 新建或复活 CrossSeedTask；返回 (task, true) 表示有新增/复活。
func (c *KeepSeedChain) upsertCrossSeedTask(downloaderID, siteID uint, infoHash, sourceName, candidateSite, candURL, save string, score int) (models.CrossSeedTask, bool) {
	var existing models.CrossSeedTask
	lookupErr := c.db.Where("info_hash = ? AND candidate_site = ?", infoHash, candidateSite).First(&existing).Error
	switch {
	case lookupErr == nil:
		if existing.Status == "pending" || existing.Status == "added" {
			return models.CrossSeedTask{}, false
		}
		if e := c.db.Model(&existing).Updates(map[string]any{
			"status": "pending", "candidate_url": candURL, "save_path": save, "resolved_at": nil, "match_score": score,
		}).Error; e != nil {
			slog.Warn("reseed revive task", "hash", infoHash, "site", candidateSite, "err", e)
			return models.CrossSeedTask{}, false
		}
		existing.Status = "pending"
		existing.CandidateURL = candURL
		existing.SavePath = save
		existing.ResolvedAt = nil
		existing.MatchScore = score
		return existing, true
	case errors.Is(lookupErr, gorm.ErrRecordNotFound):
		task := &models.CrossSeedTask{
			DownloaderID:  downloaderID,
			SiteID:        siteID,
			InfoHash:      infoHash,
			SourceName:    sourceName,
			CandidateSite: candidateSite,
			CandidateURL:  candURL,
			SavePath:      save,
			Status:        "pending",
			MatchScore:    score,
		}
		if e := c.db.Create(task).Error; e != nil {
			slog.Warn("reseed create task", "hash", infoHash, "site", candidateSite, "err", e)
			return models.CrossSeedTask{}, false
		}
		return *task, true
	default:
		slog.Warn("reseed lookup task", "hash", infoHash, "site", candidateSite, "err", lookupErr)
		return models.CrossSeedTask{}, false
	}
}
