package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/notification"
)

// NotificationHandler 处理通知测试、通道查询、通道配置与微信扫码登录。
type NotificationHandler struct {
	notif    *chain.NotificationChain
	mgr      *notification.Manager
	provider *chain.NotificationSettingsProvider
}

// NewNotificationHandler 构造。
func NewNotificationHandler(notif *chain.NotificationChain, mgr *notification.Manager, provider *chain.NotificationSettingsProvider) *NotificationHandler {
	return &NotificationHandler{notif: notif, mgr: mgr, provider: provider}
}

type notifyReq struct {
	Title string `json:"title" binding:"required,max=255"`
	Body  string `json:"body" binding:"max=2000"`
	Level string `json:"level" binding:"max=16"`
}

// notifyFanoutBudget 给「测试通知」扇出的最长耗时：单渠道 8s DNS + 15s HTTP 再加余量。
// 前端 notificationApi.test 的超时必须不短于它，否则客户端先掐断、后端还在发（#238）。
const notifyFanoutBudget = 30 * time.Second

// notifyFanoutContext 造一个「与浏览器断开无关」的有界扇出上下文。
// 抽成函数是为了可证伪：从前只有走查能看见这行 detach，负控得翻整段 handler；
// 现在父 ctx 一 cancel，判据必须照样说「渠道还在发」。
func notifyFanoutContext(req context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(req), notifyFanoutBudget)
}

// Test 发送一条测试通知到所有通道。
func (h *NotificationHandler) Test(c *gin.Context) {
	var req notifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if req.Level == "" {
		req.Level = "info"
	}
	// 扇出与「浏览器断开」解绑，并给一个有界预算（同 ops_command_run / docker_compose 的
	// detachedAction 口径）：单渠道最坏要 8s DNS（notifyResolveTimeout）+ 15s HTTP，而前端全局
	// axios 超时只有 15s——客户端一超时，正在跑的 Send 就地 canceled：消息其实已经发出去了，
	// 账面却记成失败，用户点重试就是重复推送（同 #167/#212 那条轴）。
	fctx, fcancel := notifyFanoutContext(c.Request.Context())
	defer fcancel()
	results := h.notif.Notify(fctx, notification.Message{
		Title: req.Title, Body: req.Body, Level: req.Level,
	})
	if results == nil {
		results = []notification.SendResult{}
	}
	c.JSON(http.StatusOK, gin.H{"channels": h.mgr.Channels(), "results": results})
}

// Channels 列出已注册通道。
func (h *NotificationHandler) Channels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"channels": h.mgr.Channels()})
}

// —— 通道配置（GET 任意 VIP / PUT admin）——

