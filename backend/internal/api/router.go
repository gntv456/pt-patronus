// Package api wires the Gin engine and mounts route groups.
package api

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/backup"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/filemanager"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"github.com/hxpt/ptpatronus/internal/savept"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/update"
	"github.com/hxpt/ptpatronus/internal/web"
	"github.com/hxpt/ptpatronus/internal/ws"
	"gorm.io/gorm"
)

// Deps 聚合路由装配所需的全部依赖。
type Deps struct {
	Cfg           *config.Config
	DB            *gorm.DB
	JWT           *security.JWT
	Act           *auth.Activation
	HxptVIP       *auth.HxptVIP
	HxptAuth      auth.HxptAuthenticator
	Crypto        *security.Crypto
	TVCache       *auth.TokenVersionCache
	Tickets       *auth.TicketStore
	VIPGraceDays  int
	SiteOp        *oper.SiteOper
	SiteCh        *chain.SiteChain
	Reg           *site.Registry
	SiteHC        *httpclient.Client
	DLOp          *oper.DownloaderOper
	DLCh          *chain.DownloadChain
	DLMgr         *manager.Manager
	Notif         *chain.NotificationChain
	NotifMgr      *notification.Manager
	NotifProvider *chain.NotificationSettingsProvider
	MediaCh       *chain.MediaChain
	Recommend     *chain.RecommendChain
	FileOp        *oper.FileStorageOper
	FileCh        *chain.FileManagerChain
	FileMgr       *filemanager.Manager
	KeepSeed      *chain.KeepSeedChain
	Backup        *backup.Service
	Entertainment *chain.EntertainmentChain
	Dashboard     *chain.DashboardChain
	WS            *ws.Broadcaster
	RegOp         *oper.RegistrationOper
	RegCh         *chain.RegistrationChain
	Subscribe     *chain.SubscribeChain
	Workflow      *chain.WorkflowChain
	Rss           *chain.RssChain
	BrushFlow     *chain.BrushFlowChain
	Contract      *chain.ContractChain
	Webhook       *v1.WebhookHandler
	MonOp         *oper.MonitorOper
	MonSvc        *monitor.Service
	RTSPProxy     *monitor.RTSPProxy
	SSHHostKeyOp  *oper.SSHHostKeyOper
	AgentOp       *oper.AgentOper
	AgentHub      *monitor.AgentWSHub
	Savept        *savept.Service
	Assessment    *chain.AssessmentChain
	Medal         *chain.MedalChain
	Notice        *notice.Service
	Update        *update.Service
	Plugins       *plugins.Manager
}

