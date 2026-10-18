// 站点统计快照的行构造与落库。
//
// 这里曾有两条各自逐字段抄写的映射：automation.Service.recordStat（定时聚合）与
// DashboardChain.recordStat（仪表盘即时同步）。两处必须等价，而字段有 30 个——
// UserInfo 新增一项时只补一处，另一处就静默少写一列（快照读出来是 0，趋势图当真跌）。
// 统一到 BuildSiteStat，两个调用点只剩落库差异（chain 不能反向依赖 automation）。
package chain

import (
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// BuildSiteStat 把一次 UserInfo 采样映射成 site_stats 行（SnapAt 取当前时间）。
func BuildSiteStat(siteID uint, ui *site.UserInfo) *models.SiteStat {
	return &models.SiteStat{
		SiteID:                    siteID,
		Level:                     ui.Level,
		UserName:                  ui.Name,
		UserID:                    ui.ID,
		Email:                     ui.Email,
		Uploaded:                  ui.Uploaded,
		Downloaded:                ui.Downloaded,
		Ratio:                     ui.Ratio,
		TrueUploaded:              ui.TrueUploaded,
		TrueDownloaded:            ui.TrueDownloaded,
		TrueRatio:                 ui.TrueRatio,
		Seeding:                   ui.Seeding,
		Leeching:                  ui.Leeching,
		SeedingSize:               ui.SeedingSize,
		Uploads:                   ui.Uploads,
		UniqueGroups:              ui.UniqueGroups,
		PerfectFLAC:               ui.PerfectFLAC,
		Invited:                   ui.Invited,
		Bonus:                     ui.Bonus,
		BonusPerHour:              ui.BonusPerHour,
		SeedingBonus:              ui.SeedingBonus,
		SeedingBonusPerHour:       ui.SeedingBonusPerHour,
		RescueSeedingBonusPerHour: ui.RescueSeedingBonusPerHour,
		ClaimCount:                ui.ClaimCount,
		MaxClaim:                  ui.MaxClaim,
		JoinDate:                  ui.JoinDate,
		LastAccessAt:              ui.LastAccessAt,
		SnapAt:                    time.Now(),
	}
}

// recordStat 写入一条统计快照（仪表盘即时同步路径）。
func (c *DashboardChain) recordStat(st *models.Site, ui *site.UserInfo) {
	if err := c.db.Create(BuildSiteStat(st.ID, ui)).Error; err != nil {
		slog.Warn("dashboard record stat failed", "site", st.Name, "err", err)
	}
}