// GetConfig 返回当前通知通道配置（secret 脱敏 + configured 标记）。供「设置→通知」回显。
func (h *NotificationHandler) GetConfig(c *gin.Context) {
	s := h.provider.LoadSettings()
	c.JSON(http.StatusOK, notifyConfigResp{
		Bark: notifyBarkConfig{
			Enabled: s.Bark.Enabled, Server: s.Bark.Server,
			Key: maskSecret(s.Bark.Key), KeyConfigured: s.Bark.Key != "",
		},
		Telegram: notifyTGConfig{
			Enabled: s.Telegram.Enabled, ChatID: s.Telegram.ChatID, APIURL: s.Telegram.APIURL,
			Token: maskSecret(s.Telegram.Token), TokenConfigured: s.Telegram.Token != "",
		},
		QQ: notifyQQConfig{
			Enabled: s.QQ.Enabled, AppID: s.QQ.AppID, OpenID: s.QQ.OpenID, GroupOpenID: s.QQ.GroupOpenID,
			AppSecret: maskSecret(s.QQ.AppSecret), AppSecretConfigured: s.QQ.AppSecret != "",
		},
		WeChat: notifyWXConfig{
			Enabled: s.WeChat.Enabled, AccountID: s.WeChat.AccountID, Target: s.WeChat.Target, BaseURL: s.WeChat.BaseURL,
			BotToken: maskSecret(s.WeChat.BotToken), BotTokenConfigured: s.WeChat.BotToken != "",
		},
		Webhook:  notifyWebhookConfig{Enabled: s.Webhook.Enabled, URL: s.Webhook.URL},
		DingTalk: notifyDingTalkConfig{Enabled: s.DingTalk.Enabled, URL: s.DingTalk.URL},
		Feishu:   notifyFeishuConfig{Enabled: s.Feishu.Enabled, URL: s.Feishu.URL},
		Email: notifyEmailConfig{
			Enabled: s.Email.Enabled, Host: s.Email.Host, Port: s.Email.Port, Username: s.Email.Username,
			Password: maskSecret(s.Email.Password), PasswordConfigured: s.Email.Password != "",
			From: s.Email.From, To: s.Email.To,
		},
		PushPlus: notifyPushPlusConfig{
			Enabled: s.PushPlus.Enabled, Token: maskSecret(s.PushPlus.Token), TokenConfigured: s.PushPlus.Token != "",
		},
		PushDeer: notifyPushDeerConfig{
			Enabled: s.PushDeer.Enabled, Server: s.PushDeer.Server,
			Pushkey: maskSecret(s.PushDeer.Pushkey), PushkeyConfigured: s.PushDeer.Pushkey != "",
		},
		Ntfy: notifyNtfyConfig{
			Enabled: s.Ntfy.Enabled, Server: s.Ntfy.Server, Topic: s.Ntfy.Topic,
			Token: maskSecret(s.Ntfy.Token), TokenConfigured: s.Ntfy.Token != "",
		},
		Gotify: notifyGotifyConfig{
			Enabled: s.Gotify.Enabled, Server: s.Gotify.Server,
			Token: maskSecret(s.Gotify.Token), TokenConfigured: s.Gotify.Token != "",
		},
		Apprise: notifyAppriseConfig{
			Enabled: s.Apprise.Enabled, Server: s.Apprise.Server,
			Apikey: maskSecret(s.Apprise.Apikey), ApikeyConfigured: s.Apprise.Apikey != "",
		},
		WorkWechat: notifyWorkWechatConfig{Enabled: s.WorkWechat.Enabled, URL: s.WorkWechat.URL},
		WxPusher: notifyWxPusherConfig{
			Enabled: s.WxPusher.Enabled, UID: s.WxPusher.UID, TopicID: s.WxPusher.TopicID,
			AppToken: maskSecret(s.WxPusher.AppToken), AppTokenConfigured: s.WxPusher.AppToken != "",
		},
		Lark:    notifyLarkConfig{Enabled: s.Lark.Enabled, URL: s.Lark.URL},
		Discord: notifyDiscordConfig{Enabled: s.Discord.Enabled, URL: s.Discord.URL},
		Slack:   notifySlackConfig{Enabled: s.Slack.Enabled, URL: s.Slack.URL},
		MQTT: notifyMQTTConfig{
			Enabled: s.MQTT.Enabled, Broker: s.MQTT.Broker, Topic: s.MQTT.Topic, Username: s.MQTT.Username,
			Password: maskSecret(s.MQTT.Password), PasswordConfigured: s.MQTT.Password != "",
			ClientID: s.MQTT.ClientID,
		},
		ServerChan: notifyServerChanConfig{
			Enabled: s.ServerChan.Enabled, Key: maskSecret(s.ServerChan.Key), KeyConfigured: s.ServerChan.Key != "",
		},
	})
}

