// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"fmt"
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
	// #384 读不出凭据的站点（站名 → 原因）。跑完统一报，不中断其余候选的建立。
	var unreadableSites map[string]error
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
			candURL, ok, credErr := c.buildCandidateURL(local, iyuuSite, t.TorrentID)
			if credErr != nil {
				// #384 「填了但解不开」不是「没填」：前者是本地故障（主密钥换过/密文损坏），
				// 用户该去重存凭据；后者只是没配，补上即可。两者此前都被这一行 continue
				// 吞成同形，界面只显示「没匹配到可辅种的种子」，没有任何地方说读不出凭据。
				// 这里不中断整轮（其余候选仍可建），但记下站点名，结束后一次性报出来。
				if unreadableSites == nil {
					unreadableSites = map[string]error{}
				}
				if _, seen := unreadableSites[local.Name]; !seen {
					unreadableSites[local.Name] = credErr
				}
				continue
			}
			if !ok {
				continue // 真·没配凭据（MTeam 缺 api_key / 其它站缺 passkey）：正常跳过
			}
			save := setting.SavePath
			if save == "" {
				save = si.save
			}
			// 同 (info_hash, 候选站) 去重 / 复活：
			//   pending|added → 进行中，跳过；
			//   skipped|failed → 复活为 pending（「重新发现」既定语义，见 TestReseedRediscoveryAfterSkip）；
			//   不存在 → 新建。
			// 不再用 OnConflict{DoNothing}：它冲突时不插入却仍被计入返回值，会返回 ID=0
			// 幽灵候选，且 skipped/failed 行永久阻塞候选重建。
			var existing models.CrossSeedTask
			lookupErr := c.db.Where("downloader_id = ? AND info_hash = ? AND candidate_site = ?",
				downloaderID, m.Hash, iyuuSite.Site).First(&existing).Error
			switch {
			case lookupErr == nil:
				if existing.Status == "pending" || existing.Status == "added" {
					continue // 进行中，去重
				}
				wasSkip := existing.Status == "skipped"
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
				if wasSkip {
					// 用户点过「跳过」的候选：重新发现只把行恢复成 pending 让他在列表里看见、
					// 再决定一次，但不放进返回值——cron 的 AutoReseedAdd 会把返回值逐条
					// Resolve("add")，交出去等于自动化推翻用户的显式否决。
					// failed 仍照常返回（自动重试失败任务是设计的一部分）。
					continue
				}
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
	// #384 有站点的凭据读不出来时必须说出来，不能让用户对着「没匹配到同种」猜。
	// 逐站带名字（用户不知道该去查哪站），并说清下一步是「重存凭据」而不是「去补填」。
	// 已建成的候选照常返回 —— 读不出的只是其中几站，不该把整轮成果一起丢掉。
	if len(unreadableSites) > 0 {
		names := make([]string, 0, len(unreadableSites))
		var first error
		for n, e := range unreadableSites {
			names = append(names, n)
			if first == nil {
				first = e
			}
		}
		sort.Strings(names)
		slog.Warn("reseed: 站点凭据读不出，相关候选已跳过", "sites", strings.Join(names, ", "))
		return created, fmt.Errorf("这些站点的凭据读不出来（可能主密钥换过或密文损坏），相关候选已跳过：%s；"+
			"请到「站点」页重新保存这些站的凭据：%w", strings.Join(names, "、"), first)
	}
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
	lookupErr := c.db.Where("downloader_id = ? AND info_hash = ? AND candidate_site = ?",
		downloaderID, infoHash, candidateSite).First(&existing).Error
	switch {
	case lookupErr == nil:
		if existing.Status == "pending" || existing.Status == "added" {
			return models.CrossSeedTask{}, false
		}
		wasSkip := existing.Status == "skipped"
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
		// 与 Reseed 同一规则：用户「跳过」过的候选恢复成 pending 让他看得见，但不交给
		// 调用方（本地辅种/Torznab 补候选的返回值同样会被 AutoReseedAdd 逐条添加）。
		if wasSkip {
			return models.CrossSeedTask{}, false
		}
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
