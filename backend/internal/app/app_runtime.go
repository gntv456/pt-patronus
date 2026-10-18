package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/aiagent"
	"github.com/hxpt/ptpatronus/internal/api"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	v1 "github.com/hxpt/ptpatronus/internal/api/v1"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/automation"
	"github.com/hxpt/ptpatronus/internal/backup"
	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/dirwatch"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/filemanager"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/llm"
	"github.com/hxpt/ptpatronus/internal/mcp"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/plugins"
	"github.com/hxpt/ptpatronus/internal/savept"
	"github.com/hxpt/ptpatronus/internal/scheduler"
	"github.com/hxpt/ptpatronus/internal/site"
	subauto "github.com/hxpt/ptpatronus/internal/subtitle/automation"
	"github.com/hxpt/ptpatronus/internal/tgbot"
	"github.com/hxpt/ptpatronus/internal/transfer"
	"github.com/hxpt/ptpatronus/internal/update"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"github.com/hxpt/ptpatronus/internal/workflow/nodes"
	"github.com/hxpt/ptpatronus/internal/ws"
	"gorm.io/gorm"
)

// runtimeDeps 汇总 buildHTTPServer 所需的全部已构造依赖（业务 chain / oper / 服务 / 基础设施）。
type runtimeDeps struct {
	cfg            *config.Config
	database       *gorm.DB
	bus            *event.Bus
	hc             *httpclient.Client
	registry       *site.Registry
	base           chain.Base
	act            *auth.Activation
	assessCh       *chain.AssessmentChain
	auto           *automation.Service
	backupSvc      *backup.Service
	brushCh        *chain.BrushFlowChain
	cfgOp          *oper.SystemConfigOper
	contractCh     *chain.ContractChain
	cookieCloudCh  *chain.CookieCloudChain
	crypto         *security.Crypto
	dashCh         *chain.DashboardChain
	dlCh           *chain.DownloadChain
	dlMgr          *manager.Manager
	dlOp           *oper.DownloaderOper
	entCh          *chain.EntertainmentChain
	fileCh         *chain.FileManagerChain
	fileMgr        *filemanager.Manager
	fileOp         *oper.FileStorageOper
	filterEng      *filter.Engine
	filterStore    *filter.Store
	hxpt           *auth.HxptVIP
	hxptLogin      *auth.HxptLogin
	j              *security.JWT
	keepSeedCh     *chain.KeepSeedChain
	mcpSrv         *mcp.Server
	medalCh        *chain.MedalChain
	mediaCh        *chain.MediaChain
	mediaServerCh  *chain.MediaServerChain
	cloudCh        *chain.CloudChain
	cloudTaskOp    *oper.CloudTaskOper
	noticeOp       *oper.NoticeOper
	noticeSvc      *notice.Service
	notifCh        *chain.NotificationChain
	notifMgr       *notification.Manager
	notifProvider  *chain.NotificationSettingsProvider
	oidcSvc        *auth.OIDCService
	pluginMgr      *plugins.Manager
	recommendCh    *chain.RecommendChain
	regCh          *chain.RegistrationChain
	regOp          *oper.RegistrationOper
	rssCh          *chain.RssChain
	saveptSvc      *savept.Service
	sched          *scheduler.Scheduler
	siteCh         *chain.SiteChain
	siteOp         *oper.SiteOper
	subAutoSvc     *subauto.Service
	subCh          *chain.SubscribeChain
	subtitleCh     *chain.SubtitleChain
	traktCh        *chain.TraktChain
	transferCh     *chain.TransferChain
	transferHistOp *oper.TransferHistoryOper
	tvCache        *auth.TokenVersionCache
	updateSvc      *update.Service
	wfOp           *oper.WorkflowOper
	wfReg          *workflow.Registry
	wfRunOp        *oper.WorkflowRunOper
	wsBC           *ws.Broadcaster
}

