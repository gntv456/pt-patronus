package automation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/site"
)

// runCheckin 遍历启用站点执行签到（attendance.php），汇总结果并通知。
func (s *Service) runCheckin() {
	ctx, cancel := context.WithTimeout(s.rootCtx, 5*time.Minute)
	defer cancel()

	sites, err := s.siteOp.ListActive()
	if err != nil {
		slog.Warn("checkin list sites failed", "err", err)
		return
	}
	success, fail := 0, 0
	for i := range sites {
		st := &sites[i]
		result, err := s.siteCh.Checkin(ctx, st, "cron")
		if err != nil {
			slog.Warn("checkin failed", "site", st.Name, "err", err)
			fail++
			continue
		}
		msg := result.Message
		if result.Bonus > 0 {
			msg += fmt.Sprintf(" +%g 火花", result.Bonus)
		}
		slog.Info("checkin", "site", st.Name, "result", msg)
		if result.Success {
			success++
		} else {
			fail++
		}
	}
	s.notif.Notify(s.rootCtx, notification.Message{
		Title: "签到", Body: fmt.Sprintf("签到完成：成功 %d，失败 %d", success, fail), Level: "info",
	})
}

// runCheckinCleanup 清理超期签到记录（控制表体积；保留近 CheckinRecordRetainDays 天）。
func (s *Service) runCheckinCleanup() {
	before := time.Now().AddDate(0, 0, -s.cfg.CheckinRecordRetainDays)
	res := s.db.Where("created_at < ?", before).Delete(&models.CheckinRecord{})
	if res.Error != nil {
		slog.Warn("checkin record cleanup failed", "err", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("checkin records cleaned", "deleted", res.RowsAffected, "retain_days", s.cfg.CheckinRecordRetainDays)
	}
}

// siteStatRetainDays 统计快照保留天数，与趋势图最大区间（90 天）对齐：
// 趋势查询为 snap_at > now-90d，删除 snap_at < now-90d 不会动到任何可能被渲染的点。
const siteStatRetainDays = 90

// runSiteStatCleanup 清理超期站点统计快照。每次 site-snapshot 对每站新增一行，
// 不清理则 site_stats 无限膨胀（30 站 × 6 次/天 ≈ 6.5 万行/年），拖慢仪表盘趋势/历史聚合。
func (s *Service) runSiteStatCleanup() {
	before := time.Now().AddDate(0, 0, -siteStatRetainDays)
	res := s.db.Where("snap_at < ?", before).Delete(&models.SiteStat{})
	if res.Error != nil {
		slog.Warn("site stat cleanup failed", "err", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("site stats cleaned", "deleted", res.RowsAffected, "retain_days", siteStatRetainDays)
	}
}

// recordStat 写入一条统计快照。
func (s *Service) recordStat(st *models.Site, ui *site.UserInfo) {
	stat := &models.SiteStat{
		SiteID:                    st.ID,
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
	if err := s.db.Create(stat).Error; err != nil {
		slog.Warn("record stat failed", "site", st.Name, "err", err)
	}
}
