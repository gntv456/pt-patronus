package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	results := c.siteCh.SearchSites(ctx, name, "", nil) // 辅种按名称+大小匹配，不限媒体类型
	var out []keepseed.Candidate
	for _, r := range results {
		for _, t := range r.Torrents {
			score := keepseed.Score(name, t.Title, size, site.ParseBytes(t.Size))
			if score >= 60 {
				out = append(out, keepseed.Candidate{
					SiteID: r.SiteID, SiteName: r.SiteName, Torrent: t, Score: score,
				})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// IYUUTokenKey 是 SystemConfig 中 IYUU token 的键。
const IYUUTokenKey = "iyuu_token"

// GetToken 读 IYUU token（空表示未配置）。
func (c *KeepSeedChain) GetToken() (string, error) { return c.cfgOp.Get(IYUUTokenKey) }

// SetToken 写 IYUU token。
func (c *KeepSeedChain) SetToken(token string) error { return c.cfgOp.Set(IYUUTokenKey, token) }

// Reseed 对指定下载器的做种种子做 IYUU 精确辅种匹配，落 pending 候选任务并返回新增项。
func (c *KeepSeedChain) Reseed(ctx context.Context, downloaderID uint) ([]models.CrossSeedTask, error) {
	token, err := c.cfgOp.Get(IYUUTokenKey)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, errors.New("未配置 IYUU token，请先在辅种页保存")
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
		if t.Hash == "" || t.SavePath == "" {
			continue
		}
		srcByHash[t.Hash] = src{t.Name, t.SavePath}
		hashes = append(hashes, t.Hash)
	}
	if len(hashes) == 0 {
		return nil, nil
	}

	matches, err := c.iyuu.HashIndex(ctx, token, hashes)
	if err != nil {
		return nil, err
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
			if c.taskExists(m.Hash, iyuuSite.Site) {
				continue // 同 hash+候选站已有 pending/added，去重
			}
			save := setting.SavePath
			if save == "" {
				save = si.save
			}
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
			// OnConflict DoNothing：(info_hash,candidate_site) 唯一索引兜底，
			// cron 重叠（已加 SkipIfStillRunning 进一步收敛）或手动并发时重复候选静默跳过。
			if err := c.db.Clauses(clause.OnConflict{DoNothing: true}).Create(task).Error; err != nil {
				slog.Warn("reseed create task", "hash", m.Hash, "site", iyuuSite.Site, "err", err)
				continue
			}
			created = append(created, *task)
		}
	}
	slog.Info("reseed done", "downloader", downloaderID, "hashes", len(hashes), "candidates", len(created))
	return created, nil
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
		if err := dl.AddTorrent(ctx, downloader.AddOptions{
			URL:      url,
			SavePath: task.SavePath,
			Paused:   setting.PauseAfterAdd,
		}); err != nil {
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

// ListTasks 列出辅种候选任务（status 空则全部），按创建时间倒序。
func (c *KeepSeedChain) ListTasks(status string) ([]models.CrossSeedTask, error) {
	var out []models.CrossSeedTask
	q := c.db.Model(&models.CrossSeedTask{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Order("created_at DESC").Limit(500).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// loadSetting 取该下载器的辅种设置（save_path 覆盖 / 添加后暂停）；无记录返默认值
//（PauseAfterAdd=true，与 Setting/模型 default 一致——避免无 setting 行时「不暂停」添加）。
func (c *KeepSeedChain) loadSetting(downloaderID uint) models.CrossSeedSetting {
	s, _ := c.Setting(downloaderID)
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

// taskExists 同 infohash + 候选站是否已有 pending/added 任务（去重）。
func (c *KeepSeedChain) taskExists(infoHash, candidateSite string) bool {
	var count int64
	c.db.Model(&models.CrossSeedTask{}).
		Where("info_hash = ? AND candidate_site = ? AND status IN ?", infoHash, candidateSite, []string{"pending", "added"}).
		Count(&count)
	return count > 0
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
		if h != "" && (strings.HasSuffix(h, want) || strings.HasSuffix(want, h)) {
			return &sites[i]
		}
	}
	return nil
}

// hostOf 从（可能含 scheme 的）URL 取小写 host。
func hostOf(rawURL string) string {
	s := strings.TrimSpace(rawURL)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}