// splitOrigins 把逗号分隔的 CORS 配置拆为来源切片（空切片表示开发全开）。
func splitOrigins(s string) []string {
	var out []string
	for _, o := range strings.Split(s, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// loginRPS 把「每分钟上限」换算为每秒令牌速率，默认 10/min。
func loginRPS(cfg *config.Config) float64 {
	m := cfg.LoginRatePerMin
	if m <= 0 {
		m = 10
	}
	return float64(m) / 60.0
}
func Build(d *Deps) *gin.Engine {
	if d.Cfg.IsRelease() {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	// .torrent 文件上传上限 8MB（Gin 默认 32MB，收紧防滥用）。
	engine.MaxMultipartMemory = 8 << 20
	// 仅信任配置的反向代理；未配置时不信任任何代理（ClientIP 取直连 TCP 对端），
	// 避免伪造 X-Forwarded-For 绕过限流并放大令牌桶内存。
	if tp := splitOrigins(d.Cfg.TrustedProxies); len(tp) > 0 {
		_ = engine.SetTrustedProxies(tp)
	} else {
		_ = engine.SetTrustedProxies(nil)
	}
	engine.Use(gin.Recovery())
	engine.Use(middleware.RequestID())
	engine.Use(middleware.AccessLog())
	engine.Use(middleware.CORS(splitOrigins(d.Cfg.CORSAllowedOrigins)...))
	engine.Use(middleware.SecurityHeaders())
	engine.Use(middleware.MaxBody(64 << 20)) // 全局 body 上限兜底，防超大 JSON 灌内存 DoS（公开端点亦覆盖）
	engine.Use(middleware.JWTAuth(d.JWT, d.Cfg.APIKey, d.TVCache))
	// 全局限流（每 IP 每秒）
	globalRPS := d.Cfg.GlobalRatePerSec
	if globalRPS <= 0 {
		globalRPS = 20
	}
	engine.Use(middleware.NewRateLimiter(float64(globalRPS), globalRPS).Middleware())

	authH := v1.NewAuthHandler(d.DB, d.JWT, d.Act, d.HxptAuth, d.Crypto, d.TVCache, d.Tickets, d.Cfg.DemoMode)
	sysH := v1.NewSystemHandler(d.Update)

	api := engine.Group("/api/v1")
	{
		api.GET("/system/health", sysH.Health)
		api.GET("/system/info", sysH.Info)

		// 公开路由：激活前可达。登录/激活端点加更严限流防爆破。
		loginLimiter := middleware.NewRateLimiter(loginRPS(d.Cfg), 5)
		authRoutes := api.Group("/auth")
		authRoutes.Use(loginLimiter.Middleware())
		{
			authRoutes.POST("/setup", authH.Setup)
			authRoutes.POST("/login", authH.Login)
			authRoutes.POST("/hxpt/captcha", authH.HxptCaptcha)
			authRoutes.POST("/hxpt/login", authH.HxptLogin)
			authRoutes.POST("/hxpt/cookie-login", authH.HxptCookieLogin)
			authRoutes.POST("/demo-login", authH.DemoLogin)
			authRoutes.POST("/activate", authH.Activate)
			authRoutes.GET("/status", authH.Status)
			authRoutes.POST("/verify-vip", authH.VerifyVIP)
		}

		// 受保护路由：需登录 + 好学 VIP（按用户，宽限期内放行）。
		guarded := api.Group("")
		guarded.Use(middleware.RequireAuth())
		vipGrace := d.VIPGraceDays
		if vipGrace <= 0 {
			vipGrace = 3
		}
		guarded.Use(auth.RequireVIP(d.DB, vipGrace))
		guarded.POST("/auth/logout", authH.Logout)
		guarded.POST("/auth/ws-ticket", authH.WsTicket)
		// 在线更新：客户端经本后端中转检查（不直连官方源）；管理员写自托管清单覆盖官方源
		guarded.GET("/system/update/check", sysH.UpdateCheck)
		guarded.POST("/admin/update/manifest", middleware.RequireAdmin(), sysH.SetManifest)
		{
			siteH := v1.NewSiteHandler(d.SiteOp, d.SiteCh, d.Reg, d.SiteHC)
			sites := guarded.Group("/sites")
			{
				sites.GET("", siteH.List)
				sites.POST("", siteH.Create)
				sites.GET("/definitions", siteH.Definitions)
				sites.POST("/inspect", siteH.Inspect)
				sites.GET("/:id", siteH.Get)
				sites.PUT("/:id", siteH.Update)
				sites.DELETE("/:id", middleware.RequireAdmin(), siteH.Delete)
				sites.POST("/:id/test", siteH.Test)
				sites.POST("/:id/checkin", siteH.Checkin)
				sites.POST("/checkin", siteH.CheckinAll)
				sites.GET("/checkin/records", siteH.CheckinRecords)
				sites.POST("/:id/search", siteH.Search)
				sites.GET("/:id/torrent/detail", siteH.TorrentDetail)
				sites.POST("/:id/cookie", siteH.UpdateCookie)
			}
			guarded.POST("/search", siteH.SearchAll)

			// 站点信息聚合：全站免费/促销 + 公告（套用站点 capability，per-site 缓存）
			flH := v1.NewFreeleechHandler(d.SiteCh, d.SiteOp)
			fl := guarded.Group("/freeleech")
			{
				fl.GET("", flH.All)
				fl.POST("/sync/:id", flH.SyncOne)
			}
			annH := v1.NewAnnouncementHandler(d.SiteCh, d.SiteOp)
			ann := guarded.Group("/announcements")
			{
				ann.GET("", annH.All)
				ann.POST("/sync/:id", annH.SyncOne)
			}

			// 后宫：邀请库存 + 被邀请人花名册
			haremH := v1.NewHaremHandler(d.SiteCh, d.SiteOp)
			harem := guarded.Group("/harem")
			{
				harem.GET("", haremH.All)
				harem.POST("/sync/:id", haremH.SyncOne)
				harem.GET("/export", haremH.Export)
			}

			// 站点考核：per-platform 适配（MTeam JSON / NexusPHP HTML / TNode 不支持）
			assessH := v1.NewAssessmentHandler(d.Assessment, d.SiteOp)
			assess := guarded.Group("/assessment")
			{
				assess.GET("", assessH.All)
				assess.POST("/sync/:id", assessH.SyncOne)
			}

			// 勋章墙：跨站勋章聚合 + 购买/佩戴/摘下
			medalH := v1.NewMedalHandler(d.Medal, d.SiteOp)
			medal := guarded.Group("/medalwall")
			{
				medal.GET("", medalH.All)
				medal.POST("/buy", medalH.Buy)
				medal.POST("/wear", medalH.Wear)
				medal.POST("/takeoff", medalH.TakeOff)
			}

			// 站点私信：列表 + 正文 + 回复/删除/标记已读
			msgH := v1.NewMessageHandler(d.SiteCh, d.SiteOp)
			msg := guarded.Group("/messages")
			{
				msg.GET("", msgH.All)
				msg.POST("/sync/:id", msgH.SyncOne)
				msg.GET("/:id/content", msgH.Content)
				msg.POST("/:id/reply", msgH.Reply)
				msg.POST("/:id/delete", msgH.Delete)
				msg.POST("/:id/markread", msgH.MarkRead)
			}

			dlH := v1.NewDownloaderHandler(d.DLOp, d.DLCh, d.DLMgr)
			dls := guarded.Group("/downloaders")
			{
				dls.GET("", dlH.List)
				dls.POST("", dlH.Create)
				dls.PUT("/:id", dlH.Update)
				dls.DELETE("/:id", middleware.RequireAdmin(), dlH.Delete)
				dls.POST("/:id/test", dlH.Test)
				dls.GET("/:id/torrents", dlH.Torrents)
				dls.POST("/:id/torrents", dlH.Add)
				dls.POST("/:id/pause", dlH.Pause)
				dls.POST("/:id/resume", dlH.Resume)
				dls.POST("/:id/remove", dlH.Remove)
				dls.GET("/:id/stats", dlH.Stats)

				// 进阶操作（P0+P1）
				dls.POST("/:id/torrents/upload", dlH.UploadTorrent)   // 上传 .torrent 文件
				dls.GET("/:id/categories", dlH.Categories)            // 列分类
				dls.GET("/:id/tags", dlH.Tags)                        // 列标签
				dls.POST("/:id/torrents/category", dlH.SetCategoryOp) // 设置分类
				dls.POST("/:id/torrents/tags", dlH.TagsOp)            // 标签 set/add/remove
				dls.POST("/:id/torrents/recheck", dlH.Recheck)        // 重新校验
				dls.POST("/:id/torrents/reannounce", dlH.Reannounce)  // 重新汇报
				dls.POST("/:id/torrents/limits", dlH.SetLimits)       // 单种子限速
				dls.POST("/:id/torrents/location", dlH.SetLocation)   // 移动保存路径
				dls.GET("/:id/torrents/files", dlH.Files)             // 文件列表
				dls.POST("/:id/torrents/file-priority", dlH.SetFilePriority)
				dls.GET("/:id/torrents/trackers", dlH.Trackers) // tracker 列表
				dls.POST("/:id/torrents/trackers/add", dlH.AddTrackers)
				dls.POST("/:id/torrents/trackers/remove", dlH.RemoveTrackers)
				dls.GET("/:id/speed-limits", dlH.GetSpeedLimits) // 全局限速
				dls.POST("/:id/speed-limits", dlH.SetSpeedLimits)

				// P2：做种限制 / 队列 / 强制 / 顺序 / Peers / 分类增删
				dls.POST("/:id/torrents/share-limits", dlH.SetShareLimits)
				dls.POST("/:id/torrents/queue", dlH.SetQueuePriority)
				dls.POST("/:id/torrents/force-start", dlH.SetForceStart)
				dls.POST("/:id/torrents/sequential", dlH.ToggleSequential)
				dls.POST("/:id/torrents/first-last", dlH.ToggleFirstLast)
				dls.GET("/:id/torrents/peers", dlH.Peers)
				dls.POST("/:id/categories/create", dlH.CreateCategory)
				dls.POST("/:id/categories/edit", dlH.EditCategory)
				dls.POST("/:id/categories/delete", dlH.DeleteCategories)
			}

			notifH := v1.NewNotificationHandler(d.Notif, d.NotifMgr, d.NotifProvider)
			notif := guarded.Group("/notification")
			{
				notif.GET("/channels", notifH.Channels)
				notif.POST("/test", middleware.RequireAdmin(), notifH.Test)
				notif.GET("/config", notifH.GetConfig)
				notif.PUT("/config", middleware.RequireAdmin(), notifH.SetConfig)
				notif.POST("/wechat/qrcode", middleware.RequireAdmin(), notifH.WeChatQRCode)
				notif.GET("/wechat/qrcode/status", middleware.RequireAdmin(), notifH.WeChatQRStatus)
			}

			mediaH := v1.NewMediaHandler(d.MediaCh, d.Recommend)
			md := guarded.Group("/media")
			{
				md.GET("/search", mediaH.Search)
				md.GET("/recommend", mediaH.Recommend)
				md.GET("/discover", mediaH.Discover)
				md.GET("/poster", mediaH.Poster)
				md.GET("/detail", mediaH.Detail)
				md.GET("/image", mediaH.Image)
				md.POST("/nfo", mediaH.NFO)
				md.GET("/config", mediaH.GetConfig)
				md.PUT("/config", middleware.RequireAdmin(), mediaH.SetConfig)
			}

			ksH := v1.NewKeepSeedHandler(d.KeepSeed)
			guarded.POST("/keepseed/find", ksH.Find)
			ks := guarded.Group("/keepseed")
			{
				ks.GET("/config", ksH.GetConfig)
				ks.PUT("/config", ksH.SetConfig)
				ks.POST("/reseed", ksH.Reseed)
			ks.POST("/missing", ksH.Missing)
				ks.GET("/tasks", ksH.Tasks)
				ks.POST("/tasks/:id/resolve", ksH.Resolve)
				ks.GET("/setting", ksH.GetSetting)
				ks.PUT("/setting", ksH.SetSetting)
			}

			fsH := v1.NewFileStorageHandler(d.FileOp, d.FileCh, d.FileMgr)
			storages := guarded.Group("/storages")
			{
				storages.GET("", fsH.List)
				storages.POST("", fsH.Create)
				storages.PUT("/:id", fsH.Update)
				storages.DELETE("/:id", fsH.Delete)
				storages.POST("/:id/test", fsH.Test)
				storages.GET("/:id/files", fsH.Files)
				storages.POST("/:id/upload", fsH.Upload)
				storages.GET("/:id/download", fsH.Download)
				storages.POST("/:id/mkdir", fsH.Mkdir)
				storages.POST("/:id/delete", fsH.DeleteFile)
			}

			bkH := v1.NewBackupHandler(d.Backup, d.Notice)
			bk := guarded.Group("/backup")
			{
				bk.GET("/export", middleware.RequireAdmin(), bkH.Export)
				bk.POST("/import", middleware.RequireAdmin(), bkH.Import)
				bk.POST("/import-moviepilot", middleware.RequireAdmin(), bkH.ImportMoviePilot)
				bk.POST("/import-ptd", middleware.RequireAdmin(), bkH.ImportPTD)
			}

			credH := v1.NewCredentialHandler(oper.NewCredentialOper(d.DB, d.Crypto))
			creds := guarded.Group("/ops/credentials")
			creds.Use(middleware.RequireAdmin())
			{
				creds.GET("", credH.List)
				creds.POST("", credH.Create)
				creds.PUT("/:id", credH.Update)
				creds.DELETE("/:id", credH.Delete)
			}
			cfgH := v1.NewOpsConfigHandler(oper.NewOpsConfigOper(d.DB))
			opsCfg := guarded.Group("/ops/configs")
			opsCfg.Use(middleware.RequireAdmin())
			{
				opsCfg.GET("", cfgH.List)
				opsCfg.POST("", cfgH.Create)
				opsCfg.PUT("/:id", cfgH.Update)
				opsCfg.DELETE("/:id", cfgH.Delete)
				opsCfg.GET("/:id/history", cfgH.History)
			}
			cmdRunH := v1.NewCommandRunHandler(d.MonOp, oper.NewCommandRunOper(d.DB), d.MonSvc)
			cmdRuns := guarded.Group("/ops/command-runs")
			cmdRuns.Use(middleware.RequireAdmin())
			{
				cmdRuns.GET("", cmdRunH.List)
				cmdRuns.POST("", cmdRunH.Create)
				cmdRuns.GET("/:id", cmdRunH.Get)
			}

			entH := v1.NewEntertainmentHandler(d.Entertainment, d.Tickets, splitOrigins(d.Cfg.CORSAllowedOrigins))
			ent := guarded.Group("/entertainment")
			{
				ent.GET("/:id/games", entH.Games)
				ent.POST("/:id/play", entH.Play)
				ent.GET("/:id/games/:game/stats", entH.Stats)
				ent.GET("/:id/games/:game/buy-options", entH.BuyOptions)
				ent.POST("/:id/games/:game/buy", entH.Buy)
				ent.POST("/:id/games/:game/play-multi", entH.PlayMulti)
				ent.GET("/:id/games/:game/prizes", entH.Prizes)
				ent.GET("/:id/games/:game/history", entH.History)
				ent.GET("/:id/games/:game/syntheses", entH.Syntheses)
				ent.POST("/:id/games/:game/draw", entH.Draw)
				ent.POST("/:id/games/:game/synthesize", entH.Synthesize)
				ent.GET("/:id/games/:game/medal-tree", entH.MedalTree)
				ent.GET("/:id/games/:game/medal-shards", entH.MedalShards)
				ent.POST("/:id/games/:game/compose", entH.ComposeMedal)
				ent.POST("/:id/games/:game/compose-big", entH.ComposeBigMedal)
				ent.POST("/:id/games/:game/convert-shard", entH.ConvertShard)
				ent.GET("/:id/games/:game/farm", entH.Farm)
				ent.POST("/:id/games/:game/plant", entH.Plant)
				ent.POST("/:id/games/:game/harvest", entH.Harvest)
				ent.POST("/:id/games/:game/harvest-all", entH.HarvestAll)
				ent.POST("/:id/games/:game/sell", entH.Sell)
				ent.POST("/:id/games/:game/bet", entH.Bet)
				ent.GET("/:id/games/:game/events", entH.Events)
				ent.GET("/:id/games/:game/events/:eventID", entH.EventDetail)
				ent.POST("/:id/games/:game/blackjack", entH.Blackjack)
				ent.GET("/:id/games/:game/flip-state", entH.FlipState)
				ent.GET("/:id/games/:game/bigsmall-state", entH.BigSmallState)
				ent.GET("/:id/games/:game/bigsmall-chat", entH.BigSmallChat)
				ent.POST("/:id/games/:game/bigsmall-task", entH.BigSmallTaskAction)
				ent.POST("/:id/games/:game/bigsmall-item", entH.BigSmallUseItem)
				ent.POST("/:id/games/:game/bigsmall-chat", entH.BigSmallSendChat)
				ent.POST("/:id/games/:game/bigsmall-redpacket", entH.BigSmallRedpacket)
				// 翻面夺宝 websocket 桥接：?ticket= 鉴权（浏览器 WS 不能发 header JWT），
				// 注册在 api（非 guarded）绕过 header 鉴权，handler 内自验 ticket。
				api.GET("/entertainment/:id/games/:game/flip-ws", entH.FlipWS)
			}

			dashH := v1.NewDashboardHandler(d.Dashboard)
			guarded.GET("/dashboard", dashH.Overview)
			guarded.POST("/dashboard/sync", dashH.Sync)
			guarded.POST("/dashboard/sync/:id", dashH.SyncOne)
			guarded.GET("/dashboard/history/:id", dashH.History)
			guarded.GET("/dashboard/trend", dashH.Trend)

			// 运行日志（环形缓冲；级别筛选，排查用）
			guarded.GET("/system/logs", middleware.RequireAdmin(), sysH.Logs)
			guarded.DELETE("/system/logs", middleware.RequireAdmin(), sysH.ClearLogs)
			regH := v1.NewRegistrationHandler(d.RegOp, d.RegCh)
			regs := guarded.Group("/registrations")
			{
				regs.GET("", regH.List)
				regs.POST("", regH.Create)
				regs.PUT("/:id", regH.Update)
				regs.DELETE("/:id", regH.Delete)
				regs.POST("/:id/check", regH.Check)
			}

			subH := v1.NewSubscribeHandler(d.Subscribe)
			subs := guarded.Group("/subscriptions")
			{
				subs.GET("", subH.List)
				subs.POST("", subH.Create)
				subs.PUT("/:id", subH.Update)
				subs.DELETE("/:id", subH.Delete)
				subs.POST("/:id/check", subH.Check)
			}

			// 工作流：可视化节点图引擎。静态路由 /actions、/runs 置于 /:id 前避免冲突。
			wfH := v1.NewWorkflowHandler(d.Workflow)
			wfs := guarded.Group("/workflows")
			{
				wfs.GET("", wfH.List)
				wfs.POST("", wfH.Create)
				wfs.GET("/actions", wfH.Catalog)
				wfs.GET("/templates", wfH.Templates)
				wfs.GET("/stats", wfH.Stats)
				wfs.GET("/:id", wfH.Get)
				wfs.PUT("/:id", wfH.Update)
				wfs.DELETE("/:id", wfH.Delete)
				wfs.POST("/:id/run", wfH.Run)
				wfs.POST("/:id/enable", wfH.Enable)
				wfs.POST("/:id/disable", wfH.Disable)
				wfs.POST("/:id/reset", wfH.Reset)
			}
			wfRuns := guarded.Group("/workflow-runs")
			{
				wfRuns.GET("", wfH.ListRuns)
				wfRuns.GET("/:rid", wfH.RunDetail)
			}

			pluginH := v1.NewPluginHandler(d.Plugins, d.Tickets)
			api.GET("/plugins/:id/assets/*path", pluginH.Asset)
			plugins := guarded.Group("/plugins")
			{
				plugins.GET("", pluginH.List)
				plugins.GET("/logs", pluginH.Logs)
				plugins.POST("/events", middleware.RequireAdmin(), pluginH.PublishEvent)
				plugins.GET("/market", middleware.RequireAdmin(), pluginH.Market)
				plugins.GET("/market-sources", middleware.RequireAdmin(), pluginH.MarketSources)
				plugins.PUT("/market-sources", middleware.RequireAdmin(), pluginH.SetMarketSources)
				plugins.GET("/trust-config", middleware.RequireAdmin(), pluginH.TrustConfig)
				plugins.PUT("/trust-config", middleware.RequireAdmin(), pluginH.SetTrustConfig)
				plugins.POST("/market/install", middleware.RequireAdmin(), pluginH.Install)
				plugins.POST("/:id/assets/ticket", middleware.RequireAdmin(), pluginH.AssetTicket)
				plugins.GET("/:id/releases", pluginH.Releases)
				plugins.POST("/:id/bridge", middleware.RequireAdmin(), pluginH.Bridge)
				plugins.POST("/:id/rollback", middleware.RequireAdmin(), pluginH.Rollback)
				plugins.GET("/:id", pluginH.Get)
				plugins.PUT("/:id/config", middleware.RequireAdmin(), pluginH.SaveConfig)
				plugins.POST("/:id/enable", middleware.RequireAdmin(), pluginH.Enable)
				plugins.POST("/:id/disable", middleware.RequireAdmin(), pluginH.Disable)
				plugins.POST("/:id/action", middleware.RequireAdmin(), pluginH.Action)
				plugins.POST("/:id/reload", middleware.RequireAdmin(), pluginH.Reload)
				plugins.DELETE("/:id", middleware.RequireAdmin(), pluginH.Uninstall)
			}

			// 刷流引擎：任务 CRUD + 手动选种/扫删 + 种子加入历史
			bfH := v1.NewBrushFlowHandler(d.BrushFlow)
			bf := guarded.Group("/brushflow")
			{
				bf.GET("", bfH.List)
				bf.POST("", bfH.Create)
				bf.GET("/:id", bfH.Get)
				bf.PUT("/:id", bfH.Update)
				bf.DELETE("/:id", middleware.RequireAdmin(), bfH.Delete)
				bf.POST("/:id/select", bfH.Select)
				bf.POST("/:id/sweep", bfH.Sweep)
				bf.GET("/:id/seeds", bfH.Seeds)
			}

			// 保种契约：规则 CRUD + 现场检查
			ctH := v1.NewContractHandler(d.Contract)
			ct := guarded.Group("/contract")
			{
				ct.GET("/rules", ctH.List)
				ct.POST("/rules", ctH.Upsert)
				ct.DELETE("/rules/:id", middleware.RequireAdmin(), ctH.Delete)
				ct.POST("/rules/:id/check", ctH.Check)
			}

			// RSS 订阅：feed 抓取→过滤→去重→(可选)下载→通知。
			rssH := v1.NewRssHandler(d.Rss)
			rss := guarded.Group("/rss")
			{
				rss.GET("", rssH.List)
				rss.POST("", rssH.Create)
				rss.PUT("/:id", rssH.Update)
				rss.DELETE("/:id", rssH.Delete)
				rss.POST("/:id/check", rssH.Check)
				rss.GET("/:id/items", rssH.ListItems)
			}

			// 进站 Webhook 管理（CRUD）；公开接收端点 /webhooks/in/:token 在下方公开区注册。
			whH := d.Webhook
			whs := guarded.Group("/webhooks")
			{
				whs.GET("", whH.List)
				whs.POST("", middleware.RequireAdmin(), whH.Create)
				whs.PUT("/:id", middleware.RequireAdmin(), whH.Update)
				whs.DELETE("/:id", middleware.RequireAdmin(), whH.Delete)
				whs.POST("/:id/rotate", middleware.RequireAdmin(), whH.Rotate)
			}

			monH := v1.NewMonitorHandler(d.MonOp, d.MonSvc, d.SSHHostKeyOp)
			mons := guarded.Group("/monitors")
			{
				mons.GET("", monH.List)
				mons.POST("", middleware.RequireAdmin(), monH.Create)
				mons.PUT("/:id", middleware.RequireAdmin(), monH.Update)
				mons.DELETE("/:id", middleware.RequireAdmin(), monH.Delete)
				mons.POST("/:id/probe", middleware.RequireAdmin(), monH.Probe)
				mons.POST("/:id/collect", middleware.RequireAdmin(), monH.Collect)
				mons.POST("/:id/exec", middleware.RequireAdmin(), monH.Exec)
				mons.POST("/:id/power", middleware.RequireAdmin(), monH.Power)
				mons.GET("/:id/metrics", monH.Metrics)
				mons.POST("/:id/accept-host-key", middleware.RequireAdmin(), monH.AcceptHostKey)
			}
			alerts := guarded.Group("/monitor/alerts")
			{
				alerts.GET("", monH.ListAlerts)
				alerts.POST("/:id/read", monH.ReadAlert)
				alerts.POST("/:id/ack", middleware.RequireAdmin(), monH.AckAlert)
				alerts.POST("/:id/silence", middleware.RequireAdmin(), monH.SilenceAlert)
			}

			// Agent 管理（push 架构）：创建 agent 返回明文 secret、列表、删除、重置 secret。
			agentH := v1.NewAgentHandler(d.AgentOp, d.MonOp, d.DB)
			agents := guarded.Group("/monitors/agents")
			{
				agents.GET("", agentH.List)
				agents.POST("", middleware.RequireAdmin(), agentH.Create)
				agents.DELETE("/:id", middleware.RequireAdmin(), agentH.Delete)
				agents.POST("/:id/rotate", middleware.RequireAdmin(), agentH.Rotate)
			}

			// PT监护室：聚合 savept.icu 站点健康数据
			saveptH := v1.NewSaveptHandler(d.Savept)
			guarded.GET("/savept", saveptH.Get)

			// 全局通知中心（站内收件箱）
			noticeH := v1.NewNoticeHandler(d.Notice)
			ntc := guarded.Group("/notices")
			{
				ntc.GET("", noticeH.List)
				ntc.POST("/read-all", noticeH.ReadAll)
				ntc.POST("/clear", noticeH.Clear)
				ntc.POST("/:id/read", noticeH.Read)
				ntc.DELETE("/:id", noticeH.Delete)
			}
		}

		// WebSocket 实时推送（?ticket=<一次性> 鉴权；不进 guarded 的 header 鉴权）。
		wsH := v1.NewWSHandler(d.WS, d.Tickets, splitOrigins(d.Cfg.CORSAllowedOrigins))
		api.GET("/ws", wsH.Handle)

		// RTSP 视频流（?token=<jwt> 鉴权；浏览器不能直播 RTSP，由 RTSPProxy 转 WS）。
		streamH := v1.NewMonitorStreamHandler(d.RTSPProxy)
		api.GET("/monitor/cameras/:id/stream", streamH.Stream)
		terminalH := v1.NewMonitorTerminalHandler(d.MonSvc, d.Tickets, d.DB, splitOrigins(d.Cfg.CORSAllowedOrigins))
		api.GET("/monitors/:id/terminal", terminalH.Handle)
		// Agent 上报长连接（?agent_id=&secret= 鉴权；生产强制 wss://，secret 走 query 非 TLS 会被嗅探）。
		api.GET("/monitor/agents/ws", d.AgentHub.HandleWS)
		// Agent 一键安装脚本（公开，curl | bash；secret 由 env 传入不入 URL）。
		api.GET("/monitor/agents/install.sh", v1.ServeAgentInstall)
		// 摄像头 HLS（标准协议，供移动端 media_kit 等消费；同 ?token=<jwt> 鉴权）。
		api.GET("/monitor/cameras/:id/hls/playlist.m3u8", streamH.Playlist)
		api.GET("/monitor/cameras/:id/hls/segment/:seq", streamH.Segment)
		// 进站 Webhook 公开接收（token 即鉴权；静态 /in 前缀避开管理端 :id 参数名冲突）。
		api.POST("/webhooks/in/:token", d.Webhook.Receive)
	}

	// 非匹配路径回退到内嵌的 Web UI（go:embed），由 vue-router 接管。
	// /api/ 前缀的未命中必须返回 JSON 404：否则前端会把 SPA 的 index.html 当 JSON 解析，
	// 报 "Unexpected token '<'"，掩盖真正的端点拼写 / HTTP 方法错误（如把 PUT 误用为 POST）。
	spa := web.Handler() // http.Handler：文件存在直出，否则回退 index.html
	engine.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		spa.ServeHTTP(c.Writer, c.Request)
	})
	return engine
}