type notifyConfigReq struct {
	Bark struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server" binding:"max=512"`
		Key     string `json:"key" binding:"max=256"`
	} `json:"bark"`
	Telegram struct {
		Enabled bool   `json:"enabled"`
		Token   string `json:"token" binding:"max=256"`
		ChatID  string `json:"chat_id" binding:"max=64"`
		APIURL  string `json:"api_url" binding:"max=512"`
	} `json:"telegram"`
	QQ struct {
		Enabled     bool   `json:"enabled"`
		AppID       string `json:"app_id" binding:"max=32"`
		AppSecret   string `json:"app_secret" binding:"max=128"`
		OpenID      string `json:"openid" binding:"max=128"`
		GroupOpenID string `json:"group_openid" binding:"max=128"`
	} `json:"qq"`
	WeChat struct {
		Enabled   bool   `json:"enabled"`
		BotToken  string `json:"bot_token" binding:"max=256"`
		AccountID string `json:"account_id" binding:"max=64"`
		Target    string `json:"target" binding:"max=128"`
		BaseURL   string `json:"base_url" binding:"max=512"`
	} `json:"wechat"`
	Webhook struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url" binding:"max=512"`
	} `json:"webhook"`
	DingTalk struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url" binding:"max=512"`
	} `json:"dingtalk"`
	Feishu struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url" binding:"max=512"`
	} `json:"feishu"`
	Email struct {
		Enabled  bool   `json:"enabled"`
		Host     string `json:"host" binding:"max=255"`
		Port     int    `json:"port" binding:"min=1,max=65535"`
		Username string `json:"username" binding:"max=128"`
		Password string `json:"password" binding:"max=256"`
		From     string `json:"from" binding:"max=128"`
		To       string `json:"to" binding:"max=512"`
	} `json:"email"`
	PushPlus struct {
		Enabled bool   `json:"enabled"`
		Token   string `json:"token" binding:"max=256"`
	} `json:"pushplus"`
	PushDeer struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server" binding:"max=512"`
		Pushkey string `json:"pushkey" binding:"max=256"`
	} `json:"pushdeer"`
	Ntfy struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server" binding:"max=512"`
		Topic   string `json:"topic" binding:"max=256"`
		Token   string `json:"token" binding:"max=256"`
	} `json:"ntfy"`
	Gotify struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server" binding:"max=512"`
		Token   string `json:"token" binding:"max=256"`
	} `json:"gotify"`
	Apprise struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server" binding:"max=512"`
		Apikey  string `json:"apikey" binding:"max=256"`
	} `json:"apprise"`
	WorkWechat struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url" binding:"max=1024"`
	} `json:"workwechat"`
	WxPusher struct {
		Enabled  bool   `json:"enabled"`
		AppToken string `json:"app_token" binding:"max=256"`
		UID      string `json:"uid" binding:"max=128"`
		TopicID  string `json:"topic_id" binding:"max=128"`
	} `json:"wxpusher"`
	Lark struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url" binding:"max=1024"`
	} `json:"lark"`
	Discord struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url" binding:"max=1024"`
	} `json:"discord"`
	Slack struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url" binding:"max=1024"`
	} `json:"slack"`
	MQTT struct {
		Enabled  bool   `json:"enabled"`
		Broker   string `json:"broker" binding:"max=512"`
		Topic    string `json:"topic" binding:"max=256"`
		Username string `json:"username" binding:"max=128"`
		Password string `json:"password" binding:"max=256"`
		ClientID string `json:"client_id" binding:"max=128"`
	} `json:"mqtt"`
	ServerChan struct {
		Enabled bool   `json:"enabled"`
		Key     string `json:"key" binding:"max=256"`
	} `json:"serverchan"`
}

