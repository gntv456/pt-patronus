// Package app wires all dependencies and owns the HTTP server lifecycle.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/automation"
	"github.com/hxpt/ptpatronus/internal/backup"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/filemanager"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"github.com/hxpt/ptpatronus/internal/mcp"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/ocr"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"github.com/hxpt/ptpatronus/internal/savept"
	"github.com/hxpt/ptpatronus/internal/scheduler"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/schemas"
	subauto "github.com/hxpt/ptpatronus/internal/subtitle/automation"
	"github.com/hxpt/ptpatronus/internal/transfer"
	"github.com/hxpt/ptpatronus/internal/update"
	"github.com/hxpt/ptpatronus/internal/workflow"
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

	registry, bus, base, mteam := newSiteInfra(hc)

	siteOp := oper.NewSiteOper(database, crypto)
	cfgOp := oper.NewSystemConfigOper(database)

	// 签到图形验证码 OCR（SystemConfig 热改 ocr.endpoint 即生效；未配置=回退人工提示）。
	// 与登录链路共用同一 OCR 服务配置，NexusPHP 适配器签到时自动识别填表。
	if a, ok := registry.AdapterFor("nexusphp"); ok {
		if nexus, ok := a.(*schemas.NexusPHP); ok {
			nexus.SetOCRConfig(func() ocr.Config {
				get := func(k string) string { v, _ := cfgOp.Get(k); return v }
				return ocr.Config{Endpoint: get("ocr.endpoint"), APIKey: get("ocr.api_key"), Mode: get("ocr.mode")}
			})
		}
	}
	siteCh := chain.NewSiteChain(base, siteOp, registry)
	// 注入站点 HTTP 客户端（FetchTorrentFile 下载 .torrent 用，对标 MoviePilot）。
	siteCh.SetHTTPClient(hc)
	// 签到历史落库：chain.Checkin 统一记录（cron 自动 + 手动触发均覆盖），未注入则不落库。
	checkinRecordOp := oper.NewCheckinRecordOper(database)
	siteCh.SetRecordOper(checkinRecordOp)

	// 站点限流同步：LimitInterval>0 的站点按 host 下发 per-host 最小请求间隔
	// （对标 MP 站点流控；启动时全量同步一次，站点增改后由 API 层再次同步）。
	if allSites, err := siteOp.List(); err == nil {
		siteCh.SyncHostLimits(allSites, hc)
	}

	dlOp := oper.NewDownloaderOper(database, crypto)
	dlMgr := manager.NewManager(dlOp)
	dlCh := chain.NewDownloadChain(base, dlMgr, dlOp, siteCh)

	// WebSocket 实时推送：Hub 连接池 + 广播器（随进程常驻；无连接时广播为空操作）。
	wsHub := ws.NewHub()
	wsBC := ws.NewBroadcaster(wsHub, dlOp, dlMgr, 0)
	go wsBC.Run(context.Background())

	// SystemConfig 运行时可变配置（媒体源凭据 / 通知通道凭据 / IYUU token 共用同一 KV 表，
	// 按 key 命名空间隔离）。上方已创建 cfgOp，此处供通知 provider 与媒体 provider 复用。

	// 通知：通道凭据走 SystemConfig（UI「设置→通知」热改，回退 env 引导默认），由 provider 统一构建/重建；
	// NotificationChain 与 notice.Service 共用同一 *Manager，provider rebuild→Replace 全局即时生效。
	notifMgr := notification.NewManager()
	notifDefaults := wireNotificationDefaults(cfg)
	notifProvider := chain.NewNotificationSettingsProvider(cfgOp, notifDefaults, notifMgr)
	notifCh := chain.NewNotificationChain(base, notifMgr)

	// 全局通知中心：站内收件箱（持久化）+ WS 实时推送 + 外发复用 NotificationChain。
	noticeOp := oper.NewNoticeOper(database)
	noticeSvc := notice.NewService(noticeOp, wsHub.Broadcast, notifMgr)
	wsBC.SetNotice(noticeSvc) // 下载器种子完成 diff → 通知中心

	pluginMgr := plugins.NewManager(database, bus, noticeSvc, siteOp, oper.NewPluginKVOper(database))
	pluginMgr.SetCfgOp(cfgOp)                                   // 插件市场源等 UI 持久化配置（SystemConfig）
	pluginMgr.SetMarketOper(oper.NewPluginMarketOper(database)) // 市场运营层：百鸟朝凤/收藏/分神/天序
	pluginMgr.EnsureDefaultMarketSource()                       // 首次启动种子化官方市场仓库（纯 DB 写，无网络/插件活动；安全模式也执行）
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
	mediaCh, recommendCh := wireMediaStack(cfg, cfgOp, base, pluginMgr, database)
	// 文件管理（WebDAV / Alist）
	fileOp := oper.NewFileStorageOper(database, crypto)
	fileMgr := filemanager.NewManager(fileOp)
	fileCh := chain.NewFileManagerChain(base, fileMgr)

	// 辅种（IYUU 精确匹配：iyuu 客户端复用共享 httpclient；token 走 SystemConfig）
	keepSeedCh := chain.NewKeepSeedChain(base, siteCh, iyuu.New(hc, ""), chain.NewManagerProvider(dlMgr), mteam, siteOp, cfgOp, database, noticeSvc)

	// 备份
	backupSvc := backup.New(siteOp, dlOp, registry)

	// 娱乐游戏（好学：九宫格 + 魔力刮刮乐 + 勋章合成；财神：刮刮乐）
	entReg := newEntertainmentRegistry()
	entCh := chain.NewEntertainmentChain(base, entReg, hc, siteOp)
	dashCh := chain.NewDashboardChain(database, siteOp, dlOp, siteCh)

	// PT监护室：聚合 savept.icu 站点健康数据（独立于站点登录）
	saveptSvc := savept.NewService(hc, siteOp, registry)

	// 站点考核：per-platform 适配器 Registry（MTeam JSON / NexusPHP HTML / TNode 不支持）
	assessReg := newAssessmentRegistry()
	assessOp := oper.NewAssessmentOper(database)
	assessCh := chain.NewAssessmentChain(siteCh, assessReg, assessOp, hc, noticeSvc)

	// 勋章墙：per-platform 适配器 Registry（NexusPHP HTML 双布局 / MTeam 占位；TNode/Unit3D 待 P5）
	medalReg := newMedalRegistry()
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
	// 整理链注入云盘上传能力：整理成功 → 自动上传网盘 + 生成 .strm（transfer.config.cloud_upload）。
	transferCh.SetCloudUploader(cloudCh)
	// 云端任务队列（上传/STRM/目录树导入：进度可视 + 自动重试）+ transfer 配置读取注入。
	// AppBaseURL：strm 内容渲染绝对 URL（Emby 把 strm 交给远端播放器，相对路径解析不了）。
	cloudTaskOp := oper.NewCloudTaskOper(database)
	cloudCh.SetTransferConfigGetter(cfgOp)
	cloudCh.SetAppBaseURL(cfg.AppBaseURL)
	cloudCh.SetCloudTaskOper(cloudTaskOp)
	// 云端上传任务落地 .strm 后自动扫库（整理链的 OnDownloadCompleted 复用：
	// 整理瞬间扫库时 strm 尚未生成，此处补一次，云端条目才能进媒体库）。
	transferCfg := cfgOp
	cloudCh.SetLibraryRefresher(func(name string) {
		if c := transfer.Load(transferCfg); c.RefreshLibrary {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			mediaServerCh.OnDownloadCompleted(ctx, name)
		}
	})
	// 云端整理识别：复用 MediaChain.Search（取首个结果；ParseMeta 在适配器内先洗词）。
	cloudCh.SetMediaSearcher(func(ctx context.Context, keyword string, year int, filter string) (*chain.MediaSearchItem, error) {
		res, err := mediaCh.Search(ctx, keyword, year, filter)
		if err != nil || len(res) == 0 {
			return nil, err
		}
		m := res[0]
		return &chain.MediaSearchItem{Title: m.Title, Year: m.Year, Type: string(m.Type)}, nil
	})

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
	// 媒体健康中心联动：missing_subtitle 一键补齐（复用字幕搜索/下载源）。
	mediaServerCh.SetSubtitleFixer(chain.SubtitleChainAdapter{Ch: subtitleCh})

	// 字幕自动化：监听 transfer_done → 按语言/provider 优先级自动搜字幕落盘到媒体同目录（Bazarr /
	// ChineseSubFinder 模式）。复用 subtitleCh 搜/下载（结构化实现 subauto.Searcher）；配置走 cfgOp，
	// 通知走 noticeSvc；默认关闭，用户经 /subtitle/automation/config 开启。异步 handler 不阻塞整理。
	subAutoSvc := subauto.New(bus, subtitleCh, cfgOp, noticeSvc)
	subAutoSvc.Start()

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
	// 宿主级内置工具插件（WOL 唤醒 / 豆瓣榜单订阅）：与 MCP 插件同为主机进程内注册，
	// market 生态插件不受影响（Builtin=true 不可被替换）。
	if err := pluginMgr.Register(plugins.NewWOLPlugin()); err != nil {
		slog.Warn("register wol plugin", "err", err)
	}
	if err := pluginMgr.Register(plugins.NewDoubanChartPlugin()); err != nil {
		slog.Warn("register douban chart plugin", "err", err)
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
		RegistrationCron:        cfg.RegistrationCron,
		HRWarnDeadlineHours:     cfg.HRWarnDeadlineHours,
		CheckinRecordRetainDays: cfg.CheckinRecordRetainDays,
		VIPGraceDays:            cfg.VIPGraceDays,
		KeepSeedCron:            cfg.KeepSeedCron,
		SnapshotCron:            cfg.SnapshotCron,
		CookieKeepAliveCron:     cfg.CookieKeepAliveCron,
	}, context.Background(), wsHub, hxptLogin, crypto, noticeSvc, cfgOp, assessCh)
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
	registerWorkflowNodes(wfReg, database, hc, siteOp, siteCh, filterEng, dlCh, dlOp, subCh, noticeSvc, keepSeedCh, mediaCh, recommendCh, fileCh, notifCh)

	srv, monCancel, dwCancel, tgCancel := buildHTTPServer(runtimeDeps{
		cfg:            cfg,
		database:       database,
		bus:            bus,
		hc:             hc,
		registry:       registry,
		base:           base,
		act:            act,
		assessCh:       assessCh,
		auto:           auto,
		backupSvc:      backupSvc,
		brushCh:        brushCh,
		cfgOp:          cfgOp,
		contractCh:     contractCh,
		cookieCloudCh:  cookieCloudCh,
		crypto:         crypto,
		dashCh:         dashCh,
		dlCh:           dlCh,
		dlMgr:          dlMgr,
		dlOp:           dlOp,
		entCh:          entCh,
		fileCh:         fileCh,
		fileMgr:        fileMgr,
		fileOp:         fileOp,
		filterEng:      filterEng,
		filterStore:    filterStore,
		hxpt:           hxpt,
		hxptLogin:      hxptLogin,
		j:              j,
		keepSeedCh:     keepSeedCh,
		mcpSrv:         mcpSrv,
		medalCh:        medalCh,
		mediaCh:        mediaCh,
		mediaServerCh:  mediaServerCh,
		cloudCh:        cloudCh,
		cloudTaskOp:    cloudTaskOp,
		noticeOp:       noticeOp,
		noticeSvc:      noticeSvc,
		notifCh:        notifCh,
		notifMgr:       notifMgr,
		notifProvider:  notifProvider,
		oidcSvc:        oidcSvc,
		pluginMgr:      pluginMgr,
		recommendCh:    recommendCh,
		regCh:          regCh,
		regOp:          regOp,
		rssCh:          rssCh,
		saveptSvc:      saveptSvc,
		sched:          sched,
		siteCh:         siteCh,
		siteOp:         siteOp,
		subAutoSvc:     subAutoSvc,
		subCh:          subCh,
		subtitleCh:     subtitleCh,
		traktCh:        traktCh,
		transferCh:     transferCh,
		transferHistOp: transferHistOp,
		tvCache:        tvCache,
		updateSvc:      updateSvc,
		wfOp:           wfOp,
		wfReg:          wfReg,
		wfRunOp:        wfRunOp,
		wsBC:           wsBC,
	})

	return &App{cfg: cfg, db: database, sched: sched, pluginMgr: pluginMgr, http: srv, monCancel: monCancel, dirwatchCancel: dwCancel, tgbotCancel: tgCancel}, nil
}