// buildHTTPServer 构造 monitor/工作流/Agent/目录监视/TG bot 运行时，注册全部后台 cron，
// 组装 api.Deps 并返回 HTTP server 与三个后台 goroutine 的取消函数。
func buildHTTPServer(d runtimeDeps) (*http.Server, context.CancelFunc, context.CancelFunc, context.CancelFunc) {
	cfg := d.cfg
	database := d.database
	bus := d.bus
	hc := d.hc
	registry := d.registry
	base := d.base
	act := d.act
	assessCh := d.assessCh
	auto := d.auto
	backupSvc := d.backupSvc
	brushCh := d.brushCh
	cfgOp := d.cfgOp
	contractCh := d.contractCh
	cookieCloudCh := d.cookieCloudCh
	crypto := d.crypto
	dashCh := d.dashCh
	dlCh := d.dlCh
	dlMgr := d.dlMgr
	dlOp := d.dlOp
	entCh := d.entCh
	fileCh := d.fileCh
	fileMgr := d.fileMgr
	fileOp := d.fileOp
	filterEng := d.filterEng
	filterStore := d.filterStore
	hxpt := d.hxpt
	hxptLogin := d.hxptLogin
	j := d.j
	keepSeedCh := d.keepSeedCh
	mcpSrv := d.mcpSrv
	medalCh := d.medalCh
	mediaCh := d.mediaCh
	mediaServerCh := d.mediaServerCh
	cloudCh := d.cloudCh
	noticeOp := d.noticeOp
	noticeSvc := d.noticeSvc
	notifCh := d.notifCh
	notifMgr := d.notifMgr
	notifProvider := d.notifProvider
	oidcSvc := d.oidcSvc
	pluginMgr := d.pluginMgr
	recommendCh := d.recommendCh
	regCh := d.regCh
	regOp := d.regOp
	rssCh := d.rssCh
	saveptSvc := d.saveptSvc
	sched := d.sched
	siteCh := d.siteCh
	siteOp := d.siteOp
	subAutoSvc := d.subAutoSvc
	subCh := d.subCh
	subtitleCh := d.subtitleCh
	traktCh := d.traktCh
	transferCh := d.transferCh
	transferHistOp := d.transferHistOp
	tvCache := d.tvCache
	updateSvc := d.updateSvc
	wfOp := d.wfOp
	wfReg := d.wfReg
	wfRunOp := d.wfRunOp
	wsBC := d.wsBC
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
	// 站点考核：已并入 site-snapshot 聚合任务（复用 UserInfo 的 uid 访问 userdetails.php?id=<uid>，
	// 不再每整小时裸访 userdetails.php——52movie 警告根因）。前端/API 手动刷新仍走 assessCh.AssessAll。
	// 与下载完成事件钩子互补：覆盖重启/漏触发期；幂等（按 download_hash 成功去重）。

	// AI Agent（ReAct 循环 + 双 LLM provider + 工具调用 + 多轮上下文）。
	// 工具集绑定各业务 chain（读+写，admin-only）；会话/消息持久化；配置+API key 走 SystemConfig+Crypto。
	agentConvOp := oper.NewAgentConversationOper(database)
	agentMsgOp := oper.NewAgentMessageOper(database)
	agentTools := registerAgentTools(siteOp, siteCh, subCh, dlOp, dlCh, mediaCh, recommendCh, transferCh, keepSeedCh, dashCh, notifCh, filterStore, mediaServerCh, cloudCh, brushCh, rssCh, subtitleCh, fileCh, wfCh)
	// 把全部 chain 能力批量暴露给插件 Host API（GET /tools、POST /tools/{name}、Bridge tool.*）。
	// 必须在所有 chain 构造之后调用（闭包捕获 chain 指针）。
	registerPluginHostTools(pluginMgr, agentTools, dlCh, subCh, mediaServerCh, cloudCh, brushCh, rssCh, subtitleCh, transferCh, keepSeedCh, fileCh, wfCh)
	// 单 Agent 实例同时服务内置助手聊天 / LLM 网关两协议端点 / TG bot；Prometheus 来源
	// 标签统一记 assistant（细分入口留后续按 handler 各自实例化时再拆）。
	agentSvc := aiagent.New(bizmetrics.LLMSourceAssistant, llm.New, agentTools, agentMsgOp, agentConvOp, cfgOp, crypto)
	agentChatH := v1.NewAgentChatHandler(agentSvc, agentConvOp.List, agentConvOp.Count, agentConvOp.Create, agentConvOp.Delete, agentConvOp.Get, agentMsgOp.ListByConv, cfgOp, crypto)
	llmGatewayH := v1.NewLLMGatewayHandler(agentSvc, agentConvOp.Create, cfgOp, crypto)
	doctorH := v1.NewDoctorHandler(cfg, database, buildDoctorBusinessChecks(d))

	// Telegram bot（入站 chat-ops）：getUpdates 长轮询 → 自然语言经 AI Agent 驱动全平台。
	// safe-mode 下不启；配置保存后热重载（停旧 goroutine、按新配置再起），无需重启进程。
	var tgMu sync.Mutex
	var tgCancel context.CancelFunc
	sessions := newTgChatSessions()
	talk := func(ctx context.Context, chatKey, text string) (<-chan llm.Event, error) {
		// 与 HTTP 面同款预检：AI 未就绪时不建会话（否则未配置期间每条 TG 消息都留一条空会话）。
		switch agentSvc.Ready() {
		case aiagent.ReadyDisabled:
			return nil, errcode.AgentDisabled
		case aiagent.ReadyNoKey:
			return nil, errcode.AgentNoAPIKey
		case aiagent.ReadyKeyUnreadable:
			// 这两个原因从前落到 switch 外面：照旧建会话，「未就绪期间每条 TG 消息留一条空会话」的后果仍在。
			return nil, errcode.AgentKeyUnreadable
		case aiagent.ReadyConfigUnreadable:
			return nil, errcode.AgentConfigUnreadable
		}
		convID, err := sessions.resolve(chatKey, agentConvOp.Create)
		if err != nil {
			return nil, err
		}
		return agentSvc.Run(ctx, convID, text), nil
	}
	clear := func(chatKey string) { sessions.clear(chatKey) }
	var startTgBot func()
	startTgBot = func() {
		tgMu.Lock()
		defer tgMu.Unlock()
		if tgCancel != nil {
			tgCancel()
			tgCancel = nil
		}
		if cfg.SafeMode {
			return
		}
		tgc := tgbot.Load(cfgOp)
		if !tgc.Enabled || tgc.BotTokenEnc == "" {
			return
		}
		token := tgbot.BotToken(tgc, crypto)
		if token == "" {
			return
		}
		bot := tgbot.New(token, tgc.APIURL, tgc.AllowedUserIDs, tgc.AllowGroupChat, talk, clear)
		tgCtx, cancel := context.WithCancel(context.Background())
		tgCancel = cancel
		go bot.Run(tgCtx)
	}
	startTgBot()
	tgBotH := v1.NewTgBotHandler(cfgOp, crypto, func() { startTgBot() })
	// Servarr 兼容（Sonarr/Radarr v3）：订阅 ↔ Movie/Series，hasFile 查整理历史。无外部依赖，safe-mode 下仍注册。
	servarrH := v1.NewServarrHandler(subCh, mediaCh, transferHistOp.HasSuccessByMedia, cfg.APIKey)

	// 目录监视：fsnotify 实时喂整理（补 DoneHook + cron 覆盖不到的手动投放 / 非托管来源）。safe-mode 下不启。
	var dwCancel context.CancelFunc
	if !cfg.SafeMode {
		if dwc := dirwatch.Load(cfgOp); dwc.Enabled && len(dwc.Dirs) > 0 {
			dw := dirwatch.New(dwc, func(ctx context.Context, srcDir string, files []transfer.FileInput) {
				_, _ = transferCh.ProcessDiscoveredFiles(ctx, srcDir, files)
			})
			dwCtx, cancel := context.WithCancel(context.Background())
			dwCancel = cancel
			go dw.Run(dwCtx)
		}
	}

	registerCronJobs(sched, cfg, brushCh, contractCh, traktCh, cookieCloudCh, rssCh, wfRunOp, monOp, noticeOp, saveptSvc, notifCh, noticeSvc, medalCh, subCh, transferCh, cloudCh, oper.NewRssItemOper(database), oper.NewPluginOper(database), backupSvc)

	engine := api.Build(&api.Deps{
		Cfg: cfg, DB: database, JWT: j, Act: act,
		HxptVIP: hxpt, HxptAuth: hxptLogin, Crypto: crypto, TVCache: tvCache, VIPGraceDays: cfg.VIPGraceDays,
		Tickets: ticketStore,
		OIDC:    oidcSvc,
		SiteOp:  siteOp, SiteCh: siteCh, Reg: registry, SiteHC: hc,
		DLOp: dlOp, DLCh: dlCh, DLMgr: dlMgr,
		Notif: notifCh, NotifMgr: notifMgr, NotifProvider: notifProvider, CfgOp: cfgOp,
		MediaCh: mediaCh, FileOp: fileOp, FileCh: fileCh, FileMgr: fileMgr,
		Recommend: recommendCh,
		KeepSeed:  keepSeedCh, Backup: backupSvc,
		Entertainment: entCh,
		Dashboard:     dashCh,
		WS:            wsBC,
		RegOp:         regOp, RegCh: regCh,
		Subscribe:          subCh,
		Workflow:           wfCh,
		Rss:                rssCh,
		BrushFlow:          brushCh,
		Contract:           contractCh,
		MediaServer:        mediaServerCh,
		Transfer:           transferCh,
		AgentChat:          agentChatH,
		LLMGateway:         llmGatewayH,
		Doctor:             doctorH,
		TgBot:              tgBotH,
		Servarr:            servarrH,
		Cloud:              cloudCh,
		Subtitle:           subtitleCh,
		SubtitleAutomation: subAutoSvc,
		Trakt:              traktCh,
		CookieCloud:        cookieCloudCh,
		MCP:                mcpSrv,
		Webhook:            webhookH,
		MonOp:              monOp, MonSvc: monSvc, RTSPProxy: rtspProxy,
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
		Automation: auto,
	})

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: engine}

	return srv, monCancel, dwCancel, tgCancel
}

