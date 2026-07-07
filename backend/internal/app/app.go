// Package app wires all dependencies and owns the HTTP server lifecycle.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	bundledDefinitions "github.com/hxpt/ptpatronus/definitions"
	"github.com/hxpt/ptpatronus/internal/api"
	"github.com/hxpt/ptpatronus/internal/api/v1"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/automation"
	"github.com/hxpt/ptpatronus/internal/backup"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/entertainment"
	"github.com/hxpt/ptpatronus/internal/filemanager"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"github.com/hxpt/ptpatronus/internal/savept"
	"github.com/hxpt/ptpatronus/internal/scheduler"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/assessment"
	"github.com/hxpt/ptpatronus/internal/site/medalwall"
	"github.com/hxpt/ptpatronus/internal/site/schemas"
	"github.com/hxpt/ptpatronus/internal/update"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"github.com/hxpt/ptpatronus/internal/workflow/nodes"
	"github.com/hxpt/ptpatronus/internal/ws"
	"gorm.io/gorm"
)

// App is the running application.
type App struct {
	cfg       *config.Config
	db        *gorm.DB
	sched     *scheduler.Scheduler
	http      *http.Server
	monCancel context.CancelFunc // 停监控采集 goroutine
}

// New wires all dependencies and builds the HTTP server (does not start it).
func New(cfg *config.Config) (*App, error) {
	database, err := db.Open(cfg)
	if err != nil {
		return nil, err
	}
	crypto := security.NewCrypto(cfg.SecretKey)
	j := security.NewJWT(cfg.SecretKey, cfg.JWTExpireMin)
	act := auth.NewActivation(database, cfg.ActivationSecret)

	// 站点抓取客户端
	hc, err := httpclient.New(httpclient.Options{FollowRedirect: true})
	if err != nil {
		return nil, err
	}

	registry := site.NewRegistry()
	defsDir := os.Getenv("DEFINITIONS_DIR")
	defsDirExplicit := defsDir != ""
	if defsDir == "" {
		defsDir = defaultDefinitionsDir()
	}
	loadSiteDefinitions(registry, defsDir, defsDirExplicit)
	nexus := schemas.NewNexusPHP(hc)
	registry.RegisterAdapter(nexus)
	mteam := schemas.NewMTeam(hc)
	registry.RegisterAdapter(mteam)
	unit3d := schemas.NewUnit3D(hc)
	registry.RegisterAdapter(unit3d)
	gazelle := schemas.NewGazelle(hc)
	registry.RegisterAdapter(gazelle)
	luminance := schemas.NewLuminance(hc)
	registry.RegisterAdapter(luminance)
	gazelleJSONAPI := schemas.NewGazelleJSONAPI(hc)
	registry.RegisterAdapter(gazelleJSONAPI)
	// Generic HTML（兜底，多 schema 实例）：解锁 "" + 命名 HTML schema 的选择器驱动站。
	genericHTMLs := make([]*schemas.GenericHTML, 0, 12)
	for _, sc := range []string{"", "xbtit", "tcg", "tbsource", "filelist", "discuz", "rartracker", "cgbtsource", "meantorrent", "aidoruonline", "smallhorse"} {
		g := schemas.NewGenericHTML(hc, sc)
		registry.RegisterAdapter(g)
		genericHTMLs = append(genericHTMLs, g)
	}
	// Avistaz 网络（JWT token API，4 站）
	avistaz := schemas.NewAvistaz(hc)
	registry.RegisterAdapter(avistaz)
	// Rousi Pro（Bearer passkey API）
	rousi := schemas.NewRousi(hc)
	registry.RegisterAdapter(rousi)
	// Generic JSON（命名 JSON schema）
	genericJSONs := make([]*schemas.GenericJSON, 0, 5)
	for _, sc := range []string{"tnode", "yemapt", "f3nix", "tbdev", "fsm"} {
		gj := schemas.NewGenericJSON(hc, sc)
		registry.RegisterAdapter(gj)
		genericJSONs = append(genericJSONs, gj)
	}

	mm := module.New()
	mm.Register(nexus)
	mm.Register(mteam)
	mm.Register(unit3d)
	mm.Register(gazelle)
	mm.Register(luminance)
	mm.Register(gazelleJSONAPI)
	for _, g := range genericHTMLs {
		mm.Register(g)
	}
	mm.Register(avistaz)
	mm.Register(rousi)
	for _, gj := range genericJSONs {
		mm.Register(gj)
	}
	bus := event.New()
	base := chain.NewBase(mm, bus)

	siteOp := oper.NewSiteOper(database, crypto)
	siteCh := chain.NewSiteChain(base, siteOp, registry)
	// 签到历史落库：chain.Checkin 统一记录（cron 自动 + 手动触发均覆盖），未注入则不落库。
	checkinRecordOp := oper.NewCheckinRecordOper(database)
	siteCh.SetRecordOper(checkinRecordOp)

	dlOp := oper.NewDownloaderOper(database, crypto)
	dlMgr := manager.NewManager(dlOp)
	dlCh := chain.NewDownloadChain(base, dlMgr, dlOp)

	// WebSocket 实时推送：Hub 连接池 + 广播器（随进程常驻；无连接时广播为空操作）。
	wsHub := ws.NewHub()
	wsBC := ws.NewBroadcaster(wsHub, dlOp, dlMgr, 0)
	go wsBC.Run(context.Background())

	// SystemConfig 运行时可变配置（媒体源凭据 / 通知通道凭据 / IYUU token 共用同一 KV 表，
	// 按 key 命名空间隔离）。此处统一创建，供下方通知 provider 与媒体 provider 复用。
	cfgOp := oper.NewSystemConfigOper(database)

	// 通知：通道凭据走 SystemConfig（UI「设置→通知」热改，回退 env 引导默认），由 provider 统一构建/重建；
	// NotificationChain 与 notice.Service 共用同一 *Manager，provider rebuild→Replace 全局即时生效。
	notifMgr := notification.NewManager()
	notifDefaults := chain.NotificationSettings{}
	notifDefaults.Bark.Enabled = cfg.BarkKey != ""
	notifDefaults.Bark.Server = cfg.BarkServer
	notifDefaults.Bark.Key = cfg.BarkKey
	notifDefaults.Telegram.Enabled = cfg.TGToken != "" && cfg.TGChatID != ""
	notifDefaults.Telegram.Token = cfg.TGToken
	notifDefaults.Telegram.ChatID = cfg.TGChatID
	notifDefaults.Telegram.APIURL = cfg.TGAPIURL
	notifDefaults.QQ.Enabled = cfg.QQAppID != "" && cfg.QQAppSecret != ""
	notifDefaults.QQ.AppID = cfg.QQAppID
	notifDefaults.QQ.AppSecret = cfg.QQAppSecret
	notifDefaults.QQ.OpenID = cfg.QQOpenID
	notifDefaults.QQ.GroupOpenID = cfg.QQGroupOpenID
	notifDefaults.WeChat.Enabled = cfg.WXClawBotToken != ""
	notifDefaults.WeChat.BotToken = cfg.WXClawBotToken
	notifDefaults.WeChat.AccountID = cfg.WXClawBotAccountID
	notifDefaults.WeChat.Target = cfg.WXClawBotTarget
	notifDefaults.WeChat.BaseURL = cfg.WXClawBotBaseURL
	notifDefaults.Webhook.Enabled = cfg.WebhookURL != ""
	notifDefaults.Webhook.URL = cfg.WebhookURL
	notifProvider := chain.NewNotificationSettingsProvider(cfgOp, notifDefaults, notifMgr)
	notifCh := chain.NewNotificationChain(base, notifMgr)

	// 全局通知中心：站内收件箱（持久化）+ WS 实时推送 + 外发复用 NotificationChain。
	noticeOp := oper.NewNoticeOper(database)
	noticeSvc := notice.NewService(noticeOp, wsHub.Broadcast, notifMgr)
	wsBC.SetNotice(noticeSvc) // 下载器种子完成 diff → 通知中心

	pluginMgr := plugins.NewManager(database, bus, noticeSvc)
	if err := pluginMgr.LoadExternal(filepath.Join(cfg.DataDir, "plugins")); err != nil {
		slog.Warn("load external plugins failed", "err", err)
	}
	pluginMgr.Start(context.Background())

	// 在线更新：拉官方版本清单(UPDATE_MANIFEST_URL)与当前版本比较，供 Web/客户端检查；
	// 管理员可写自托管清单(data/update-manifest.json)覆盖官方源。
	updateSvc := update.NewService(cfg, hc)

	// 媒体元数据（TMDB 主源 + 豆瓣 + Bangumi + MusicBrainz）。
	// 凭据走 SystemConfig（UI 可热改，回退环境变量默认），由 provider 统一构建/重建，
	// 搜索(MediaChain)与推荐(RecommendChain)共用，UI 改 key/域名/镜像即时生效。
	mediaProvider := chain.NewMediaSourceProvider(cfgOp, chain.MediaSettings{
		TMDBAPIKey:      cfg.TMDBAPIKey,
		TMDBAPIDomain:   cfg.TMDBAPIDomain,
		DoubanSecret:    cfg.DoubanSecret,
		DoubanAppKey:    cfg.DoubanAppKey,
		BangumiToken:    cfg.BangumiToken,
		XingguangjiBase: cfg.XingguangjiBase,
	})
	posterCache := oper.NewPosterCacheOper(database)
	mediaCh := chain.NewMediaChain(base, mediaProvider, posterCache)
	// 推荐信息流：复用同一份 provider（豆瓣热映 / TMDB trending / Bangumi 热门）
	recommendCh := chain.NewRecommendChain(base, mediaProvider)

	// 文件管理（WebDAV / Alist）
	fileOp := oper.NewFileStorageOper(database, crypto)
	fileMgr := filemanager.NewManager(fileOp)
	fileCh := chain.NewFileManagerChain(base, fileMgr)

	// 辅种（IYUU 精确匹配：iyuu 客户端复用共享 httpclient；token 走 SystemConfig）
	keepSeedCh := chain.NewKeepSeedChain(base, siteCh, iyuu.New(hc, ""), chain.NewManagerProvider(dlMgr), mteam, siteOp, cfgOp, database, noticeSvc)

	// 备份
	backupSvc := backup.New(siteOp, dlOp)

	// 娱乐游戏（好学：九宫格 + 魔力刮刮乐 + 勋章合成；财神：刮刮乐）
	entReg := entertainment.NewRegistry()
	entReg.Register("hxpt.org", entertainment.JggGame{})
	entReg.Register("hxpt.org", entertainment.MagicScratchGame{})
	entReg.Register("hxpt.org", entertainment.MedalSynthesisGame{})
	for _, h := range []string{"cspt.top", "cspt.cc"} {
		entReg.Register(h, entertainment.CsptScratchGame{})
	}
	// LongPT：幸运大转盘 + 每日抽奖 + 六十四卦勋章（/pt-api/v1/lucky|medals|nexus/*，纯 Cookie 鉴权）
	entReg.Register("longpt.org", entertainment.LongPTWheelGame{})
	entReg.Register("longpt.org", entertainment.LongPTDailyGame{})
	entReg.Register("longpt.org", entertainment.LongPTMedalGame{})
	// 魔力农场（通用 magic_fram.php 插件，好学 hxpt + NovaHD 共用；纯 Cookie，货币=seedbonus）
	entReg.Register("hxpt.org", entertainment.MagicFarmGame{})
	entReg.Register("novahd.top", entertainment.MagicFarmGame{})
	// NovaHD：魔力值刮刮乐 + 幸运抽奖（magic_scratch.php / fortune-wheel-spin.php，纯 Cookie 鉴权）
	entReg.Register("novahd.top", entertainment.NovaHDScratchGame{})
	entReg.Register("novahd.top", entertainment.NovaHDLuckyGame{})
	// 多站幸运转盘/老虎机/抽奖（批1，纯 Cookie）：CBG(藏宝阁) 转盘+老虎机、UB 转盘、PANDA+蟹黄堡 LuckyDraw 插件、March 转盘、不可躺抽奖
	entReg.Register("cangbao.ge", entertainment.CbgWheelGame{})
	entReg.Register("cangbao.ge", entertainment.CbgSlotGame{})
	entReg.Register("ubits.club", entertainment.UbWheelGame{})
	entReg.Register("pandapt.net", entertainment.LuckyDrawWheelGame{})
	entReg.Register("crabpt.vip", entertainment.LuckyDrawWheelGame{})
	entReg.Register("duckboobee.org", entertainment.MarchWheelGame{})
	entReg.Register("www.tangpt.top", entertainment.TangptLotteryGame{})
	// 下注/选号（批2，Bettable）：猜大小/骰宝/双色球/翻面/赛事押注/红包
	entReg.Register("hxpt.org", entertainment.HxptBigSmallGame{})
	entReg.Register("pandapt.net", entertainment.PandaBigSmallGame{})
	entReg.Register("cangbao.ge", entertainment.CbgSicboGame{})
	entReg.Register("crabpt.vip", entertainment.CrabUnionLottoGame{})
	entReg.Register("pandapt.net", entertainment.PandaFlipGame{})
	entReg.Register("ourbits.club", entertainment.OurbitsBetGame{})
	entReg.Register("ubits.club", entertainment.UbitsRedpacketGame{})
	// 状态机/红包雨（批3）：二十一点 + 好学红包雨
	entReg.Register("audiences.me", entertainment.AudiencesBlackjackGame{})
	entReg.Register("hxpt.org", entertainment.HxptRedpacketGame{})
	entCh := chain.NewEntertainmentChain(base, entReg, hc, siteOp)
	dashCh := chain.NewDashboardChain(database, siteOp, dlOp, siteCh)

	// PT监护室：聚合 savept.icu 站点健康数据（独立于站点登录）
	saveptSvc := savept.NewService(hc, siteOp)

	// 站点考核：per-platform 适配器 Registry（MTeam JSON / NexusPHP HTML / TNode 不支持）
	assessReg := assessment.NewRegistry()
	assessReg.Register(assessment.NexusPHPAdapter{})
	assessReg.Register(assessment.MTeamAdapter{})
	assessReg.Register(assessment.TNodeAdapter{})
	assessOp := oper.NewAssessmentOper(database)
	assessCh := chain.NewAssessmentChain(siteCh, assessReg, assessOp, hc, noticeSvc)

	// 勋章墙：per-platform 适配器 Registry（NexusPHP HTML 双布局 / MTeam 占位；TNode/Unit3D 待 P5）
	medalReg := medalwall.NewRegistry()
	medalReg.Register(medalwall.NexusPHPAdapter{})
	medalReg.Register(medalwall.MTeamAdapter{})
	medalReg.Register(medalwall.TNodeAdapter{})
	medalReg.Register(medalwall.Unit3DAdapter{})
	medalOp := oper.NewMedalOper(database)
	medalCh := chain.NewMedalChain(siteCh, medalReg, medalOp, hc, noticeSvc)

	regOp := oper.NewRegistrationOper(database)
	regCh := chain.NewRegistrationChain(base, hc, regOp)

	// 媒体订阅：cron 跨站搜种子→推送默认下载器+通知。复用 siteCh(搜)、dlCh/dlOp(下载)、noticeSvc(通知)。
	subOp := oper.NewSubscriptionOper(database)
	subCh := chain.NewSubscribeChain(siteCh, dlCh, dlOp, subOp, noticeSvc)

	hxpt := auth.NewHxptVIP(registry, "hxpt", cfg.AuthSiteURL, cfg.AuthVIPLevel, hc)

	// 好学登录服务：编排账号登录两步 + VIP 校验；每条登录用独立临时 client（独立 jar、不跟随重定向）
	loginClientFactory := func() (site.LoginHTTPDoer, error) {
		return httpclient.New(httpclient.Options{Timeout: 30 * time.Second, FollowRedirect: false})
	}
	hxptLogin := auth.NewHxptLogin(registry, "hxpt", cfg.AuthSiteURL, hxpt, loginClientFactory)
	// TokenVersion 缓存（登出失效；当前按单实例设计）
	tvCache := auth.NewTokenVersionCache(database, 60*time.Second)

	// 后台自动化（7×24）
	sched := scheduler.New()
	auto := automation.New(sched, siteCh, siteOp, notifCh, regCh, keepSeedCh, database, automation.Config{
		StatsSyncCron:           cfg.StatsSyncCron,
		CookieKeepAliveCron:     cfg.CookieKeepAliveCron,
		RegistrationCron:        cfg.RegistrationCron,
		HRWarnDeadlineHours:     cfg.HRWarnDeadlineHours,
		CheckinRecordRetainDays: cfg.CheckinRecordRetainDays,
		VIPGraceDays:            cfg.VIPGraceDays,
		FreeleechCron:           cfg.FreeleechCron,
		AnnouncementCron:        cfg.AnnouncementCron,
		KeepSeedCron:            cfg.KeepSeedCron,
		MessagesCron:            cfg.MessagesCron,
	}, context.Background(), wsHub, hxptLogin, crypto, noticeSvc, cfgOp)
	auto.Start()

	// RSS 订阅 + 工作流引擎（依赖 siteCh/dlCh/subCh/keepSeedCh/noticeSvc/siteOp/dlOp/hc/database，均已构造）。
	rssOp := oper.NewRssOper(database)
	rssItemOp := oper.NewRssItemOper(database)
	rssCh := chain.NewRssChain(base, rssOp, rssItemOp, dlCh, dlOp, siteOp, noticeSvc, hc)

	wfOp := oper.NewWorkflowOper(database)
	wfRunOp := oper.NewWorkflowRunOper(database)
	wfReg := workflow.NewRegistry()
	wfReg.Register(nodes.NewFetchRSSNode(hc, siteOp))
	wfReg.Register(nodes.NewSearchTorrentsNode(siteCh))
	wfReg.Register(nodes.NewFetchSiteStatsNode(database))
	wfReg.Register(nodes.NewFilterTorrentsNode())
	wfReg.Register(nodes.NewLimitNode())
	wfReg.Register(nodes.NewAddDownloadNode(dlCh, dlOp))
	wfReg.Register(nodes.NewAddSubscribeNode(subCh))
	wfReg.Register(nodes.NewSendMessageNode(noticeSvc))
	wfReg.Register(nodes.NewHTTPRequestNode(hc))
	wfReg.Register(nodes.NewExtractFieldsNode())
	wfReg.Register(nodes.NewCheckinNode(siteCh, siteOp))
	wfReg.Register(nodes.NewReseedNode(keepSeedCh))
	wfReg.Register(nodes.NewConditionNode())
	wfReg.Register(nodes.NewSwitchNode())
	wfReg.Register(nodes.NewWebhookSendNode(hc))
	wfReg.Register(nodes.NewMathNode())
	wfReg.Register(nodes.NewDelayNode())
	wfReg.Register(nodes.NewNoteNode())
	wfReg.Register(nodes.NewSetVariableNode())
	// 下载管理（封装 DownloadChain 进阶操作）
	wfReg.Register(nodes.NewListTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewPauseTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewResumeTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewDeleteTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewRecheckTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewReannounceTorrentsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewForceStartNode(dlCh, dlOp))
	wfReg.Register(nodes.NewSetCategoryNode(dlCh, dlOp))
	wfReg.Register(nodes.NewAddTagsNode(dlCh, dlOp))
	wfReg.Register(nodes.NewSetSavePathNode(dlCh, dlOp))
	wfReg.Register(nodes.NewSetSpeedLimitNode(dlCh, dlOp))
	wfReg.Register(nodes.NewSetTorrentLimitNode(dlCh, dlOp))
	wfReg.Register(nodes.NewSetShareLimitNode(dlCh, dlOp))
	wfReg.Register(nodes.NewAddTrackerNode(dlCh, dlOp))
	// 站点业务（封装 SiteChain）
	wfReg.Register(nodes.NewCheckinAllNode(siteCh, siteOp))
	wfReg.Register(nodes.NewFetchFreeleechNode(siteCh, siteOp))
	wfReg.Register(nodes.NewFetchAnnouncementNode(siteCh, siteOp))
	wfReg.Register(nodes.NewFetchMessagesNode(siteCh, siteOp))
	wfReg.Register(nodes.NewCheckHRNode(siteCh, siteOp))
	wfReg.Register(nodes.NewKeepAliveNode(siteCh, siteOp))
	wfReg.Register(nodes.NewSiteUserinfoNode(siteCh, siteOp))
	wfReg.Register(nodes.NewSiteTestNode(siteCh, siteOp))
	wfReg.Register(nodes.NewHaremStatusNode(siteCh, siteOp))
	// 资源处理
	wfReg.Register(nodes.NewSearchAllSitesNode(siteCh))
	wfReg.Register(nodes.NewTorrentDetailNode(siteCh, siteOp))
	wfReg.Register(nodes.NewSortTorrentsNode())
	wfReg.Register(nodes.NewDedupeTorrentsNode())
	wfReg.Register(nodes.NewPickBestNode())
	// 媒体（MediaChain / RecommendChain）
	wfReg.Register(nodes.NewSearchMediaNode(mediaCh))
	wfReg.Register(nodes.NewMediaDetailNode(mediaCh))
	wfReg.Register(nodes.NewRecommendFeedNode(recommendCh))
	// 文件（FileManagerChain）
	wfReg.Register(nodes.NewListFilesNode(fileCh))
	wfReg.Register(nodes.NewDeleteFileNode(fileCh))
	wfReg.Register(nodes.NewMkdirNode(fileCh))
	// 数据
	wfReg.Register(nodes.NewParseJSONNode())
	wfReg.Register(nodes.NewRegexExtractNode())
	// 通知（直发外发渠道）
	wfReg.Register(nodes.NewSendNotificationNode(notifCh))
	// 流程
	wfReg.Register(nodes.NewStopFlowNode())
	wfReg.Register(nodes.NewRandomPickNode())
	wfReg.Register(nodes.NewFormatSizeNode())
	wfCh := chain.NewWorkflowChain(base, wfOp, wfRunOp, wfReg, sched, base.Bus, noticeSvc)
	// 进站 Webhook：外部 POST /api/v1/webhooks/in/:token → 触发目标工作流 + 广播 "webhook" 事件 + 通知。
	webhookOp := oper.NewWebhookOper(database, crypto)
	webhookH := v1.NewWebhookHandler(webhookOp, wfCh, bus, noticeSvc)
	// 启动恢复：进程崩溃可能留下 state=running 的工作流（下次触发被重入守卫挡死），先重置为 failed。
	if err := wfOp.ResetStuckRunning(); err != nil {
		slog.Warn("workflow stuck-running reset failed", "err", err)
	}
	// 启动时重建工作流触发器（cron 任务随进程重启丢失；event 监听惰性查 Enabled 自废）。
	if err := wfCh.RegisterAllTriggers(context.Background()); err != nil {
		slog.Warn("workflow trigger registration failed", "err", err)
	}
	// RSS 全局扫描（按各订阅 interval_min 决定是否到点抓取）。
	sched.AddFunc("rss-check", cfg.RssCheckCron, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		rssCh.CheckAll(ctx)
	})
	// 每日清理 30 天前的工作流运行记录。
	sched.AddFunc("workflow-run-cleanup", "0 3 * * *", func() {
		if _, err := wfRunOp.Cleanup(time.Now().Add(-30 * 24 * time.Hour)); err != nil {
			slog.Warn("workflow run cleanup failed", "err", err)
		}
	})

	// monitor 设备监控（服务器 host / NAS / 摄像头 camera）+ RTSP→WS 转发。
	monOp := oper.NewMonitorOper(database, crypto)
	sshKeyOp := oper.NewSSHHostKeyOper(database) // SSH host key TOFU 存储（满足 monitor.HostKeyStore）
	monReg := monitor.NewRegistry()
	monReg.Register("host", "local", monitor.NewHostCollector())
	monReg.Register("host", "ssh", monitor.NewSSHCollector(sshKeyOp))
	monReg.Register("nas", "api", monitor.NewNASCollector(hc))
	monReg.Register("nas", "snmp", monitor.NewNASCollector(hc))
	monReg.Register("camera", "onvif", monitor.NewCameraCollector())
	monReg.Register("camera", "rtsp", monitor.NewCameraCollector())
	// 告警通知适配到现有 NotificationChain（bark/tg/webhook 多通道扇出）。
	monNotifier := monitor.Notifier(func(ctx context.Context, title, body, level string) {
		notifCh.Notify(ctx, notification.Message{Title: title, Body: body, Level: level})
	})
	monSvc := monitor.NewService(monOp, monReg, monNotifier, 10*time.Second, noticeSvc)
	var monitorOrigins []string
	for _, o := range strings.Split(cfg.CORSAllowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			monitorOrigins = append(monitorOrigins, o)
		}
	}
	ticketStore := auth.NewTicketStore() // WS/RTSP 一次性 ticket（替代 query JWT）
	rtspProxy := monitor.NewRTSPProxy(j, ticketStore, tvCache, monitorOrigins, 8, func(id uint) (string, error) {
		// url 由服务端按 :id 从 DB 取（防 RTSP 代理 SSRF/流劫持），禁止客户端传入
		t, err := monOp.Get(id)
		if err != nil {
			return "", err
		}
		return t.RTSPURL, nil
	})
	// Agent push 架构：agentOp 实现 AgentAuthenticator 接口（Authenticate+TouchSeen）；
	// ingest = monSvc.IngestAgentReport（复用 pull 链路的 RecordSnapshot + Evaluate）。
	agentOp := oper.NewAgentOper(database)
	agentHub := monitor.NewAgentWSHub(agentOp, monSvc.IngestAgentReport, 0)
	fileMgr.SetAgentHub(agentHub) // agent 文件管理后端依赖 hub；fileMgr 早于 agentHub 构建，此处后注入
	monCtx, monCancel := context.WithCancel(context.Background())
	go monSvc.Run(monCtx)
	// 每日 3 点清理 30 天前的指标快照。
	sched.AddFunc("metric-cleanup", "0 3 * * *", func() {
		if _, err := monOp.CleanupSnapshots(time.Now().Add(-30 * 24 * time.Hour)); err != nil {
			slog.Warn("metric cleanup failed", "err", err)
		}
	})
	// 每日 3 点清理 30 天前已解决的告警（保留未解决的活跃告警）。
	sched.AddFunc("alert-cleanup", "0 3 * * *", func() {
		if _, err := monOp.CleanupAlerts(time.Now().Add(-30 * 24 * time.Hour)); err != nil {
			slog.Warn("alert cleanup failed", "err", err)
		}
	})
	// 每日 3 点清理 30 天前已读通知（保留未读）。
	sched.AddFunc("notice-cleanup", "0 3 * * *", func() {
		if _, err := noticeOp.Cleanup(time.Now().Add(-30 * 24 * time.Hour)); err != nil {
			slog.Warn("notice cleanup failed", "err", err)
		}
	})
	// PT监护室：定时刷新 savept.icu 全局缓存；出现病危/讣告时通知。
	sched.AddFunc("savept-refresh", cfg.SaveptCron, func() {
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
				noticeSvc.Notice(ctx, notice.Input{
					Source: notice.SourceSiteAnniv,
					Key:    "anniv:" + time.Now().Format("2006-01-02"),
					Title:  "今日站庆：" + a.Title,
					Body:   a.Content,
					Level:  "info",
				}, true)
			}
		}
	})
	// 站点考核：定时刷新缓存（3 级缓存的网络层；前端读取秒返回）
	sched.AddFunc("assessment-refresh", cfg.AssessmentCron, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cards := assessCh.AssessAll(ctx, true)
		slog.Debug("assessment cache refreshed", "sites", len(cards))
	})
	// 勋章墙：定时刷新（3 级缓存网络层 + 价格/上架/售罄 diff → 通知中心）
	sched.AddFunc("medal-refresh", cfg.MedalCron, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cards := medalCh.MedalAll(ctx, true)
		slog.Debug("medal cache refreshed", "medals", len(cards))
	})
	// 媒体订阅：定时跨站搜种子，命中即推送到默认下载器 + 通知中心提醒。
	sched.AddFunc("subscribe-check", cfg.SubscribeCron, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		subCh.CheckAll(ctx)
	})

	engine := api.Build(&api.Deps{
		Cfg: cfg, DB: database, JWT: j, Act: act,
		HxptVIP: hxpt, HxptAuth: hxptLogin, Crypto: crypto, TVCache: tvCache, VIPGraceDays: cfg.VIPGraceDays,
		Tickets: ticketStore,
		SiteOp:  siteOp, SiteCh: siteCh, Reg: registry, SiteHC: hc,
		DLOp: dlOp, DLCh: dlCh, DLMgr: dlMgr,
		Notif: notifCh, NotifMgr: notifMgr, NotifProvider: notifProvider,
		MediaCh: mediaCh, FileOp: fileOp, FileCh: fileCh, FileMgr: fileMgr,
		Recommend: recommendCh,
		KeepSeed:  keepSeedCh, Backup: backupSvc,
		Entertainment: entCh,
		Dashboard:     dashCh,
		WS:            wsBC,
		RegOp:         regOp, RegCh: regCh,
		Subscribe: subCh,
		Workflow:  wfCh,
		Rss:       rssCh,
		Webhook:   webhookH,
		MonOp:     monOp, MonSvc: monSvc, RTSPProxy: rtspProxy,
		SSHHostKeyOp: sshKeyOp,
		AgentOp:      agentOp, AgentHub: agentHub,
		Savept:     saveptSvc,
		Assessment: assessCh,
		Medal:      medalCh,
		Notice:     noticeSvc,
		Update:     updateSvc,
		Plugins:    pluginMgr,
	})

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: engine}
	return &App{cfg: cfg, db: database, sched: sched, http: srv, monCancel: monCancel}, nil
}

