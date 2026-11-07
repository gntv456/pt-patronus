// wire_extract.go —— New() 按域拆出的小装配函数（通知默认值 / 媒体链栈 / 守护神名片）。
// 目标：压 app.go New() 的行数与合并冲突面；逻辑零变化，纯搬运。
package app

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/mcp"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/plugins"
	subauto "github.com/hxpt/ptpatronus/internal/subtitle/automation"
)

// wireSubtitleStack 字幕链装配：多源 Provider（ASSRT/OpenSubtitles API + SubHD/Zimuku 抓取）
// → SubtitleChain → 健康中心一键补齐注入 → 字幕自动化服务启动。从 New() 拆出，逻辑零变化。
// 返回的 error 由调用方负责 wsCancel() 收尾。
func wireSubtitleStack(cfg *config.Config, cfgOp *oper.SystemConfigOper, base chain.Base, bus *event.Bus,
	noticeSvc *notice.Service, mediaServerCh *chain.MediaServerChain) (*chain.SubtitleChain, *subauto.Service, error) {
	// 源凭据走 SystemConfig（回退 env ASSRT_TOKEN/OPENSUBTITLES_KEY），UI 改动即时 rebuild。
	subtitleProvider, err := chain.NewSubtitleSourceProvider(cfgOp, chain.SubtitleSettings{
		AssrtToken:       cfg.AssrtToken,
		OpenSubtitlesKey: cfg.OpenSubtitlesKey,
	})
	if err != nil {
		return nil, nil, err
	}
	subtitleCh := chain.NewSubtitleChain(base, subtitleProvider)
	// 媒体健康中心联动：missing_subtitle 一键补齐（复用字幕搜索/下载源）。
	mediaServerCh.SetSubtitleFixer(chain.SubtitleChainAdapter{Ch: subtitleCh})

	// 字幕自动化：监听 transfer_done → 按语言/provider 优先级自动搜字幕落盘到媒体同目录（Bazarr /
	// ChineseSubFinder 模式）。复用 subtitleCh 搜/下载（结构化实现 subauto.Searcher）；配置走 cfgOp，
	// 通知走 noticeSvc；默认关闭，用户经 /subtitle/automation/config 开启。异步 handler 不阻塞整理。
	subAutoSvc := subauto.New(bus, subtitleCh, cfgOp, noticeSvc)
	subAutoSvc.Start()
	return subtitleCh, subAutoSvc, nil
}

// wireNotificationDefaults 从 env 配置构建通知通道默认值（UI 未覆盖时的回退）。
func wireNotificationDefaults(cfg *config.Config) chain.NotificationSettings {
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
	return notifDefaults
}

// wireMediaStack 构建媒体元数据链（TMDB/豆瓣/Bangumi/MusicBrainz provider + 搜索链 + 推荐链），
// 并把 MediaChain 暴露给插件的 Host API（/media/search），刮削类插件复用宿主元数据源。
func wireMediaStack(cfg *config.Config, cfgOp *oper.SystemConfigOper, base chain.Base, pluginMgr *plugins.Manager, database *gorm.DB) (mediaCh *chain.MediaChain, recommendCh *chain.RecommendChain) {
	mediaProvider := chain.NewMediaSourceProvider(cfgOp, chain.MediaSettings{
		TMDBAPIKey:     cfg.TMDBAPIKey,
		TMDBAPIDomain:  cfg.TMDBAPIDomain,
		DoubanSecret:   cfg.DoubanSecret,
		DoubanAppKey:   cfg.DoubanAppKey,
		BangumiToken:   cfg.BangumiToken,
		CunguangjiBase: cfg.CunguangjiBase,
		OMDbAPIKey:     cfg.OMDbAPIKey,
	})
	posterCache := oper.NewPosterCacheOper(database)
	mediaCh = chain.NewMediaChain(base, mediaProvider, posterCache, cfgOp)
	pluginMgr.SetMediaSearcher(mediaSearchAdapter{mediaCh})
	// 推荐信息流：复用同一份 provider（豆瓣热映 / TMDB trending / Bangumi 热门）
	recommendCh = chain.NewRecommendChain(base, mediaProvider)
	return
}

// wireMCPAndBuiltinPlugins 构建 MCP 服务端（宿主能力暴露为 MCP 工具，供 AI Agent / IDE 经
// JSON-RPC 调用；配置由 ptp-mcp-server 插件承载，SettingsGetter 每次请求读插件配置，热改即时
// 生效；公开路由 /api/v1/mcp 在 router 注册），并注册宿主级内置工具插件（WOL 唤醒 / 豆瓣榜单
// 订阅——与 MCP 插件同为主机进程内注册，Builtin=true 不可被 market 生态替换）。
func wireMCPAndBuiltinPlugins(pluginMgr *plugins.Manager, siteOp *oper.SiteOper, siteCh *chain.SiteChain, subCh *chain.SubscribeChain, dlOp *oper.DownloaderOper) *mcp.Server {
	srv := mcp.NewServer()
	registerHostMCPTools(srv, siteOp, siteCh, subCh, dlOp)
	srv.SetSettingsGetter(func() mcp.Settings {
		if v, err := pluginMgr.Get(plugins.MCPPluginID); err == nil {
			return plugins.ParseMCPSettings(v.Config)
		}
		return mcp.Settings{}
	})
	if err := pluginMgr.Register(plugins.NewMCPServerPlugin(srv)); err != nil {
		slog.Warn("register mcp plugin", "err", err)
	}
	if err := pluginMgr.Register(plugins.NewWOLPlugin()); err != nil {
		slog.Warn("register wol plugin", "err", err)
	}
	if err := pluginMgr.Register(plugins.NewDoubanChartPlugin()); err != nil {
		slog.Warn("register douban chart plugin", "err", err)
	}
	return srv
}
