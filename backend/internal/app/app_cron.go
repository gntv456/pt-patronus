package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/savept"
	"github.com/hxpt/ptpatronus/internal/scheduler"
)

// registerCronJobs 注册所有后台定时任务（刷流/保种/Trakt/CookieCloud/RSS/工作流清理/
// monitor 指标·告警·通知清理/PT监护室/勋章墙/媒体订阅/媒体整理/网盘 STRM）。
// 单任务失败不阻断其它任务；scheduler 自带 SkipIfStillRunning 防重叠。
func registerCronJobs(
	sched *scheduler.Scheduler,
	cfg *config.Config,
	brushCh *chain.BrushFlowChain,
	contractCh *chain.ContractChain,
	traktCh *chain.TraktChain,
	cookieCloudCh *chain.CookieCloudChain,
	rssCh *chain.RssChain,
	wfRunOp *oper.WorkflowRunOper,
	monOp *oper.MonitorOper,
	noticeOp *oper.NoticeOper,
	saveptSvc *savept.Service,
	notifCh *chain.NotificationChain,
	noticeSvc *notice.Service,
	medalCh *chain.MedalChain,
	subCh *chain.SubscribeChain,
	transferCh *chain.TransferChain,
	cloudCh *chain.CloudChain,
) {
		// 刷流 cron：选种每 10 分钟、扫删每 5 分钟（单任务失败不阻断；scheduler 自带 SkipIfStillRunning 防重叠）
		_ = sched.AddFunc("brushflow-select", "*/10 * * * *", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			brushCh.RunAllSelect(ctx)
		})

		// 刷流扫删：每 5 分钟检查删除规则并淘汰种子。
		_ = sched.AddFunc("brushflow-sweep", "*/5 * * * *", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			brushCh.RunAllSweep(ctx)
		})

		// 保种契约检查：每日 9 点（低于分享率/做种/上传下限则告警，当天去重）
		_ = sched.AddFunc("contract-check", "0 9 * * *", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			contractCh.CheckAll(ctx)
		})

		// Trakt 同步：每小时拉取一次 WatchList → 订阅（已授权后；授权/令牌刷新在链内自管）。
		_ = sched.AddFunc("trakt-sync", "7 * * * *", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if err := traktCh.SyncAll(ctx); err != nil {
				slog.Warn("trakt sync failed", "err", err)
			}
		})

		// CookieCloud 软间隔：固定每 5 分钟 tick，链内按 DB 配置的 interval + last_sync 决定是否真正同步。
		_ = sched.AddFunc("cookiecloud-tick", "*/5 * * * *", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			cookieCloudCh.MaybeSync(ctx)
		})

		// RSS 全局扫描（按各订阅 interval_min 决定是否到点抓取）。
		_ = sched.AddFunc("rss-check", cfg.RssCheckCron, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			rssCh.CheckAll(ctx)
		})

		// 每日清理 30 天前的工作流运行记录。
		_ = sched.AddFunc("workflow-run-cleanup", "0 3 * * *", func() {
			if _, err := wfRunOp.Cleanup(time.Now().Add(-30 * 24 * time.Hour)); err != nil {
				slog.Warn("workflow run cleanup failed", "err", err)
			}
		})

		// 每日 3 点清理 30 天前的指标快照。
		_ = sched.AddFunc("metric-cleanup", "0 3 * * *", func() {
			if _, err := monOp.CleanupSnapshots(time.Now().Add(-30 * 24 * time.Hour)); err != nil {
				slog.Warn("metric cleanup failed", "err", err)
			}
		})

		// 每日 3 点清理 30 天前已解决的告警（保留未解决的活跃告警）。
		_ = sched.AddFunc("alert-cleanup", "0 3 * * *", func() {
			if _, err := monOp.CleanupAlerts(time.Now().Add(-30 * 24 * time.Hour)); err != nil {
				slog.Warn("alert cleanup failed", "err", err)
			}
		})

		// 每日 3 点清理 30 天前已读通知（保留未读）。
		_ = sched.AddFunc("notice-cleanup", "0 3 * * *", func() {
			if _, err := noticeOp.Cleanup(time.Now().Add(-30 * 24 * time.Hour)); err != nil {
				slog.Warn("notice cleanup failed", "err", err)
			}
		})

		// PT监护室：定时刷新 savept.icu 全局缓存；出现病危/讣告时通知。
		_ = sched.AddFunc("savept-refresh", cfg.SaveptCron, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			res, err := saveptSvc.Fetch(ctx, true)
			if err != nil {
				slog.Warn("savept refresh failed", "err", err)
				return
			}
			for _, a := range res.Alerts {
				if a.Type == savept.AlertDanger {
					notifCh.Notify(ctx, notification.Message{
						Title: "PT监护室：" + a.Title, Body: a.Content, Level: "warning",
					})
					break
				}
			}
			// 今日站庆 / 站庆预告 → 通知中心（当天聚合去重一次）
			for _, a := range res.Alerts {
				if a.Type == savept.AlertCelebration || a.Type == savept.AlertInfo {
					_ = noticeSvc.Notice(ctx, notice.Input{
						Source: notice.SourceSiteAnniv,
						Key:    "anniv:" + time.Now().Format("2006-01-02"),
						Title:  "今日站庆：" + a.Title,
						Body:   a.Content,
						Level:  "info",
					}, true)
				}
			}
		})

		// 勋章墙：定时刷新（3 级缓存网络层 + 价格/上架/售罄 diff → 通知中心）
		_ = sched.AddFunc("medal-refresh", cfg.MedalCron, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			cards := medalCh.MedalAll(ctx, true)
			slog.Debug("medal cache refreshed", "medals", len(cards))
		})

		// 媒体订阅：定时跨站搜种子，命中即推送到默认下载器 + 通知中心提醒。
		_ = sched.AddFunc("subscribe-check", cfg.SubscribeCron, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			subCh.CheckAll(ctx)
		})

		// 媒体整理：定时兜底（catch-up/重试）——已完成种子→识别→重命名→硬链→刮削。
		// 与下载完成事件钩子互补：覆盖重启/漏触发期；幂等（按 download_hash 成功去重）。
		_ = sched.AddFunc("transfer-sweep", cfg.TransferSweepCron, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			transferCh.Sweep(ctx)
		})

		// 网盘 STRM：定时生成 STRM + 订阅追更匹配（默认每 6 小时；单规则失败不阻断）。
		_ = sched.AddFunc("cloud-strm", cfg.CloudStrmCron, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			cloudCh.GenerateAll(ctx)
			cloudCh.SyncSubscriptions(ctx)
		})
}