// Run starts the HTTP server (blocking).
func (a *App) Run() error { return a.http.ListenAndServe() }

func defaultDefinitionsDir() string {
	for _, dir := range []string{"./definitions", "./backend/definitions"} {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return "./definitions"
}

func loadSiteDefinitions(registry *site.Registry, defsDir string, externalExplicit bool) {
	if err := registry.LoadDefinitionsFS(bundledDefinitions.FS, "."); err != nil {
		slog.Warn("load bundled site definitions failed", "err", err)
	} else {
		slog.Info("loaded bundled site definitions", "count", len(registry.Definitions()))
	}

	if strings.TrimSpace(defsDir) == "" {
		return
	}
	if err := registry.LoadDefinitions(defsDir); err != nil {
		if !externalExplicit && errors.Is(err, os.ErrNotExist) {
			slog.Info("external site definitions directory not found; using bundled definitions", "dir", defsDir, "count", len(registry.Definitions()))
			return
		}
		slog.Warn("load external site definitions failed; using bundled definitions", "dir", defsDir, "err", err)
		return
	}
	slog.Info("loaded external site definitions", "dir", defsDir, "total", len(registry.Definitions()))
}

// Shutdown gracefully stops scheduler + monitor loop + HTTP server.
func (a *App) Shutdown(ctx context.Context) error {
	if a.monCancel != nil {
		a.monCancel()
	}
	if a.sched != nil {
		a.sched.Stop()
	}
	return a.http.Shutdown(ctx)
}
