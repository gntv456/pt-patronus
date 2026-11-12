// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// SiteStatItem 是某站点的最新统计。
type SiteStatItem struct {
	SiteID                    uint      `json:"site_id"`
	SiteName                  string    `json:"site_name"`
	UserName                  string    `json:"username"` // 站点用户名（导出海报身份行，best-effort）
	UserID                    string    `json:"user_id"`  // 站点用户 ID
	Email                     string    `json:"email"`    // 账号邮箱（best-effort）
	Level                     string    `json:"level"`
	Uploaded                  int64     `json:"uploaded"`
	Downloaded                int64     `json:"downloaded"`
	Ratio                     float64   `json:"ratio"`
	TrueUploaded              int64     `json:"true_uploaded"`
	TrueDownloaded            int64     `json:"true_downloaded"`
	TrueRatio                 float64   `json:"true_ratio"`
	Seeding                   int       `json:"seeding"`
	Leeching                  int       `json:"leeching"`
	SeedingSize               int64     `json:"seeding_size"`
	Uploads                   int       `json:"uploads"`
	UniqueGroups              int       `json:"unique_groups"`
	PerfectFLAC               int       `json:"perfect_flac"`
	Invited                   int       `json:"invited"`
	Bonus                     float64   `json:"bonus"`
	BonusPerHour              float64   `json:"bonus_per_hour"`
	SeedingBonus              float64   `json:"seeding_bonus"`
	SeedingBonusPerHour       float64   `json:"seeding_bonus_per_hour"`
	RescueSeedingBonusPerHour float64   `json:"rescue_seeding_bonus_per_hour"`
	ClaimCount                int       `json:"claim_count"`
	MaxClaim                  int       `json:"max_claim"`
	JoinDate                  string    `json:"join_date"`
	LastAccessAt              string    `json:"last_access_at"`
	UpdatedAt                 time.Time `json:"updated_at"` // 该统计的抓取时间（SiteStat.SnapAt）
}

// DashboardOverview 是仪表盘聚合数据。
type DashboardOverview struct {
	Totals struct {
		Uploaded                  int64   `json:"uploaded"`
		Downloaded                int64   `json:"downloaded"`
		TrueUploaded              int64   `json:"true_uploaded"`
		TrueDownloaded            int64   `json:"true_downloaded"`
		TrueRatio                 float64 `json:"true_ratio"`
		Seeding                   int     `json:"seeding"`
		Leeching                  int     `json:"leeching"`
		SeedingSize               int64   `json:"seeding_size"`
		UniqueGroups              int     `json:"unique_groups"`
		PerfectFLAC               int     `json:"perfect_flac"`
		Invited                   int     `json:"invited"`
		Bonus                     float64 `json:"bonus"`
		BonusPerHour              float64 `json:"bonus_per_hour"`
		SeedingBonus              float64 `json:"seeding_bonus"`
		SeedingBonusPerHour       float64 `json:"seeding_bonus_per_hour"`
		RescueSeedingBonusPerHour float64 `json:"rescue_seeding_bonus_per_hour"`
		ClaimCount                int     `json:"claim_count"`
		MaxClaim                  int     `json:"max_claim"`
	} `json:"totals"`
	Sites           []SiteStatItem `json:"sites"`
	SiteCount       int            `json:"site_count"`
	DownloaderCount int            `json:"downloader_count"`
	NoDataSites     []NoDataSite   `json:"no_data_sites"` // 启用且非论坛、却无任何统计的站点（疑似 cookie 失效/未采集）
	// StatReadError 非空表示「每站最新统计」这条查询本身失败（库锁/表异常）。
	// 此时 NoDataSites 刻意留空：读不到统计不等于站点没数据，混在一起会让用户
	// 白跑一趟去重登所有站点。前端据这两个字段区分「未采集」与「读取故障」。
	StatReadError string `json:"stat_read_error,omitempty"`
}

// NoDataSite 标记一个「应采集却无数据」的 PT 站点（用于仪表盘首屏红色汇总）。
type NoDataSite struct {
	SiteID   uint   `json:"site_id"`
	SiteName string `json:"site_name"`
	Platform string `json:"platform"`
}

