package chain

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
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
	results := c.siteCh.SearchSites(ctx, name, "", nil) // 辅种按名称+大小匹配，不限媒体类型
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

// IYUUTokenKey 是 SystemConfig 中 IYUU token 的键。
const IYUUTokenKey = "iyuu_token"

// Torznab 配置键（Jackett/Prowlarr 端点，可选辅种候选源）。
const (
	TorznabURLKey    = "torznab_url"
	TorznabAPIKeyKey = "torznab_apikey"
)

// GetToken 读 IYUU token（空表示未配置）。
func (c *KeepSeedChain) GetToken() (string, error) { return c.cfgOp.Get(IYUUTokenKey) }

// SetToken 写 IYUU token。
func (c *KeepSeedChain) SetToken(token string) error { return c.cfgOp.Set(IYUUTokenKey, token) }

// GetTorznab 读 Torznab 端点配置。
func (c *KeepSeedChain) GetTorznab() (baseURL, apiKey string, err error) {
	baseURL, err = c.cfgOp.Get(TorznabURLKey)
	if err != nil {
		return "", "", err
	}
	apiKey, err = c.cfgOp.Get(TorznabAPIKeyKey)
	return baseURL, apiKey, err
}

// SetTorznab 写 Torznab 端点配置。
// url 空：清空 url+key；url 非空且 apiKey 空：只更新 url、保留已有 key（避免前端仅改 URL 时把 key 冲掉）。
func (c *KeepSeedChain) SetTorznab(baseURL, apiKey string) error {
	baseURL = strings.TrimSpace(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	if err := c.cfgOp.Set(TorznabURLKey, baseURL); err != nil {
		return err
	}
	if apiKey != "" {
		return c.cfgOp.Set(TorznabAPIKeyKey, apiKey)
	}
	if baseURL == "" {
		return c.cfgOp.Set(TorznabAPIKeyKey, "")
	}
	return nil // 保留旧 key
}

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

// findTorznabCandidates 用 Jackett/Prowlarr Torznab 端点搜候选，再按 keepseed.Score 评分。
// 标题中若含 IMDb ID（tt...），优先带 imdbid 查询；结果 IMDb 一致时抬升到 ≥90。
// SiteID=0：添加时走下载器直链 URL（Torznab/indexer 下载链），不经站点 cookie。

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
		slog.Debug("torznab search failed", "err", err, "q", name)
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
//   - 其它站：passkey 直链（iyuu.BuildDownloadURL），需 passkey（否则 false 跳过）。
func (c *KeepSeedChain) buildCandidateURL(local *models.Site, iyuuSite iyuu.SiteInfo, torrentID int) (string, bool) {
	if local.Platform == "mteam" {
		apiKey, _ := c.siteOp.APIKey(local)
		if apiKey == "" {
			return "", false // MTeam 无 api_key 无法 genDlToken
		}
		return mteamURLPrefix + strconv.Itoa(torrentID), true
	}
	passkey := c.siteOp.DecryptedPasskey(local)
	if passkey == "" {
		return "", false
	}
	return iyuu.BuildDownloadURL(iyuuSite, torrentID, passkey), true
}

// resolveCandidateURL 取添加用的真实 URL：MTeam 哨兵 → 现取 genDlToken；其它原样返回。
func (c *KeepSeedChain) resolveCandidateURL(ctx context.Context, task *models.CrossSeedTask) (string, error) {
	if !strings.HasPrefix(task.CandidateURL, mteamURLPrefix) {
		return task.CandidateURL, nil
	}
	if c.mt == nil {
		return "", errors.New("MTeam 辅种不可用：未注入 mteam 客户端")
	}
	st, err := c.siteOp.Get(task.SiteID)
	if err != nil {
		return "", fmt.Errorf("load site %d: %w", task.SiteID, err)
	}
	apiKey, err := c.siteOp.APIKey(st)
	if err != nil {
		return "", err
	}
	if apiKey == "" {
		return "", errors.New("MTeam 站未配置 api_key")
	}
	return c.mt.GenDlToken(ctx, st.BaseURL, apiKey, strings.TrimPrefix(task.CandidateURL, mteamURLPrefix))
}

// Resolve 处理一个候选任务：add（添加辅种到下载器）/ skip（标记跳过）。
func (c *KeepSeedChain) Resolve(ctx context.Context, taskID uint, action string) error {
	var task models.CrossSeedTask
	if err := c.db.First(&task, taskID).Error; err != nil {
		return err
	}
	now := time.Now()
	switch action {
	case "skip":
		task.Status = "skipped"
		task.ResolvedAt = &now
		return c.db.Save(&task).Error
	case "add":
		dl, err := c.dl.Get(ctx, task.DownloaderID)
		if err != nil {
			return err
		}
		url, err := c.resolveCandidateURL(ctx, &task)
		if err != nil {
			task.Status = "failed"
			task.ResolvedAt = &now
			if e := c.db.Save(&task).Error; e != nil {
				// 状态落库失败会让失败任务被当作未处理而反复重试。
				slog.Error("keepseed mark failed (resolve) persist error", "task_id", task.ID, "err", e)
			}
			return err
		}
		setting := c.loadSetting(task.DownloaderID)
		opts := downloader.AddOptions{
			URL:      url,
			SavePath: task.SavePath,
			Paused:   setting.PauseAfterAdd,
		}
		// 辅种 URL 多为 passkey/MTeam token 直链（FetchTorrentFileByID 按 schema 自动委托）；
		// 否则用站点 cookie 下载 .torrent 再以文件方式推（对标 DownloadChain.Add）。
		if task.SiteID != 0 && c.siteCh != nil {
			if content, _, ferr := c.siteCh.FetchTorrentFileByID(ctx, task.SiteID, url); ferr == nil {
				opts.FileContent, opts.URL = content, ""
			} else if !errors.Is(ferr, ErrDelegatedToDownloader) {
				task.Status = "failed"
				task.ResolvedAt = &now
				_ = c.db.Save(&task).Error
				return ferr
			}
		}
		if err := dl.AddTorrent(ctx, opts); err != nil {
			task.Status = "failed"
			task.ResolvedAt = &now
			if e := c.db.Save(&task).Error; e != nil {
				slog.Error("keepseed mark failed (add) persist error", "task_id", task.ID, "err", e)
			}
			return err
		}
		task.Status = "added"
		task.ResolvedAt = &now
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceReseedDone,
				Title:  "辅种完成：" + task.SourceName,
				Body:   "已添加 " + task.CandidateSite + " 的辅种种子到下载器",
				Level:  "success",
			}, false)
		}
		return c.db.Save(&task).Error
	default:
		return errors.New("invalid action: " + action)
	}
}