// SetConfig 保存通知通道配置并即时 rebuild（无需重启）。秘密字段空串=保留旧值。
func (h *NotificationHandler) SetConfig(c *gin.Context) {
	var req notifyConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	in := chain.NotificationSettings{}
	in.Bark.Enabled, in.Bark.Server, in.Bark.Key = req.Bark.Enabled, req.Bark.Server, req.Bark.Key
	in.Telegram.Enabled, in.Telegram.Token, in.Telegram.ChatID, in.Telegram.APIURL = req.Telegram.Enabled, req.Telegram.Token, req.Telegram.ChatID, req.Telegram.APIURL
	in.QQ.Enabled, in.QQ.AppID, in.QQ.AppSecret, in.QQ.OpenID, in.QQ.GroupOpenID = req.QQ.Enabled, req.QQ.AppID, req.QQ.AppSecret, req.QQ.OpenID, req.QQ.GroupOpenID
	in.WeChat.Enabled, in.WeChat.BotToken, in.WeChat.AccountID, in.WeChat.Target, in.WeChat.BaseURL = req.WeChat.Enabled, req.WeChat.BotToken, req.WeChat.AccountID, req.WeChat.Target, req.WeChat.BaseURL
	in.Webhook.Enabled, in.Webhook.URL = req.Webhook.Enabled, req.Webhook.URL
	in.DingTalk.Enabled, in.DingTalk.URL = req.DingTalk.Enabled, req.DingTalk.URL
	in.Feishu.Enabled, in.Feishu.URL = req.Feishu.Enabled, req.Feishu.URL
	in.Email.Enabled, in.Email.Host, in.Email.Port, in.Email.Username, in.Email.Password, in.Email.From, in.Email.To = req.Email.Enabled, req.Email.Host, req.Email.Port, req.Email.Username, req.Email.Password, req.Email.From, req.Email.To
	in.PushPlus.Enabled, in.PushPlus.Token = req.PushPlus.Enabled, req.PushPlus.Token
	in.PushDeer.Enabled, in.PushDeer.Server, in.PushDeer.Pushkey = req.PushDeer.Enabled, req.PushDeer.Server, req.PushDeer.Pushkey
	in.Ntfy.Enabled, in.Ntfy.Server, in.Ntfy.Topic, in.Ntfy.Token = req.Ntfy.Enabled, req.Ntfy.Server, req.Ntfy.Topic, req.Ntfy.Token
	in.Gotify.Enabled, in.Gotify.Server, in.Gotify.Token = req.Gotify.Enabled, req.Gotify.Server, req.Gotify.Token
	in.Apprise.Enabled, in.Apprise.Server, in.Apprise.Apikey = req.Apprise.Enabled, req.Apprise.Server, req.Apprise.Apikey
	in.WorkWechat.Enabled, in.WorkWechat.URL = req.WorkWechat.Enabled, req.WorkWechat.URL
	in.WxPusher.Enabled, in.WxPusher.AppToken, in.WxPusher.UID, in.WxPusher.TopicID = req.WxPusher.Enabled, req.WxPusher.AppToken, req.WxPusher.UID, req.WxPusher.TopicID
	in.Lark.Enabled, in.Lark.URL = req.Lark.Enabled, req.Lark.URL
	in.Discord.Enabled, in.Discord.URL = req.Discord.Enabled, req.Discord.URL
	in.Slack.Enabled, in.Slack.URL = req.Slack.Enabled, req.Slack.URL
	in.MQTT.Enabled, in.MQTT.Broker, in.MQTT.Topic, in.MQTT.Username, in.MQTT.Password, in.MQTT.ClientID = req.MQTT.Enabled, req.MQTT.Broker, req.MQTT.Topic, req.MQTT.Username, req.MQTT.Password, req.MQTT.ClientID
	in.ServerChan.Enabled, in.ServerChan.Key = req.ServerChan.Enabled, req.ServerChan.Key
	if err := h.provider.ApplySettings(in); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// —— 微信 ClawBot 扫码登录（admin）——

// WeChatQRCode 请求登录二维码。base_url 取当前微信配置（空→官方默认）。
func (h *NotificationHandler) WeChatQRCode(c *gin.Context) {
	baseURL := h.provider.LoadSettings().WeChat.BaseURL
	qrcode, qrcodeURL, err := notification.RequestQRCode(c.Request.Context(), baseURL)
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"qrcode": qrcode, "qrcode_url": qrcodeURL})
}

// WeChatQRStatus 查询扫码状态；登录成功自动绑定 token/accountID（provider.SetWeChatCredentials）。
func (h *NotificationHandler) WeChatQRStatus(c *gin.Context) {
	qrcode := c.Query("qrcode")
	if qrcode == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	baseURL := h.provider.LoadSettings().WeChat.BaseURL
	r, err := notification.PollQRStatus(c.Request.Context(), baseURL, qrcode)
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	loggedIn := false
	if r.Success && r.Token != "" {
		if err := h.provider.SetWeChatCredentials(r.Token, r.AccountID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "credential update failed", "status": r.Status})
			return
		}
		loggedIn = true
	}
	c.JSON(http.StatusOK, gin.H{"status": r.Status, "logged_in": loggedIn, "account_id": r.AccountID, "message": r.Message})
}