// isForumPlatform 判断该平台是否为「无 PT 用户统计」的论坛/社区——此类站点本就没有
// 上传/下载/做种等指标，不应纳入「无数据」告警。
func isForumPlatform(p string) bool {
	switch p {
	case "flarum", "discuz":
		return true
	}
	return false
}

// TrendPoint 是某一天的跨站聚合趋势点。
type TrendPoint struct {
	Day         string  `json:"day"` // YYYY-MM-DD（string 跨 SQLite/MySQL/PG 兼容）
	Uploaded    int64   `json:"uploaded"`
	Downloaded  int64   `json:"downloaded"`
	Seeding     int     `json:"seeding"`
	SeedingSize int64   `json:"seeding_size"`
	Bonus       float64 `json:"bonus"`
	Ratio       float64 `json:"ratio"` // 当日跨站聚合分享率 = uploaded/downloaded（Scan 后补算，DB 无关）
}

// DashboardTrend 是按天聚合的趋势序列。
type DashboardTrend struct {
	Days   int          `json:"days"`
	Points []TrendPoint `json:"points"`
}

// DashboardChain 聚合仪表盘数据（纯 DB，无网络调用）。
type DashboardChain struct {
	db     *gorm.DB
	siteOp *oper.SiteOper
	dlOp   *oper.DownloaderOper
	siteCh *SiteChain // 即时统计同步：抓取 UserInfo 写快照

	// emitProgress 全站同步进度推送（W4 任务中心）：每完成一站回调
	// (done, total, siteName)——app 装配期接到 WS 广播；nil 跳过（测试/安全模式）。
	emitProgress func(done, total int, siteName string)
}

// SetProgressEmitter 注入进度回调（app 装配期）。
func (c *DashboardChain) SetProgressEmitter(fn func(done, total int, siteName string)) {
	c.emitProgress = fn
}

// syncMu 全站同步单飞（二轮 P1-3）：手动触发与 cron 并发时只跑一个，
// 避免双份逐站抓取打爆站点限流。
var syncMu sync.Mutex

// syncing 当前是否有全站同步在跑（StartSyncAll 管理）。
var syncing atomic.Bool

// StartSyncAll 后台启动全站同步（立即返回是否受理）。已同步使用中的
// httpclient per-host 限流；ctx 独立 15min——不随 HTTP 客户端断开取消。
func (c *DashboardChain) StartSyncAll() bool {
	if !syncing.CompareAndSwap(false, true) {
		return false
	}
	syncMu.Lock() // 与 cron 路径互斥（见 SyncAllLocked 说明）
	go func() {
		defer syncing.Store(false)
		defer syncMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if _, err := c.SyncAll(ctx); err != nil {
			slog.Warn("dashboard background sync failed", "err", err)
		}
	}()
	return true
}

// NewDashboardChain 构造。
func NewDashboardChain(db *gorm.DB, siteOp *oper.SiteOper, dlOp *oper.DownloaderOper, siteCh *SiteChain) *DashboardChain {
	return &DashboardChain{db: db, siteOp: siteOp, dlOp: dlOp, siteCh: siteCh}
}

