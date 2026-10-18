// Package automation 实现后台自动化服务（7×24 跑在服务端）：
//   - site-snapshot 聚合快照（真实）：每 4h 串行抓 stats / 站免 / 公告 / 消息 / cookie 保活 / 考核；另有小时级 cookie-keepalive，
//     复用 UserInfo 的 uid 访问 userdetails.php?id=<uid>（避免裸访触发站点风控）；降低站点感知的
//     请求频率，替代原 stats/hr/freeleech/announcement/messages/cookie-keepalive 6 个分散 cron。
//   - 签到（真实）
//   - 注册开放检测（真实）：探测 signup 页 → 关闭→开放转换时通知
//   - 好学 VIP 重验 / IYUU 辅种（真实）
//
// 对应原 Kotlin 项目的 BackgroundAutomationService，但常驻后端 Docker。
package automation

import (
	"context"
	"log/slog"

	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/notification"
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

	// 站点私信新邮件轮询（通知中心「新邮件」来源）——已并入 SnapshotCron，保留字段向后兼容
	MessagesCron string // deprecated：并入 SnapshotCron

	// 聚合站点快照（合并 stats/hr/freeleech/announcement/messages/cookie-keepalive/assessment
	// 7 个原任务为一次 4h 串行抓取，降低站点感知的请求频率），默认 7 */4 * * * *
	SnapshotCron string
}

// Service 是后台自动化服务。
type Service struct {
	sched    *scheduler.Scheduler
	siteCh   *chain.SiteChain
	siteOp   *oper.SiteOper
	notif    notifier
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
	assessCh *chain.AssessmentChain // 聚合快照复用 uid 跑考核（nil 则跳过 assessment）
	// hxcardPush 守护神名片自动推送（site-snapshot 末尾触发；nil 跳过。闭包内已含开关/密钥判断）
	hxcardPush func(ctx context.Context)
	hrGapDay   string // H&R 漏检自曝的当日去重标记（进程内，重启后当天再提示一次可接受）
}

// SetHxCardPush 注入守护神名片自动推送闭包（app 装配时；nil 安全）。
func (s *Service) SetHxCardPush(fn func(ctx context.Context)) {
	s.hxcardPush = fn
}

// New 构造服务。rootCtx 用于任务执行（随进程关闭取消）。
func New(sched *scheduler.Scheduler, siteCh *chain.SiteChain, siteOp *oper.SiteOper,
	notif notifier, regCh *chain.RegistrationChain, keepSeed keepSeedRunner,
	db *gorm.DB, cfg Config,
	rootCtx context.Context, hub *ws.Hub, hxptAuth auth.HxptAuthenticator, crypto *security.Crypto,
	notice *notice.Service, cfgOp *oper.SystemConfigOper, assessCh *chain.AssessmentChain) *Service {

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
	if cfg.SnapshotCron == "" {
		cfg.SnapshotCron = "7 */4 * * *" // 每 4h 错峰 7 分：降频与 session 保活的平衡
	}
	return &Service{sched: sched, siteCh: siteCh, siteOp: siteOp, notif: notif, regCh: regCh,
		keepSeed: keepSeed, db: db, cfg: cfg, rootCtx: rootCtx, hub: hub, hxptAuth: hxptAuth, crypto: crypto,
		notice: notice, cfgOp: cfgOp, assessCh: assessCh}
}

// Start 注册并启动所有自动化任务。
func (s *Service) Start() {
	// DB 配置覆盖 env 默认（前端「调度频率」改的值优先于环境变量）。
	if s.cfgOp != nil {
		if v, _ := s.cfgOp.Get("schedule.site-snapshot"); v != "" {
			s.cfg.SnapshotCron = v
		}
		if v, _ := s.cfgOp.Get("schedule.cookie-keepalive"); v != "" {
			s.cfg.CookieKeepAliveCron = v
		}
	}
	must := func(name, spec string, fn func()) {
		if err := s.sched.AddFunc(name, spec, fn); err != nil {
			slog.Error("register automation failed", "job", name, "err", err)
		}
	}
	must("site-snapshot", s.cfg.SnapshotCron, s.runSnapshot)                  // 聚合 stats/free/ann/msg/keepalive/assessment（H&R 独立）
	must("cookie-keepalive", s.cfg.CookieKeepAliveCron, s.runCookieKeepAlive) // 小时级 session 滑动续期（短于 Laravel 默认 120min）
	must("hr-check", "23 * * * *", s.runHRCheck)                              // H&R 每小时独立（封号命脉时效优先；错峰 :23）
	must("checkin", "10 8 * * *", s.runCheckin)
	must("checkin-record-cleanup", "30 4 * * *", s.runCheckinCleanup) // 每天 4:30 清理超期签到记录
	must("site-stat-cleanup", "40 4 * * *", s.runSiteStatCleanup)     // 每天 4:40 清理超期统计快照（错峰签到清理）
	must("registration", s.cfg.RegistrationCron, s.runRegistration)
	must("vip-recheck", s.cfg.VIPRecheckCron, s.runVIPRecheck)
	if s.keepSeed != nil {
		must("keepseed-reseed", s.cfg.KeepSeedCron, s.runKeepSeed)
	}
	s.sched.Start()
}

// runStatsSync 已并入 site-snapshot 聚合任务（dispatchSnapshotSite → recordStat）。
// humanBytes / recordStat 见 stats.go。

// notifier 是自动化域要用的通知能力（链层 NotificationChain 天然满足）。
// 抽成接口是为了让失败出声这条判据可测：此前 runKeepSeed 的两处失败出口只写日志，
// 而「读设置失败」那处会通知——形状不一致，且测试无法验证后两处（#352）。
type notifier interface {
	Notify(ctx context.Context, msg notification.Message) []notification.SendResult
}
