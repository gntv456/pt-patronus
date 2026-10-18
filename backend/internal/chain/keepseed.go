package chain

import (
	"context"
	"log/slog"
	"sort"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"github.com/hxpt/ptpatronus/internal/keepseed"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/torznab"
	"gorm.io/gorm"
)

// reseedClient 是辅种所需的最小下载器能力（取做种列表 + 添加种子）。
type reseedClient interface {
	ListTorrents(ctx context.Context) ([]downloader.Torrent, error)
	AddTorrent(ctx context.Context, opts downloader.AddOptions) error
}

// dlProvider 按 id 取下载器（解耦 *manager.Manager，便于测试注入 fake）。
type dlProvider interface {
	Get(ctx context.Context, id uint) (reseedClient, error)
}

// dlSource 是 *manager.Manager 满足的最小接口（Get 返回完整 Downloader，可退化为 reseedClient）。
type dlSource interface {
	Get(ctx context.Context, id uint) (downloader.Downloader, error)
}

// ManagerProvider 把 *manager.Manager 适配为 dlProvider。
type ManagerProvider struct{ S dlSource }

// Get 取下载器（downloader.Downloader 天然满足 reseedClient）。
func (p ManagerProvider) Get(ctx context.Context, id uint) (reseedClient, error) {
	return p.S.Get(ctx, id)
}

// NewManagerProvider 用 *manager.Manager 构造 dlProvider。
func NewManagerProvider(s dlSource) dlProvider { return ManagerProvider{S: s} }

// mtTokenGen 是 MTeam 辅种所需的最小能力：现取签名下载 URL（genDlToken）。
// *schemas.MTeam 天然满足；测试注入 fake，沿用 dlProvider 范式。
type mtTokenGen interface {
	GenDlToken(ctx context.Context, baseURL, apiKey, torrentID string) (string, error)
}

// mteam 候选 URL 哨兵前缀（真 URL 由 Resolve 现取 genDlToken，避免入库后时效失效）。
const mteamURLPrefix = "mteam:"

// KeepSeedChain 编排辅种：① FindCandidates 名称模糊匹配（跨站搜索+评分）；
// ② Reseed IYUU 精确 infohash 匹配（取下载器做种 hash → IYUU → 拼 passkey URL → 落 CrossSeedTask）；
// ③ Resolve 对候选任务一键添加辅种 / 跳过。
type KeepSeedChain struct {
	Base
	siteCh *SiteChain
	iyuu   *iyuu.Client
	dl     dlProvider
	mt     mtTokenGen // MTeam genDlToken（nil 则 MTeam 候选无法现取链接）
	siteOp *oper.SiteOper
	cfgOp  *oper.SystemConfigOper
	db     *gorm.DB
	notice *notice.Service // 站内通知中心（辅种完成；nil 跳过）
}

// NewKeepSeedChain 构造。iyuu/dl/mt/siteOp/cfgOp/db 供精确辅种（Reseed/Resolve）使用。
// ns 为站内通知中心（辅种完成发通知；nil 跳过）。
func NewKeepSeedChain(b Base, siteCh *SiteChain, iyuuC *iyuu.Client, dl dlProvider, mt mtTokenGen,
	siteOp *oper.SiteOper, cfgOp *oper.SystemConfigOper, db *gorm.DB, ns *notice.Service) *KeepSeedChain {
	return &KeepSeedChain{Base: b, siteCh: siteCh, iyuu: iyuuC, dl: dl, mt: mt, siteOp: siteOp, cfgOp: cfgOp, db: db, notice: ns}
}

// FindCandidates 跨站搜索同名种子，按名称相似度+大小接近度评分，返回高分候选（名称模糊匹配）。
func (c *KeepSeedChain) FindCandidates(ctx context.Context, name string, size int64) []keepseed.Candidate {
	return c.findCandidatesWith(ctx, name, size, keepseed.IDHints{
		ImdbID: torznab.ExtractImdbID(name),
		TmdbID: keepseed.ExtractTmdbID(name),
	}, nil)
}

// findCandidatesWith 跨站搜索；ids/fileNames 用于抬分（TMDB/IMDb/文件树）。
func (c *KeepSeedChain) findCandidatesWith(ctx context.Context, name string, size int64, ids keepseed.IDHints, fileNames []string) []keepseed.Candidate {
	if c.siteCh == nil {
		return nil
	}
	results, err := c.siteCh.SearchSites(ctx, name, "", nil) // 辅种按名称+大小匹配，不限媒体类型
	if err != nil {
		// 清单读失败不能无声渲染成「无候选」：至少日志留痕（FindCandidates 签名不动，调用方多）。
		slog.Warn("keepseed search sites failed", "err", err)
		return nil
	}
	var out []keepseed.Candidate
	for _, r := range results {
		for _, tor := range r.Torrents {
			score := keepseed.ScoreWith(name, tor.Title, size, site.ParseBytes(tor.Size), ids,
				torznab.ExtractImdbID(tor.Title), keepseed.ExtractTmdbID(tor.Title))
			if fs := keepseed.ScoreFileTree(fileNames, []string{tor.Title, tor.SubTitle}); fs > 0 {
				score = keepseed.BlendFileTree(score, fs)
			}
			if score >= 60 {
				out = append(out, keepseed.Candidate{
					SiteID: r.SiteID, SiteName: r.SiteName, Torrent: tor, Score: score,
				})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