// Overview 聚合所有站点最新统计 + 下载器数。
func (c *DashboardChain) Overview(ctx context.Context) (*DashboardOverview, error) {
	sites, err := c.siteOp.List()
	if err != nil {
		return nil, err
	}
	dls, err := c.dlOp.List()
	if err != nil {
		return nil, err
	}

	// Sites 预分配空切片：无站点时 nil slice 会被 json.Marshal 序列化成 "sites":null，
	// 前端裸访问 overview.sites.length 会抛 "Cannot read properties of null"。
	out := &DashboardOverview{Sites: make([]SiteStatItem, 0, len(sites)), NoDataSites: []NoDataSite{}}
	out.DownloaderCount = len(dls)

	// 单 SQL 取每站最新统计（MAX(id) GROUP BY site_id），避免 N+1（原每站一次 First +
	// ORDER BY filesort，100 站 = 100 次查询）。id 自增 ⇔ 时间递增，MAX(id) 即最新一条。
	var latest []models.SiteStat
	// 错误不能吞：查询失败时 statMap 为空，下面每个站都会走「无统计」分支，
	// 首屏于是报「疑似 cookie 失效/未采集」——把读取故障渲染成站点集体掉线。
	if err := c.db.WithContext(ctx).
		Where("id IN (SELECT MAX(id) FROM site_stats GROUP BY site_id)").
		Find(&latest).Error; err != nil {
		slog.Warn("dashboard read latest stats failed", "err", err)
		out.StatReadError = errtext.Text(err)
	}
	statMap := make(map[uint]models.SiteStat, len(latest))
	for _, st := range latest {
		statMap[st.SiteID] = st
	}

	for _, s := range sites {
		item := SiteStatItem{
			SiteID:   s.ID,
			SiteName: s.Name,
		}
		if stat, ok := statMap[s.ID]; ok && stat.ID > 0 {
			item.Level = stat.Level
			item.UserName = stat.UserName
			item.UserID = stat.UserID
			item.Email = stat.Email
			item.Uploaded = stat.Uploaded
			item.Downloaded = stat.Downloaded
			item.Ratio = stat.Ratio
			item.TrueUploaded = stat.TrueUploaded
			item.TrueDownloaded = stat.TrueDownloaded
			item.TrueRatio = stat.TrueRatio
			item.Seeding = stat.Seeding
			item.Leeching = stat.Leeching
			item.SeedingSize = stat.SeedingSize
			item.Uploads = stat.Uploads
			item.UniqueGroups = stat.UniqueGroups
			item.PerfectFLAC = stat.PerfectFLAC
			item.Invited = stat.Invited
			item.Bonus = stat.Bonus
			item.BonusPerHour = stat.BonusPerHour
			item.SeedingBonus = stat.SeedingBonus
			item.SeedingBonusPerHour = stat.SeedingBonusPerHour
			item.RescueSeedingBonusPerHour = stat.RescueSeedingBonusPerHour
			item.ClaimCount = stat.ClaimCount
			item.MaxClaim = stat.MaxClaim
			item.JoinDate = stat.JoinDate
			item.LastAccessAt = stat.LastAccessAt
			item.UpdatedAt = stat.SnapAt
			// ratio 兜底：适配器未解析出 ratio（=0）但已有上下行量时按 uploaded/downloaded 计算，
			// 避免「上传>下载却显示成无分享率」（如朱雀 /api/plugins/ptppUserInfo 响应缺 ratio 字段）。
			if item.Ratio == 0 && item.Uploaded > 0 && item.Downloaded > 0 {
				item.Ratio = float64(item.Uploaded) / float64(item.Downloaded)
			}
			if item.TrueRatio == 0 && item.TrueUploaded > 0 && item.TrueDownloaded > 0 {
				item.TrueRatio = float64(item.TrueUploaded) / float64(item.TrueDownloaded)
			}
		}
		out.Sites = append(out.Sites, item)
		out.Totals.Uploaded += item.Uploaded
		out.Totals.Downloaded += item.Downloaded
		out.Totals.TrueUploaded += item.TrueUploaded
		out.Totals.TrueDownloaded += item.TrueDownloaded
		out.Totals.Seeding += item.Seeding
		out.Totals.Leeching += item.Leeching
		out.Totals.SeedingSize += item.SeedingSize
		out.Totals.UniqueGroups += item.UniqueGroups
		out.Totals.PerfectFLAC += item.PerfectFLAC
		out.Totals.Invited += item.Invited
		out.Totals.Bonus += item.Bonus
		out.Totals.BonusPerHour += item.BonusPerHour
		out.Totals.SeedingBonus += item.SeedingBonus
		out.Totals.SeedingBonusPerHour += item.SeedingBonusPerHour
		out.Totals.RescueSeedingBonusPerHour += item.RescueSeedingBonusPerHour
		out.Totals.ClaimCount += item.ClaimCount
		out.Totals.MaxClaim += item.MaxClaim
		// 无数据站点：启用、非论坛、却无任何统计（疑似 cookie 失效/未采集）→ 仪表盘首屏红色汇总。
		// 统计读取本身失败时不判——那是「读不到」，不是「没数据」。
		if out.StatReadError == "" && s.IsActive && !isForumPlatform(s.Platform) &&
			item.Uploaded == 0 && item.Downloaded == 0 && item.Seeding == 0 &&
			item.SeedingSize == 0 && item.Bonus == 0 {
			out.NoDataSites = append(out.NoDataSites, NoDataSite{SiteID: s.ID, SiteName: s.Name, Platform: s.Platform})
		}
	}
	if out.Totals.TrueDownloaded > 0 {
		out.Totals.TrueRatio = float64(out.Totals.TrueUploaded) / float64(out.Totals.TrueDownloaded)
	}
	out.SiteCount = len(sites)
	return out, nil
}

