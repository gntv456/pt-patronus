package chain

import (
	"context"
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// SiteStatItem 是某站点的最新统计。
type SiteStatItem struct {
	SiteID       uint      `json:"site_id"`
	SiteName     string    `json:"site_name"`
	UserName     string    `json:"username"` // 站点用户名（导出海报身份行，best-effort）
	UserID       string    `json:"user_id"`   // 站点用户 ID
	Email        string    `json:"email"`     // 账号邮箱（best-effort）
	Level        string    `json:"level"`
	Uploaded     int64     `json:"uploaded"`
	Downloaded   int64     `json:"downloaded"`
	Ratio        float64   `json:"ratio"`
	Seeding      int       `json:"seeding"`
	Leeching     int       `json:"leeching"`
	SeedingSize  int64     `json:"seeding_size"`
	Uploads      int       `json:"uploads"`
	Bonus        float64   `json:"bonus"`
	BonusPerHour float64   `json:"bonus_per_hour"`
	JoinDate     string    `json:"join_date"`
	UpdatedAt    time.Time `json:"updated_at"` // 该统计的抓取时间（SiteStat.SnapAt）
}

// DashboardOverview 是仪表盘聚合数据。
type DashboardOverview struct {
	Totals struct {
		Uploaded     int64   `json:"uploaded"`
		Downloaded   int64   `json:"downloaded"`
		Seeding      int     `json:"seeding"`
		Leeching     int     `json:"leeching"`
		SeedingSize  int64   `json:"seeding_size"`
		Bonus        float64 `json:"bonus"`
		BonusPerHour float64 `json:"bonus_per_hour"`
	} `json:"totals"`
	Sites           []SiteStatItem `json:"sites"`
	SiteCount       int            `json:"site_count"`
	DownloaderCount int            `json:"downloader_count"`
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

	out := &DashboardOverview{}
	out.DownloaderCount = len(dls)

	// 单 SQL 取每站最新统计（MAX(id) GROUP BY site_id），避免 N+1（原每站一次 First +
	// ORDER BY filesort，100 站 = 100 次查询）。id 自增 ⇔ 时间递增，MAX(id) 即最新一条。
	var latest []models.SiteStat
	c.db.Where("id IN (SELECT MAX(id) FROM site_stats GROUP BY site_id)").Find(&latest)
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
			item.Seeding = stat.Seeding
			item.Leeching = stat.Leeching
			item.SeedingSize = stat.SeedingSize
			item.Uploads = stat.Uploads
			item.Bonus = stat.Bonus
			item.BonusPerHour = stat.BonusPerHour
			item.JoinDate = stat.JoinDate
			item.UpdatedAt = stat.SnapAt
		}
		out.Sites = append(out.Sites, item)
		out.Totals.Uploaded += item.Uploaded
		out.Totals.Downloaded += item.Downloaded
		out.Totals.Seeding += item.Seeding
		out.Totals.Leeching += item.Leeching
		out.Totals.SeedingSize += item.SeedingSize
		out.Totals.Bonus += item.Bonus
		out.Totals.BonusPerHour += item.BonusPerHour
	}
	out.SiteCount = len(sites)
	return out, nil
}

// recordStat 写入一条统计快照。
// 与 automation.Service.recordStat 等价；chain 层独立实现以避免 import 循环
// （automation 依赖 chain，故 chain 不能反向依赖 automation）。
func (c *DashboardChain) recordStat(st *models.Site, ui *site.UserInfo) {
	stat := &models.SiteStat{
		SiteID:       st.ID,
		Level:        ui.Level,
		UserName:     ui.Name,
		UserID:       ui.ID,
		Email:        ui.Email,
		Uploaded:     ui.Uploaded,
		Downloaded:   ui.Downloaded,
		Ratio:        ui.Ratio,
		Seeding:      ui.Seeding,
		Leeching:     ui.Leeching,
		SeedingSize:  ui.SeedingSize,
		Uploads:      ui.Uploads,
		Bonus:        ui.Bonus,
		BonusPerHour: ui.BonusPerHour,
		JoinDate:     ui.JoinDate,
		SnapAt:       time.Now(),
	}
	if err := c.db.Create(stat).Error; err != nil {
		slog.Warn("dashboard record stat failed", "site", st.Name, "err", err)
	}
}

// SyncAll 立即抓取所有启用站点的统计并写入快照，然后返回刷新后的总览。
// 用于「加入站点后数据全为 0」的即时回填——不必等 stats-sync 定时任务（默认 30 分钟）。
func (c *DashboardChain) SyncAll(ctx context.Context) (*DashboardOverview, error) {
	sites, err := c.siteOp.ListActive()
	if err != nil {
		return nil, err
	}
	for i := range sites {
		st := &sites[i]
		ui, e := c.siteCh.UserInfo(ctx, st)
		if e != nil {
			slog.Warn("dashboard sync site failed", "site", st.Name, "err", e)
			continue
		}
		c.recordStat(st, ui)
		slog.Info("dashboard synced", "site", st.Name, "uploaded", ui.Uploaded, "ratio", ui.Ratio)
	}
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
		return nil, e
	}
	c.recordStat(st, ui)
	slog.Info("dashboard synced one", "site", st.Name, "uploaded", ui.Uploaded, "ratio", ui.Ratio)
	return c.Overview(ctx)
}

// SiteHistoryPoint 是某站点一次历史统计快照（站点明细「用户历史数据」用）。
type SiteHistoryPoint struct {
	SnapAt       time.Time `json:"snap_at"`
	Level        string    `json:"level"`
	Uploaded     int64     `json:"uploaded"`
	Downloaded   int64     `json:"downloaded"`
	Ratio        float64   `json:"ratio"`
	Seeding      int       `json:"seeding"`
	Leeching     int       `json:"leeching"`
	SeedingSize  int64     `json:"seeding_size"`
	Uploads      int       `json:"uploads"`
	Bonus        float64   `json:"bonus"`
	BonusPerHour float64   `json:"bonus_per_hour"`
}

// History 返回某站点最近 days 天的统计快照（按时间倒序，最多 500 条）。
func (c *DashboardChain) History(ctx context.Context, siteID uint, days int) ([]SiteHistoryPoint, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)
	var stats []models.SiteStat
	if err := c.db.WithContext(ctx).
		Where("site_id = ? AND snap_at > ?", siteID, since).
		Order("snap_at DESC").Limit(500).
		Find(&stats).Error; err != nil {
		return nil, err
	}
	out := make([]SiteHistoryPoint, 0, len(stats))
	for _, s := range stats {
		out = append(out, SiteHistoryPoint{
			SnapAt: s.SnapAt, Level: s.Level,
			Uploaded: s.Uploaded, Downloaded: s.Downloaded, Ratio: s.Ratio,
			Seeding: s.Seeding, Leeching: s.Leeching, SeedingSize: s.SeedingSize,
			Uploads: s.Uploads, Bonus: s.Bonus, BonusPerHour: s.BonusPerHour,
		})
	}
	return out, nil
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
