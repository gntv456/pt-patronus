// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package automation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/site"
)

var (
	// checkinTotalTimeout 每日签到总预算。按「多站各自慢到单站上限」的最坏情况放宽；
	// 仍留上限防协程泄漏。cron 每日一次，耗时上限不影响其它任务。
	checkinTotalTimeout = 20 * time.Minute
	// checkinPerSiteTimeout 单站签到超时（对齐 chain.CheckinAll 的 30s）：单站挂起不得
	// 耗尽全站总预算、把后续站全部拖入 ctx 取消（当日漏签，且 cron 当日不再重试）。
	checkinPerSiteTimeout = 30 * time.Second
)

// runCheckin 遍历启用站点执行签到（attendance.php），汇总结果并通知。
// 不支持签到的站（mteam/unit3d/gazelle 等未实现 CheckinCapable）计「跳过」而非失败：
// 否则每日通知恒报「失败 N」且 N≈非 NexusPHP 站数，把真实失败淹没在噪音里。
func (s *Service) runCheckin() {
	ctx, cancel := context.WithTimeout(s.rootCtx, checkinTotalTimeout)
	defer cancel()

	sites, err := s.siteOp.ListActive()
	if err != nil {
		slog.Warn("checkin list sites failed", "err", err)
		// 读不出站点清单时一条通知都不发，通知中心里与「今天没安排签到」长得一模一样。
		if s.notif != nil {
			s.notif.Notify(s.rootCtx, notification.Message{
				Title: "签到", Body: "签到未能开始：读取站点列表失败 — " + err.Error(), Level: "warning",
			})
		}
		return
	}
	success, fail, skipped := 0, 0, 0
	for i := range sites {
		st := &sites[i]
		siteCtx, siteCancel := context.WithTimeout(ctx, checkinPerSiteTimeout)
		result, err := s.siteCh.Checkin(siteCtx, st, "cron")
		siteCancel()
		if err != nil {
			if errors.Is(err, chain.ErrNotSupported) || errors.Is(err, chain.ErrNoAdapter) {
				skipped++
				slog.Debug("checkin skipped: unsupported", "site", st.Name)
				continue
			}
			slog.Warn("checkin failed", "site", st.Name, "err", err)
			fail++
			continue
		}
		if result == nil {
			skipped++
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
	body := fmt.Sprintf("签到完成：成功 %d，失败 %d", success, fail)
	if skipped > 0 {
		body += fmt.Sprintf("，跳过 %d（不支持签到）", skipped)
	}
	// 有失败就是需要处理的事：恒报 info 会让通知列表把它和「全部签到成功」同色。
	level := "info"
	if fail > 0 {
		level = "warning"
	}
	if s.notif != nil {
		s.notif.Notify(s.rootCtx, notification.Message{Title: "签到", Body: body, Level: level})
	}
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
	// 逐字段映射统一在 chain.BuildSiteStat：此前这里与 DashboardChain.recordStat 各抄一份
	// 30 个字段，UserInfo 加一项时只补一处就静默少写一列（快照读出来是 0，趋势图当真跌）。
	if err := s.db.Create(chain.BuildSiteStat(st.ID, ui)).Error; err != nil {
		slog.Warn("record stat failed", "site", st.Name, "err", err)
	}
}