// SyncAll 立即抓取所有启用站点的统计并写入快照，然后返回刷新后的总览。
// 用于「加入站点后数据全为 0」的即时回填——不必等 stats-sync 定时任务（默认 30 分钟）。
func (c *DashboardChain) SyncAll(ctx context.Context) (*DashboardOverview, error) {
	syncMu.Lock()
	defer syncMu.Unlock()
	sites, err := c.siteOp.ListActive()
	if err != nil {
		return nil, err
	}
	total := len(sites)
	if c.emitProgress != nil {
		c.emitProgress(0, total, "")
	}
	// 并发抓取（有界 8，同 CheckinAll 口径）：原串行 for 在 50 站规模要 2-3 分钟，
	// 「全站刷新」进度条龟速是最直观的差评点。失败也计入进度（进度条必须能走满）。
	var done atomic.Int64
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range sites {
		wg.Add(1)
		safego.Go("dashboard:sync-all", func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c.syncSiteStat(ctx, &sites[i], total, &done)
		})
	}
	wg.Wait()
	return c.Overview(ctx)
}

// SyncOne 立即抓取单个站点统计并写快照，返回刷新后的总览（供站点明细「刷新数据」按钮）。
func (c *DashboardChain) SyncOne(ctx context.Context, siteID uint) (*DashboardOverview, error) {
	st, err := c.siteOp.Get(siteID)
	if err != nil {
		return nil, err
	}
	ui, e := c.siteCh.UserInfo(ctx, st)
	if e != nil {
		bizmetrics.SiteSyncInc(bizmetrics.SyncFail)
		return nil, e
	}
	bizmetrics.SiteSyncInc(bizmetrics.SyncSuccess)
	c.recordStat(st, ui)
	slog.Info("dashboard synced one", "site", st.Name, "uploaded", ui.Uploaded, "ratio", ui.Ratio)
	return c.Overview(ctx)
}

// SiteHistoryPoint 是某站点一次历史统计快照（站点明细「用户历史数据」用）。
type SiteHistoryPoint struct {
	SnapAt                    time.Time `json:"snap_at"`
	Level                     string    `json:"level"`
	Uploaded                  int64     `json:"uploaded"`
	Downloaded                int64     `json:"downloaded"`
	Ratio                     float64   `json:"ratio"`
	TrueUploaded              int64     `json:"true_uploaded"`
	TrueDownloaded            int64     `json:"true_downloaded"`
	TrueRatio                 float64   `json:"true_ratio"`
	Seeding                   int       `json:"seeding"`
	Leeching                  int       `json:"leeching"`
	SeedingSize               int64     `json:"seeding_size"`
	Uploads                   int       `json:"uploads"`
	UniqueGroups              int       `json:"unique_groups"`
	PerfectFLAC               int       `json:"perfect_flac"`
	Invited                   int       `json:"invited"`
	Bonus                     float64   `json:"bonus"`
	BonusPerHour              float64   `json:"bonus_per_hour"`
	SeedingBonus              float64   `json:"seeding_bonus"`
	SeedingBonusPerHour       float64   `json:"seeding_bonus_per_hour"`
	RescueSeedingBonusPerHour float64   `json:"rescue_seeding_bonus_per_hour"`
	ClaimCount                int       `json:"claim_count"`
	MaxClaim                  int       `json:"max_claim"`
	LastAccessAt              string    `json:"last_access_at"`
}

// HistoryRowLimit 单站历史快照单次最多返回的行数。上限本身可以保留（一次拉太多卡），
// 但真实条数必须一起交出去：只回数组会让「该区间共 800 个快照」被渲染成「只有最近 500 个」，
// 历史表/曲线的起点静默缺失，用户以为站点那段时间没数据。
const HistoryRowLimit = 500

