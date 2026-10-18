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

// matchesHash 判断某个 key（用户传来的 / 历史里存的）是否指向这颗种子。
//
// #364 为什么不只用 torrentInfoHash 单侧比较：写入侧存的是 t.Hash
// （record / AddSkipped 传的都是 t.Hash），匹配侧若一律换成 infohash，
// Aria2 下反而对不上（库里是 GID、拿 infohash 去比）。而只比 t.Hash 又是 #363 那个洞：
// 用户手动整理传的、前端拿到的、磁力链接给的都是 infohash，Aria2 列表里却是 GID。
//
// 两侧口径不同 ⇒ 比较时把两边都归一：任一命中即算命中。
// 这样存量 Aria2 历史（存 GID）与新数据（存 infohash）都能找回，不用做数据迁移。
// 两个 key 相同时只是重复比一次，不影响正确性（纯函数，无副作用）。
func matchesHash(key string, t downloader.Torrent) bool {
	if key == "" {
		return false
	}
	byHash := t.Hash
	byInfo := t.InfoHash
	if byInfo == "" {
		byInfo = byHash
	}
	return key == byInfo || (byHash != "" && key == byHash)
}

// FindMissing 未辅种检查：对源下载器的做种，用 IYUU hash 跨站匹配检查目标站是否已辅种，
// 返回每个目标站未辅种的种子清单（仅统计，不自动添加——与 Reseed「加辅种」互补，这里是「查漏」）。
// 目标站经域名映射到 IYUU sid；未在 IYUU 目录的目标站 Count=-1 标注无法判定。
// 查漏的前置条件不满足时，原因必须原样抵达用户：从前 handler 只用 WithErr 挂原因，
// 界面统一弹「请求参数有误，请检查后重试」，用户既不知道要配 IYUU，也不知道要选目标站。
var (
	ErrMissingNoToken  = errors.New("未配置 IYUU token，请先在辅种页保存")
	ErrMissingNoTarget = errors.New("未指定要检查的目标站点")
)

func (c *KeepSeedChain) FindMissing(ctx context.Context, downloaderID uint, targetSiteIDs []uint) (*MissingSummary, error) {
	summary := &MissingSummary{DownloaderID: downloaderID, Missing: []MissingSeed{}}
	token, err := c.cfgOp.Get(IYUUTokenKey)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, ErrMissingNoToken
	}
	if len(targetSiteIDs) == 0 {
		// 空目标站 = 没有要检查的站，不是「所有站都不缺」。返回空清单会让 UI/工具把
		// 「一个站都没选」渲染成绿色的「未辅种 0 个」，这里按调用错误处理。
		return nil, ErrMissingNoTarget
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
