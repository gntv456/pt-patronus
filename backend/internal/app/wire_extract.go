// wire_extract.go —— New() 按域拆出的小装配函数（通知默认值 / 媒体链栈）。
// 目标：压 app.go New() 的行数与合并冲突面；逻辑零变化，纯搬运。
package app

import (
	"gorm.io/gorm"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

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
