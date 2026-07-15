// Package automation 实现后台自动化服务（7×24 跑在服务端）：
//   - 统计同步（真实）
//   - 签到（真实）
//   - cookie 保活（真实）：认证 GET 刷新 session + 捕获 Set-Cookie 轮换回写 + 失效告警
//   - H&R 告警（真实）：抓 H&R 列表 → 判定未达标/临期 → 通知
//   - 注册开放检测（真实）：探测 signup 页 → 关闭→开放转换时通知
//
// 对应原 Kotlin 项目的 BackgroundAutomationService，但常驻后端 Docker。
package automation

import (
	"context"
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/scheduler"
	"github.com/hxpt/ptpatronus/internal/ws"
	"gorm.io/gorm"
)

// Config 汇集自动化各任务的 cron 与阈值。空字段用默认值（见 New）。
type Config struct {
	StatsSyncCron           string // 统计同步，默认 */30 * * * *
	CookieKeepAliveCron     string // cookie 保活，默认 13 * * * *（每小时错峰，避开整点公告/半点统计的并发尖峰）
	RegistrationCron        string // 注册检测，默认 0 */6 * * *（每 6 小时）
	HRWarnDeadlineHours     int    // H&R 临期告警阈值（剩余小时），默认 24
	CheckinRecordRetainDays int    // 签到历史保留天数，默认 90（超期自动清理）
	VIPRecheckCron          string // 好学 VIP 重验，默认 0 */6 * * *（每 6 小时）
	VIPGraceDays            int    // VIP 宽限天数，默认 3

	// 站点信息聚合（刷新缓存供前端读取；站免额外 WS 推送正在站免）
	FreeleechCron    string // 默认 */30 * * * *
	AnnouncementCron string // 默认 0 * * * *（每小时）

	// 辅种定时扫描（仅扫启用 AutoReseed 的下载器；auto-add 默认关，非破坏）
	KeepSeedCron string // 默认 0 3 * * *（每天 3 点）

	// 站点私信新邮件轮询（通知中心「新邮件」来源）
	MessagesCron string // 默认 */30 * * * *
}

// Service 是后台自动化服务。
type Service struct {
	sched    *scheduler.Scheduler
	siteCh   *chain.SiteChain
	siteOp   *oper.SiteOper
	notif    *chain.NotificationChain
	regCh    *chain.RegistrationChain
	keepSeed keepSeedRunner // 定时自动辅种（nil 则跳过）
	db       *gorm.DB
	cfg      Config
	rootCtx  context.Context
	hub      *ws.Hub                // 实时推送 H&R 告警（nil 则跳过）
	hxptAuth auth.HxptAuthenticator // 好学 VIP 重验（nil 则跳过）
	crypto   *security.Crypto       // 解密 HxptCookieEnc（VIP 重验用）
	notice   *notice.Service        // 站内通知中心（站点掉线/站免/公告/新邮件；nil 跳过）
	cfgOp    *oper.SystemConfigOper // diff baseline（公告 hash / 新邮件 seen ID；nil 跳过 diff）
}

// New 构造服务。rootCtx 用于任务执行（随进程关闭取消）。
func New(sched *scheduler.Scheduler, siteCh *chain.SiteChain, siteOp *oper.SiteOper,
	notif *chain.NotificationChain, regCh *chain.RegistrationChain, keepSeed keepSeedRunner,
	db *gorm.DB, cfg Config,
	rootCtx context.Context, hub *ws.Hub, hxptAuth auth.HxptAuthenticator, crypto *security.Crypto,
	notice *notice.Service, cfgOp *oper.SystemConfigOper) *Service {

	if cfg.StatsSyncCron == "" {
		cfg.StatsSyncCron = "*/30 * * * *"
	}
	if cfg.CookieKeepAliveCron == "" {
		cfg.CookieKeepAliveCron = "13 * * * *" // 每小时：滑动续期须短于 Laravel qw_session 等 session 寿命（默认 120min），每 4h 会在两次保活间过期
	}
	if cfg.RegistrationCron == "" {
		cfg.RegistrationCron = "0 */6 * * *"
	}
	if cfg.HRWarnDeadlineHours <= 0 {
		cfg.HRWarnDeadlineHours = 24
	}
	if cfg.CheckinRecordRetainDays <= 0 {
		cfg.CheckinRecordRetainDays = 90
	}
	if cfg.VIPRecheckCron == "" {
		cfg.VIPRecheckCron = "0 */6 * * *"
	}
	if cfg.VIPGraceDays <= 0 {
		cfg.VIPGraceDays = 3
	}
	if cfg.FreeleechCron == "" {
		cfg.FreeleechCron = "*/30 * * * *"
	}
	if cfg.AnnouncementCron == "" {
		cfg.AnnouncementCron = "0 * * * *"
	}
	if cfg.KeepSeedCron == "" {
		cfg.KeepSeedCron = "0 3 * * *"
	}
	if cfg.MessagesCron == "" {
		cfg.MessagesCron = "*/30 * * * *"
	}
	return &Service{sched: sched, siteCh: siteCh, siteOp: siteOp, notif: notif, regCh: regCh,
		keepSeed: keepSeed, db: db, cfg: cfg, rootCtx: rootCtx, hub: hub, hxptAuth: hxptAuth, crypto: crypto,
		notice: notice, cfgOp: cfgOp}
}

// Start 注册并启动所有自动化任务。
func (s *Service) Start() {
	must := func(name, spec string, fn func()) {
		if err := s.sched.AddFunc(name, spec, fn); err != nil {
			slog.Error("register automation failed", "job", name, "err", err)
		}
	}
	must("stats-sync", s.cfg.StatsSyncCron, s.runStatsSync)
	must("cookie-keepalive", s.cfg.CookieKeepAliveCron, s.runCookieKeepAlive)
	must("checkin", "10 8 * * *", s.runCheckin)
	must("checkin-record-cleanup", "30 4 * * *", s.runCheckinCleanup) // 每天 4:30 清理超期签到记录
	must("hr-check", "*/30 * * * *", s.runHRCheck)
	must("registration", s.cfg.RegistrationCron, s.runRegistration)
	must("vip-recheck", s.cfg.VIPRecheckCron, s.runVIPRecheck)
	must("freeleech-check", s.cfg.FreeleechCron, s.runFreeleech)
	must("announcement-check", s.cfg.AnnouncementCron, s.runAnnouncements)
	if s.keepSeed != nil {
		must("keepseed-reseed", s.cfg.KeepSeedCron, s.runKeepSeed)
	}
	if s.notice != nil {
		must("messages-check", s.cfg.MessagesCron, s.runMessages)
	}
	s.sched.Start()
}

// runStatsSync 遍历启用站点抓取用户资料、记录统计快照。
func (s *Service) runStatsSync() {
	ctx, cancel := context.WithTimeout(s.rootCtx, 5*time.Minute)
	defer cancel()

	sites, err := s.siteOp.ListActive()
	if err != nil {
		slog.Warn("stats sync list sites failed", "err", err)
		return
	}
	for i := range sites {
		st := &sites[i]
		ui, err := s.siteCh.UserInfo(ctx, st)
		if err != nil {
			slog.Warn("stats sync fetch failed", "site", st.Name, "err", err)
			continue
		}
		s.recordStat(st, ui)
		slog.Info("stats synced", "site", st.Name, "uploaded", humanBytes(ui.Uploaded), "ratio", ui.Ratio)
	}
}

// humanBytes / recordStat 见 stats.go。