// SiteHistoryResult 单站历史快照 + 账面。
type SiteHistoryResult struct {
	Points    []SiteHistoryPoint `json:"points"`
	Days      int                `json:"days"`
	Total     int64              `json:"total"`
	Truncated bool               `json:"truncated"`
}

// History 返回某站点最近 days 天的统计快照（按时间倒序，最多 HistoryRowLimit 条）。
func (c *DashboardChain) History(ctx context.Context, siteID uint, days int) (SiteHistoryResult, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	res := SiteHistoryResult{Days: days, Points: []SiteHistoryPoint{}}
	since := time.Now().AddDate(0, 0, -days)
	if err := c.db.WithContext(ctx).Model(&models.SiteStat{}).
		Where("site_id = ? AND snap_at > ?", siteID, since).Count(&res.Total).Error; err != nil {
		return res, err
	}
	var stats []models.SiteStat
	if err := c.db.WithContext(ctx).
		Where("site_id = ? AND snap_at > ?", siteID, since).
		Order("snap_at DESC").Limit(HistoryRowLimit).
		Find(&stats).Error; err != nil {
		return res, err
	}
	res.Points = make([]SiteHistoryPoint, 0, len(stats))
	for _, s := range stats {
		res.Points = append(res.Points, SiteHistoryPoint{
			SnapAt: s.SnapAt, Level: s.Level,
			Uploaded: s.Uploaded, Downloaded: s.Downloaded, Ratio: s.Ratio,
			TrueUploaded: s.TrueUploaded, TrueDownloaded: s.TrueDownloaded, TrueRatio: s.TrueRatio,
			Seeding: s.Seeding, Leeching: s.Leeching, SeedingSize: s.SeedingSize,
			Uploads: s.Uploads, UniqueGroups: s.UniqueGroups, PerfectFLAC: s.PerfectFLAC, Invited: s.Invited, Bonus: s.Bonus, BonusPerHour: s.BonusPerHour,
			SeedingBonus: s.SeedingBonus, SeedingBonusPerHour: s.SeedingBonusPerHour, RescueSeedingBonusPerHour: s.RescueSeedingBonusPerHour,
			ClaimCount: s.ClaimCount, MaxClaim: s.MaxClaim,
			LastAccessAt: s.LastAccessAt,
		})
	}
	res.Truncated = res.Total > int64(len(res.Points))
	return res, nil
}

// Trend 返回最近 days 天的按天聚合趋势。
// uploaded/downloaded/seeding_size 是累积总量：必须先按「站×日」取 MAX（当日末值代表当日总量），
// 再跨站 SUM，否则同一天多次快照会把同一累积量重复累加。
// seeding/bonus 为瞬时值，同样走 MAX-再-SUM 取当日峰值。
// ratio 不入库聚合：Scan 后由 uploaded/downloaded 补算（当日跨站总分享率）。
func (c *DashboardChain) Trend(ctx context.Context, days int) (*DashboardTrend, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	const sql = `SELECT d AS day, SUM(uploaded) AS uploaded, SUM(downloaded) AS downloaded,
		SUM(seeding) AS seeding, SUM(seeding_size) AS seeding_size, SUM(bonus) AS bonus
		FROM (
			SELECT DATE(snap_at) AS d, site_id,
				MAX(uploaded) AS uploaded, MAX(downloaded) AS downloaded,
				MAX(seeding) AS seeding, MAX(seeding_size) AS seeding_size, MAX(bonus) AS bonus
			FROM site_stats WHERE snap_at > ?
			GROUP BY DATE(snap_at), site_id
		) AS p
		GROUP BY d ORDER BY d`
	var points []TrendPoint
	if err := c.db.WithContext(ctx).Raw(sql, since).Scan(&points).Error; err != nil {
		return nil, err
	}
	// 补算当日跨站聚合分享率（保持 SQL 稳定、跨方言兼容）。
	for i := range points {
		if points[i].Downloaded > 0 {
			points[i].Ratio = float64(points[i].Uploaded) / float64(points[i].Downloaded)
		}
	}
	return &DashboardTrend{Days: days, Points: points}, nil
}