// ListTasks 列出辅种候选任务（status 空则全部；siteID>0 时按候选站 site_id 过滤），按创建时间倒序。
func (c *KeepSeedChain) ListTasks(status string, siteID uint) ([]models.CrossSeedTask, error) {
	var out []models.CrossSeedTask
	q := c.db.Model(&models.CrossSeedTask{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if siteID > 0 {
		q = q.Where("site_id = ?", siteID)
	}
	if err := q.Order("created_at DESC").Limit(500).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// loadSetting 取该下载器的辅种设置（save_path 覆盖 / 添加后暂停）；无记录返默认值
// （PauseAfterAdd=true，与 Setting/模型 default 一致——避免无 setting 行时「不暂停」添加）。
func (c *KeepSeedChain) loadSetting(downloaderID uint) models.CrossSeedSetting {
	s, err := c.Setting(downloaderID)
	if err != nil {
		// DB 瞬态错误：返默认值（PauseAfterAdd=true）而非零值（false），避免种子加下载器不暂停违反文档默认。
		slog.Warn("keepseed loadSetting failed, using default", "downloader", downloaderID, "err", err)
		return models.CrossSeedSetting{DownloaderID: downloaderID, PauseAfterAdd: true}
	}
	return s
}

// Setting 取该下载器的辅种设置（无记录返默认值行：PauseAfterAdd=true 对齐模型 default）。
func (c *KeepSeedChain) Setting(downloaderID uint) (models.CrossSeedSetting, error) {
	var s models.CrossSeedSetting
	err := c.db.Where("downloader_id = ?", downloaderID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.CrossSeedSetting{DownloaderID: downloaderID, PauseAfterAdd: true}, nil
	}
	return s, err
}

// SaveSetting 按 downloader_id upsert 辅种设置（AutoReseed/AutoReseedAdd/SavePath/PauseAfterAdd）。
func (c *KeepSeedChain) SaveSetting(s models.CrossSeedSetting) error {
	var row models.CrossSeedSetting
	c.db.Where("downloader_id = ?", s.DownloaderID).First(&row)
	// map-based Updates 强制写入零值 bool(false)：GORM Create 对「带 default 标签的零值字段」会
	// 省略插入、由 DB 填默认（PauseAfterAdd default:true 会把 false 落库为 true，实测如此）；用 map 绕开。
	updates := map[string]any{
		"save_path":       s.SavePath,
		"pause_after_add": s.PauseAfterAdd,
		"auto_reseed":     s.AutoReseed,
		"auto_reseed_add": s.AutoReseedAdd,
	}
	if row.ID == 0 {
		s.ID = 0
		if err := c.db.Create(&s).Error; err != nil {
			return err
		}
		return c.db.Model(&models.CrossSeedSetting{}).Where("id = ?", s.ID).Updates(updates).Error
	}
	row.SavePath = s.SavePath
	row.PauseAfterAdd = s.PauseAfterAdd
	row.AutoReseed = s.AutoReseed
	row.AutoReseedAdd = s.AutoReseedAdd
	return c.db.Save(&row).Error
}

// ListAutoReseedSettings 列出所有启用定时自动辅种的下载器设置（cron 用）。
func (c *KeepSeedChain) ListAutoReseedSettings() ([]models.CrossSeedSetting, error) {
	var out []models.CrossSeedSetting
	if err := c.db.Where("auto_reseed = ?", true).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// matchSiteByHost 把 IYUU 站点 host 匹配到本地 Site（按 base_url host 精确，回退后缀）。
func matchSiteByHost(sites []models.Site, iyuuHost string) *models.Site {
	want := hostOf(iyuuHost)
	if want == "" {
		return nil
	}
	for i := range sites {
		if hostOf(sites[i].BaseURL) == want {
			return &sites[i]
		}
	}
	// 回退：子域差异（如 IYUU 下载域 vs 本地配置域）后缀包含。
	for i := range sites {
		h := hostOf(sites[i].BaseURL)
		if h != "" && sameOrSubdomain(h, want) {
			return &sites[i]
		}
	}
	return nil
}

func sameOrSubdomain(a, b string) bool {
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

// hostOf 从（可能含 scheme 的）URL 取小写 host，规整端口和常见 www 前缀。
func hostOf(rawURL string) string {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	if u, err := url.Parse(s); err == nil {
		return normalizeHost(u.Hostname())
	}
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s[i+1:], "]") {
		s = s[:i]
	}
	return normalizeHost(s)
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "."))
	return strings.TrimPrefix(host, "www.")
}
