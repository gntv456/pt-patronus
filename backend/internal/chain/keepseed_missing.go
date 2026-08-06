package chain

import (
	"context"
	"errors"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
)

// MissingSample 是一个未辅种种子的简要信息。
type MissingSample struct {
	Hash string `json:"hash"`
	Name string `json:"name"`
}

// MissingSeed 描述某目标站的未辅种情况。Count=-1 表示该站未映射到 IYUU 目录（无法判定）。
type MissingSeed struct {
	SiteID   uint            `json:"site_id"`
	SiteName string          `json:"site_name"`
	Count    int             `json:"count"`
	Samples  []MissingSample `json:"samples"`
}

// MissingSummary 未辅种检查结果。
type MissingSummary struct {
	DownloaderID uint          `json:"downloader_id"`
	Total        int           `json:"total"`   // 源做种总数（有 hash 的）
	Checked      int           `json:"checked"` // 提交 IYUU 的 hash 数
	Targets      int           `json:"targets"` // 成功映射到 IYUU sid 的目标站数
	Missing      []MissingSeed `json:"missing"`
}

func torrentInfoHash(t downloader.Torrent) string {
	if t.InfoHash != "" {
		return t.InfoHash
	}
	return t.Hash
}

// FindMissing 未辅种检查：对源下载器的做种，用 IYUU hash 跨站匹配检查目标站是否已辅种，
// 返回每个目标站未辅种的种子清单（仅统计，不自动添加——与 Reseed「加辅种」互补，这里是「查漏」）。
// 目标站经域名映射到 IYUU sid；未在 IYUU 目录的目标站 Count=-1 标注无法判定。
func (c *KeepSeedChain) FindMissing(ctx context.Context, downloaderID uint, targetSiteIDs []uint) (*MissingSummary, error) {
	summary := &MissingSummary{DownloaderID: downloaderID, Missing: []MissingSeed{}}
	token, err := c.cfgOp.Get(IYUUTokenKey)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, errors.New("未配置 IYUU token，请先在辅种页保存")
	}
	if len(targetSiteIDs) == 0 {
		return summary, nil
	}

	dl, err := c.dl.Get(ctx, downloaderID)
	if err != nil {
		return nil, err
	}
	torrents, err := dl.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	type srcInfo struct{ name string }
	srcByHash := map[string]srcInfo{}
	hashes := make([]string, 0, len(torrents))
	for _, t := range torrents {
		infoHash := torrentInfoHash(t)
		if infoHash == "" {
			continue
		}
		srcByHash[infoHash] = srcInfo{t.Name}
		hashes = append(hashes, infoHash)
	}
	summary.Total = len(hashes)
	if len(hashes) == 0 {
		return summary, nil
	}

	matches, err := c.iyuu.HashIndex(ctx, token, hashes)
	if err != nil {
		return nil, err
	}
	summary.Checked = len(hashes)

	iyuuSites, err := c.iyuu.Sites(ctx, token)
	if err != nil {
		return nil, err
	}

	localSites, err := c.siteOp.List()
	if err != nil {
		return nil, err
	}
	targetByID := map[uint]models.Site{}
	for _, ls := range localSites {
		for _, tid := range targetSiteIDs {
			if ls.ID == tid {
				targetByID[tid] = ls
			}
		}
	}
	if len(targetByID) == 0 {
		return summary, nil
	}

	// iyuu sid → 目标本地站 id（该 iyuu 站映射到某目标站）
	targetSlice := make([]models.Site, 0, len(targetByID))
	for _, s := range targetByID {
		targetSlice = append(targetSlice, s)
	}
	sidToTarget := map[int]uint{}
	for _, is := range iyuuSites {
		if t := matchSiteByHost(targetSlice, is.BaseURL); t != nil {
			sidToTarget[is.ID] = t.ID
		}
	}
	// target id 是否被至少一个 iyuu sid 覆盖
	targetHasSid := map[uint]bool{}
	for _, tid := range sidToTarget {
		targetHasSid[tid] = true
	}
	summary.Targets = len(targetHasSid)

	// hash → 命中的目标 id 集合
	hitTargets := map[string]map[uint]bool{}
	for _, m := range matches {
		set := map[uint]bool{}
		for _, t := range m.Torrents {
			if tid, ok := sidToTarget[t.Sid]; ok {
				set[tid] = true
			}
		}
		hitTargets[m.Hash] = set
	}

	// 对每个目标站收集未命中 hash
	const sampleMax = 20
	for tid, site := range targetByID {
		if !targetHasSid[tid] {
			// 该站未在 IYUU 目录中映射到，无法判定
			summary.Missing = append(summary.Missing, MissingSeed{
				SiteID: tid, SiteName: site.Name, Count: -1,
			})
			continue
		}
		var samples []MissingSample
		cnt := 0
		for hash, si := range srcByHash {
			if !hitTargets[hash][tid] {
				cnt++
				if len(samples) < sampleMax {
					samples = append(samples, MissingSample{Hash: hash, Name: si.name})
				}
			}
		}
		summary.Missing = append(summary.Missing, MissingSeed{
			SiteID: tid, SiteName: site.Name, Count: cnt, Samples: samples,
		})
	}
	return summary, nil
}

// buildCandidateURL 拼候选下载 URL：
//   - MTeam：存哨兵 mteam:<id>（真 URL 由 Resolve 现取 genDlToken，避免入库后时效失效），
//     需本地已配 api_key（否则 false 跳过）。