// Run starts the HTTP server (blocking).
func (a *App) Run() error { return a.http.ListenAndServe() }

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
		// 二轮 P2-4：等待在途 job 收尾（上限 10s）——此前直接丢弃 Stop() 返回的
		// ctx，滚动发布窗口内旧进程 job（如 brushflow 选种）与新进程并发写同一
		// 下载器，SkipIfStillRunning 管不到跨进程。
		select {
		case <-a.sched.Stop().Done():
		case <-time.After(10 * time.Second):
			slog.Warn("scheduler stop timeout, in-flight jobs may overlap with next process")
		}
	}
	if a.pluginMgr != nil {
		a.pluginMgr.Stop()
	}
	// 云任务执行器停机（P2 修复：stop channel 此前永不关闭，执行器 goroutine 泄漏，
	// 且在途 upload 用 2h Background ctx 拖到 http.Shutdown 超时强杀、任务卡 running）。
	if a.cloudCh != nil {
		a.cloudCh.StopCloudTaskExecutor()
	}
	// WS 广播器停机（四轮 P1 修复）：Run 内 close(hookQ)+wg.Wait() 会等在途下载
	// 完成钩子（媒体整理硬链/NFO）收尾后再退，此前 ctx 永不取消导致进程退出时硬杀。
	if a.wsBCStop != nil {
		a.wsBCStop()
	}
	// 整理链直传云上传兜底路径等待（四轮 P2 修复：fire-and-forget 上传/删本地被硬杀）。
	if a.transferCh != nil {
		a.transferCh.WaitCloudUploads(ctx)
	}
	return a.http.Shutdown(ctx)
}
