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
	"sync"
	"time"

	bundledDefinitions "github.com/hxpt/ptpatronus/definitions"
	"github.com/hxpt/ptpatronus/internal/aiagent"
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
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/dirwatch"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/entertainment"
	"github.com/hxpt/ptpatronus/internal/filemanager"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"github.com/hxpt/ptpatronus/internal/llm"
	"github.com/hxpt/ptpatronus/internal/mcp"
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
	"github.com/hxpt/ptpatronus/internal/tgbot"
	"github.com/hxpt/ptpatronus/internal/transfer"
	"github.com/hxpt/ptpatronus/internal/update"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"github.com/hxpt/ptpatronus/internal/workflow/nodes"
	"github.com/hxpt/ptpatronus/internal/ws"
	"gorm.io/gorm"
)

// App is the running application.
type App struct {
	cfg            *config.Config
	db             *gorm.DB
	sched          *scheduler.Scheduler
	pluginMgr      *plugins.Manager
	http           *http.Server
	monCancel      context.CancelFunc // 停监控采集 goroutine
	dirwatchCancel context.CancelFunc // 停目录监视 goroutine（C 阶段填，nil=safe-mode/未启用）
	tgbotCancel    context.CancelFunc // 停 Telegram bot goroutine（A 阶段填，nil=safe-mode/未启用）
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

	// 站点抓取客户端：开重试以容忍 Cloudflare/网络偶发的 connection reset by peer
	// （实测标准 TLS 能正常连通 CF 站点，reset 多为瞬态，重试即可恢复）。
	hc, err := httpclient.New(httpclient.Options{FollowRedirect: true, MaxRetries: 2})
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
	// Flarum 论坛社区（invites.fun / pting.club 等 PT 论坛，JSON:API + cookie + 签到）
	flarum := schemas.NewFlarum(hc)
	registry.RegisterAdapter(flarum)
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
	mm.Register(flarum)
	for _, gj := range genericJSONs {
		mm.Register(gj)
	}
	bus := event.New()
	base := chain.NewBase(mm, bus)

	siteOp := oper.NewSiteOper(database, crypto)
	siteCh := chain.NewSiteChain(base, siteOp, registry)
	// 注入站点 HTTP 客户端（FetchTorrentFile 下载 .torrent 用，对标 MoviePilot）。
	siteCh.SetHTTPClient(hc)
	// 签到历史落库：chain.Checkin 统一记录（cron 自动 + 手动触发均覆盖），未注入则不落库。
	checkinRecordOp := oper.NewCheckinRecordOper(database)
	siteCh.SetRecordOper(checkinRecordOp)

	dlOp := oper.NewDownloaderOper(database, crypto)
	dlMgr := manager.NewManager(dlOp)
	dlCh := chain.NewDownloadChain(base, dlMgr, dlOp, siteCh)

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

	pluginMgr := plugins.NewManager(database, bus, noticeSvc, siteOp, oper.NewPluginKVOper(database))
	pluginMgr.SetCfgOp(cfgOp) // 插件市场源等 UI 持久化配置（SystemConfig）
	pluginMgr.EnsureDefaultMarketSource() // 首次启动种子化官方市场仓库（纯 DB 写，无网络/插件活动；安全模式也执行）
	if !cfg.SafeMode {
		if err := pluginMgr.LoadExternal(filepath.Join(cfg.DataDir, "plugins")); err != nil {
			slog.Warn("load external plugins failed", "err", err)
		}
		if cfg.PluginDevWatch {
			pluginMgr.EnableDevWatcher(time.Duration(cfg.PluginDevWatchIntervalMS) * time.Millisecond)
		}
		pluginMgr.Start(context.Background())
	} else {
		slog.Warn("SAFE_MODE 已启用：跳过插件/调度/监控激活（核心 HTTP + 诊断可达）")
	}

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
		OMDbAPIKey:      cfg.OMDbAPIKey,
	})
	posterCache := oper.NewPosterCacheOper(database)
	mediaCh := chain.NewMediaChain(base, mediaProvider, posterCache, cfgOp)
	// 把 MediaChain 暴露给插件的 Host API（/media/search），刮削类插件复用宿主元数据源。
	pluginMgr.SetMediaSearcher(mediaSearchAdapter{mediaCh})
	// 推荐信息流：复用同一份 provider（豆瓣热映 / TMDB trending / Bangumi 热门）
	recommendCh := chain.NewRecommendChain(base, mediaProvider)

	// 文件管理（WebDAV / Alist）
	fileOp := oper.NewFileStorageOper(database, crypto)
	fileMgr := filemanager.NewManager(fileOp)
	fileCh := chain.NewFileManagerChain(base, fileMgr)

	// 辅种（IYUU 精确匹配：iyuu 客户端复用共享 httpclient；token 走 SystemConfig）
	keepSeedCh := chain.NewKeepSeedChain(base, siteCh, iyuu.New(hc, ""), chain.NewManagerProvider(dlMgr), mteam, siteOp, cfgOp, database, noticeSvc)

	// 备份
	backupSvc := backup.New(siteOp, dlOp, registry)

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
	// mediaCh 注入多评分低分保护闸门（订阅 EnableRatingGuard 或全局 MediaQualityConfig.Enabled）。
	subOp := oper.NewSubscriptionOper(database)
	// 资源过滤规则引擎（对标 MP-2 filter module）：规则/组存 SystemConfig KV；引擎无状态。
	// 订阅/刷流/RSS/工作流/Agent 共用，破除 workflow/nodes 复制 matchResolution 的循环 import 根因。
	filterStore := filter.NewStore(cfgOp)
	filterEng := filter.NewEngine(filterStore)

	subCh := chain.NewSubscribeChain(siteCh, dlCh, dlOp, subOp, noticeSvc, mediaCh, filterEng, cfgOp)

	hxpt := auth.NewHxptVIP(registry, "hxpt", cfg.AuthSiteURL, cfg.AuthVIPLevel, hc)

	// OIDC/SSO 服务（Authelia/Keycloak/Casdoor 等标准 OIDC；凭据落 SystemConfig auth.oidc）。
	oidcSvc := auth.NewOIDCService(cfgOp)

	// 好学登录服务：编排账号登录两步 + VIP 校验；每条登录用独立临时 client（独立 jar、不跟随重定向）
	loginClientFactory := func() (site.LoginHTTPDoer, error) {
		return httpclient.New(httpclient.Options{Timeout: 30 * time.Second, FollowRedirect: false})
	}
	hxptLogin := auth.NewHxptLogin(registry, "hxpt", cfg.AuthSiteURL, hxpt, loginClientFactory)
	// TokenVersion 缓存（登出失效；当前按单实例设计）
	tvCache := auth.NewTokenVersionCache(database, 60*time.Second)

	// 刷流引擎：按规则选种→加入下载器→监控做种→达删除规则自动删种（tag 识别；cron 驱动选种+扫删）
	brushOp := oper.NewBrushFlowOper(database)
	brushCh := chain.NewBrushFlowChain(base, siteOp, siteCh, dlCh, brushOp, noticeSvc, filterEng)

	// 保种契约：为站点设分享率/做种数/上传量下限，cron 定时检查 UserInfo 低于阈值则告警
	contractOp := oper.NewContractOper(database)
	contractCh := chain.NewContractChain(base, siteCh, siteOp, contractOp, noticeSvc)

	// 媒体服务器（Emby/JF/Plex）：CRUD + 库扫描/刷新 + 封面生成 + 下载完成触发刷新
	mediaServerOp := oper.NewMediaServerOper(database, crypto)
	mediaServerCh, err := chain.NewMediaServerChain(base, mediaServerOp, noticeSvc)
	if err != nil {
		return nil, err
	}
	// 下载完成 → 触发开启 RefreshOnDownload 的服务器扫库（best-effort，单服务器失败不阻断）。
	wsBC.AddDownloadDoneHook(func(ctx context.Context, dlID uint, name, hash string) {
		mediaServerCh.OnDownloadCompleted(ctx, name)
	})

	// 媒体整理：下载完成 → 识别 → 重命名 → 硬链/移动 → 刮削 NFO/海报 → 写历史 + 通知 → 扫库。
	// 复用 mediaCh(识别)/dlMgr+dlOp(下载器)/mediaServerCh(扫库)/noticeSvc(通知)；hc 取海报图。
	transferHistOp := oper.NewTransferHistoryOper(database)
	transferLinkOp := oper.NewDownloadMediaLinkOper(database)
	imageFetcher := func(ctx context.Context, url string) ([]byte, error) {
		resp, err := hc.Get(ctx, url, "", true) // ssrf=true：海报来自 TMDB/豆瓣 等白名单域
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	transferCh := chain.NewTransferChain(base, mediaCh, dlMgr, dlOp, transferHistOp, transferLinkOp, cfgOp, mediaServerCh, noticeSvc, imageFetcher)
	// 下载完成 → 第二个副作用钩子：媒体整理（与媒体库刷新并存，fan-out，互不阻断）。
	wsBC.AddDownloadDoneHook(func(ctx context.Context, dlID uint, name, hash string) {
		transferCh.OnDownloadCompleted(ctx, dlID, name, hash)
	})

	// 网盘 + STRM（115/夸克/123）：CRUD + 浏览/配额 + STRM 生成 + 订阅追更（复用 subCh 匹配订阅）。
	cloudDriveOp := oper.NewCloudDriveOper(database, crypto)
	strmRuleOp := oper.NewStrmRuleOper(database)
	cloudCh, err := chain.NewCloudChain(base, cloudDriveOp, strmRuleOp, subCh, noticeSvc)
	if err != nil {
		return nil, err
	}

	// 字幕搜索/下载：多源（ASSRT/OpenSubtitles API + SubHD/Zimuku 抓取）聚合 + 解 zip + 繁简。
	// 源凭据走 SystemConfig（回退 env ASSRT_TOKEN/OPENSUBTITLES_KEY），UI 改动即时 rebuild。
	subtitleProvider, err := chain.NewSubtitleSourceProvider(cfgOp, chain.SubtitleSettings{
		AssrtToken:       cfg.AssrtToken,
		OpenSubtitlesKey: cfg.OpenSubtitlesKey,
	})
	if err != nil {
		return nil, err
	}
	subtitleCh := chain.NewSubtitleChain(base, subtitleProvider)

	// MCP 服务端：把宿主能力（站点/订阅/下载器/搜索）暴露为 MCP 工具，供 AI Agent / IDE 经
	// JSON-RPC 调用。配置（启用/令牌）由 ptp-mcp-server 插件承载，服务端每次请求经
	// SettingsGetter 读取插件配置，单一真源、热改即时生效。公开路由 /api/v1/mcp 在 router 注册。
	mcpSrv := mcp.NewServer()
	registerHostMCPTools(mcpSrv, siteOp, siteCh, subCh, dlOp)
	mcpSrv.SetSettingsGetter(func() mcp.Settings {
		if v, err := pluginMgr.Get(plugins.MCPPluginID); err == nil {
			return plugins.ParseMCPSettings(v.Config)
		}
		return mcp.Settings{}
	})
	if err := pluginMgr.Register(plugins.NewMCPServerPlugin(mcpSrv)); err != nil {
		slog.Warn("register mcp plugin", "err", err)
	}

	// Trakt 同步：WatchList → 本地订阅（device OAuth 授权 + 定时同步）。复用 subCh 建订阅、
	// cfgOp 存凭据、noticeSvc 推送同步结果。独立 API /trakt/* + 三端 Trakt 页。
	traktCh := chain.NewTraktChain(subCh, cfgOp, noticeSvc)

	// CookieCloud 同步：浏览器扩展把最新 cookie 推到服务器，本端定时拉取并按域名覆盖本地失效站 cookie。
	// 设置存 SystemConfig（密码 AES-GCM 加密）；独立 API /cookiecloud/* + 三端设置页。tick 软间隔（读 DB interval）。
	cookieCloudCh := chain.NewCookieCloudChain(cfgOp, crypto, siteOp, registry, siteCh, hc)

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
	// 刷流 cron：选种每 10 分钟、扫删每 5 分钟（单任务失败不阻断；scheduler 自带 SkipIfStillRunning 防重叠）
	_ = sched.AddFunc("brushflow-select", "*/10 * * * *", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		brushCh.RunAllSelect(ctx)
	})
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
	if !cfg.SafeMode {
		auto.Start()
	}

	// RSS 订阅 + 工作流引擎（依赖 siteCh/dlCh/subCh/keepSeedCh/noticeSvc/siteOp/dlOp/hc/database，均已构造）。
	rssOp := oper.NewRssOper(database)
	rssItemOp := oper.NewRssItemOper(database)
	rssCh := chain.NewRssChain(base, rssOp, rssItemOp, dlCh, dlOp, siteOp, noticeSvc, hc, filterEng)

	wfOp := oper.NewWorkflowOper(database)
	wfRunOp := oper.NewWorkflowRunOper(database)
	wfReg := workflow.NewRegistry()
	wfReg.Register(nodes.NewFetchRSSNode(hc, siteOp))
	wfReg.Register(nodes.NewSearchTorrentsNode(siteCh))
	wfReg.Register(nodes.NewFetchSiteStatsNode(database))
	wfReg.Register(nodes.NewFilterTorrentsNode())
	wfReg.Register(nodes.NewLimitNode())
	wfReg.Register(nodes.NewFilterRulesNode(filterEng))
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

	// monitor 设备监控（服务器 host / NAS / 摄像头 camera）。先构造服务，供工作流运维节点复用 SSH 执行能力。
	monOp := oper.NewMonitorOper(database, crypto)
	sshKeyOp := oper.NewSSHHostKeyOper(database) // SSH host key TOFU 存储（满足 monitor.HostKeyStore）
	monReg := monitor.NewRegistry()
	localCollector := monitor.NewHostCollector()
	monReg.Register("host", "local", localCollector)
	monReg.Register("host", "ssh", monitor.NewSSHCollector(sshKeyOp))
	monReg.Register("nas", "local", localCollector)
	monReg.Register("nas", "api", monitor.NewNASCollector(hc))
	monReg.Register("nas", "snmp", monitor.NewNASCollector(hc))
	monReg.Register("camera", "onvif", monitor.NewCameraCollector())
	monReg.Register("camera", "rtsp", monitor.NewCameraCollector())
	monReg.Register("http", "http", monitor.NewHTTPCollector())
	monReg.Register("tcp", "tcp", monitor.NewTCPCollector())
	monReg.Register("process", "ssh", monitor.NewSSHProcessCollector(sshKeyOp))
	monReg.Register("script", "ssh", monitor.NewSSHScriptCollector(sshKeyOp))
	// 告警通知适配到现有 NotificationChain（bark/tg/webhook 多通道扇出）。
	monNotifier := monitor.Notifier(func(ctx context.Context, title, body, level string) {
		notifCh.Notify(ctx, notification.Message{Title: title, Body: body, Level: level})
	})
	monSvc := monitor.NewService(monOp, monReg, monNotifier, 10*time.Second, noticeSvc)
	wfReg.Register(nodes.NewSSHCommandNode(monSvc))

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

	// monitor 运行时：RTSP→WS 转发、Agent 推送、后台采集与清理任务。
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
	if !cfg.SafeMode {
		go monSvc.Run(monCtx)
	}
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
	// 媒体整理：定时兜底（catch-up/重试）——已完成种子→识别→重命名→硬链→刮削。
	// 与下载完成事件钩子互补：覆盖重启/漏触发期；幂等（按 download_hash 成功去重）。
	sched.AddFunc("transfer-sweep", cfg.TransferSweepCron, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		transferCh.Sweep(ctx)
	})
	// 网盘 STRM：定时生成 STRM + 订阅追更匹配（默认每 6 小时；单规则失败不阻断）。
	sched.AddFunc("cloud-strm", cfg.CloudStrmCron, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		cloudCh.GenerateAll(ctx)
		cloudCh.SyncSubscriptions(ctx)
	})

	// AI Agent（ReAct 循环 + 双 LLM provider + 工具调用 + 多轮上下文）。
	// 工具集绑定各业务 chain（读+写，admin-only）；会话/消息持久化；配置+API key 走 SystemConfig+Crypto。
	agentConvOp := oper.NewAgentConversationOper(database)
	agentMsgOp := oper.NewAgentMessageOper(database)
	agentTools := registerAgentTools(siteOp, siteCh, subCh, dlOp, dlCh, mediaCh, recommendCh, transferCh, keepSeedCh, dashCh, notifCh, filterStore, mediaServerCh, cloudCh, brushCh, rssCh, subtitleCh, fileCh, wfCh)
	// 把全部 chain 能力批量暴露给插件 Host API（GET /tools、POST /tools/{name}、Bridge tool.*）。
	// 必须在所有 chain 构造之后调用（闭包捕获 chain 指针）。
	registerPluginHostTools(pluginMgr, agentTools, dlCh, subCh, mediaServerCh, cloudCh, brushCh, rssCh, subtitleCh, transferCh, keepSeedCh, fileCh, wfCh)
	agentSvc := aiagent.New(llm.New, agentTools, agentMsgOp, agentConvOp, cfgOp, crypto)
	agentChatH := v1.NewAgentChatHandler(agentSvc, agentConvOp.List, agentConvOp.Create, agentConvOp.Delete, agentMsgOp.ListByConv, cfgOp, crypto)
	llmGatewayH := v1.NewLLMGatewayHandler(agentSvc, agentConvOp.Create, cfgOp, crypto)
	doctorH := v1.NewDoctorHandler(cfg, database, nil)
	tgBotH := v1.NewTgBotHandler(cfgOp, crypto)
	// Servarr 兼容（Sonarr/Radarr v3）：订阅 ↔ Movie/Series，hasFile 查整理历史。无外部依赖，safe-mode 下仍注册。
	servarrH := v1.NewServarrHandler(subCh, mediaCh, transferHistOp.HasSuccessByMedia, cfg.APIKey)

	// 目录监视：fsnotify 实时喂整理（补 DoneHook + cron 覆盖不到的手动投放 / 非托管来源）。safe-mode 下不启。
	var dwCancel context.CancelFunc
	if !cfg.SafeMode {
		if dwc := dirwatch.Load(cfgOp); dwc.Enabled && len(dwc.Dirs) > 0 {
			dw := dirwatch.New(dwc, func(ctx context.Context, srcDir string, files []transfer.FileInput) {
				transferCh.ProcessDiscoveredFiles(ctx, srcDir, files)
			})
			dwCtx, cancel := context.WithCancel(context.Background())
			dwCancel = cancel
			go dw.Run(dwCtx)
		}
	}

	// Telegram bot（入站 chat-ops）：getUpdates 长轮询 → 自然语言经 AI Agent 驱动全平台。safe-mode 下不启。
	var tgCancel context.CancelFunc
	if !cfg.SafeMode {
		if tgc := tgbot.Load(cfgOp); tgc.Enabled && tgc.BotTokenEnc != "" {
			if token := tgbot.BotToken(tgc, crypto); token != "" {
				var chatMu sync.Mutex
				chatConvs := map[string]uint{}
				talk := func(ctx context.Context, chatKey, text string) <-chan llm.Event {
					chatMu.Lock()
					convID, ok := chatConvs[chatKey]
					if !ok {
						conv := &models.AgentConversation{}
						if err := agentConvOp.Create(conv); err == nil && conv.ID != 0 {
							convID = conv.ID
							chatConvs[chatKey] = convID
						}
					}
					chatMu.Unlock()
					if convID == 0 {
						ch := make(chan llm.Event)
						close(ch)
						return ch
					}
					return agentSvc.Run(ctx, convID, text)
				}
				clear := func(chatKey string) { chatMu.Lock(); delete(chatConvs, chatKey); chatMu.Unlock() }
				bot := tgbot.New(token, tgc.APIURL, tgc.AllowedUserIDs, tgc.AllowGroupChat, talk, clear)
				tgCtx, cancel := context.WithCancel(context.Background())
				tgCancel = cancel
				go bot.Run(tgCtx)
			}
		}
	}

	engine := api.Build(&api.Deps{
		Cfg: cfg, DB: database, JWT: j, Act: act,
		HxptVIP: hxpt, HxptAuth: hxptLogin, Crypto: crypto, TVCache: tvCache, VIPGraceDays: cfg.VIPGraceDays,
		Tickets: ticketStore,
		OIDC:    oidcSvc,
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
		Subscribe:   subCh,
		Workflow:    wfCh,
		Rss:         rssCh,
		BrushFlow:   brushCh,
		Contract:    contractCh,
		MediaServer: mediaServerCh,
		Transfer:    transferCh,
		AgentChat:   agentChatH,
		LLMGateway:  llmGatewayH,
		Doctor:      doctorH,
		TgBot:       tgBotH,
		Servarr:     servarrH,
		Cloud:       cloudCh,
		Subtitle:    subtitleCh,
		Trakt:       traktCh,
		CookieCloud: cookieCloudCh,
		MCP:         mcpSrv,
		Webhook:     webhookH,
		MonOp:       monOp, MonSvc: monSvc, RTSPProxy: rtspProxy,
		SSHHostKeyOp: sshKeyOp,
		AgentOp:      agentOp, AgentHub: agentHub,
		Savept:     saveptSvc,
		Assessment: assessCh,
		Medal:      medalCh,
		Notice:     noticeSvc,
		Update:     updateSvc,
		Filter:     filterStore,
		FilterEng:  filterEng,
		Plugins:    pluginMgr,
	})

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: engine}
	return &App{cfg: cfg, db: database, sched: sched, pluginMgr: pluginMgr, http: srv, monCancel: monCancel, dirwatchCancel: dwCancel, tgbotCancel: tgCancel}, nil
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
	if a.dirwatchCancel != nil {
		a.dirwatchCancel()
	}
	if a.tgbotCancel != nil {
		a.tgbotCancel()
	}
	if a.sched != nil {
		a.sched.Stop()
	}
	if a.pluginMgr != nil {
		a.pluginMgr.Stop()
	}
	return a.http.Shutdown(ctx)
}
