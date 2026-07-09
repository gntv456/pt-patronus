package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
	Title string `json:"title" binding:"required"`
	Body  string `json:"body"`
	Level string `json:"level"`
}

// Test 发送一条测试通知到所有通道。
func (h *NotificationHandler) Test(c *gin.Context) {
	var req notifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Level == "" {
		req.Level = "info"
	}
	results := h.notif.Notify(c.Request.Context(), notification.Message{
		Title: req.Title, Body: req.Body, Level: req.Level,
	})
	c.JSON(http.StatusOK, gin.H{"channels": h.mgr.Channels(), "results": results})
}

// Channels 列出已注册通道。
func (h *NotificationHandler) Channels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"channels": h.mgr.Channels()})
}

// —— 通道配置（GET 任意 VIP / PUT admin）——

type notifyBarkConfig struct {
	Enabled       bool   `json:"enabled"`
	Server        string `json:"server"`
	Key           string `json:"key"` // 脱敏
	KeyConfigured bool   `json:"key_configured"`
}
type notifyTGConfig struct {
	Enabled         bool   `json:"enabled"`
	ChatID          string `json:"chat_id"`
	APIURL          string `json:"api_url"`
	Token           string `json:"token"` // 脱敏
	TokenConfigured bool   `json:"token_configured"`
}
type notifyQQConfig struct {
	Enabled             bool   `json:"enabled"`
	AppID               string `json:"app_id"`
	OpenID              string `json:"openid"`
	GroupOpenID         string `json:"group_openid"`
	AppSecret           string `json:"app_secret"` // 脱敏
	AppSecretConfigured bool   `json:"app_secret_configured"`
}
type notifyWXConfig struct {
	Enabled            bool   `json:"enabled"`
	AccountID          string `json:"account_id"`
	Target             string `json:"target"`
	BaseURL            string `json:"base_url"`
	BotToken           string `json:"bot_token"` // 脱敏
	BotTokenConfigured bool   `json:"bot_token_configured"`
}
type notifyWebhookConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}
type notifyDingTalkConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}
type notifyFeishuConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}
type notifyEmailConfig struct {
	Enabled            bool   `json:"enabled"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Username           string `json:"username"`
	Password           string `json:"password"` // 脱敏
	PasswordConfigured bool   `json:"password_configured"`
	From               string `json:"from"`
	To                 string `json:"to"`
}
type notifyConfigResp struct {
	Bark     notifyBarkConfig     `json:"bark"`
	Telegram notifyTGConfig       `json:"telegram"`
	QQ       notifyQQConfig       `json:"qq"`
	WeChat   notifyWXConfig       `json:"wechat"`
	Webhook  notifyWebhookConfig  `json:"webhook"`
	DingTalk notifyDingTalkConfig `json:"dingtalk"`
	Feishu   notifyFeishuConfig   `json:"feishu"`
	Email    notifyEmailConfig    `json:"email"`
}

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
	})
}

type notifyConfigReq struct {
	Bark struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server"`
		Key     string `json:"key"`
	} `json:"bark"`
	Telegram struct {
		Enabled bool   `json:"enabled"`
		Token   string `json:"token"`
		ChatID  string `json:"chat_id"`
		APIURL  string `json:"api_url"`
	} `json:"telegram"`
	QQ struct {
		Enabled     bool   `json:"enabled"`
		AppID       string `json:"app_id"`
		AppSecret   string `json:"app_secret"`
		OpenID      string `json:"openid"`
		GroupOpenID string `json:"group_openid"`
	} `json:"qq"`
	WeChat struct {
		Enabled   bool   `json:"enabled"`
		BotToken  string `json:"bot_token"`
		AccountID string `json:"account_id"`
		Target    string `json:"target"`
		BaseURL   string `json:"base_url"`
	} `json:"wechat"`
	Webhook struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url"`
	} `json:"webhook"`
	DingTalk struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url"`
	} `json:"dingtalk"`
	Feishu struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url"`
	} `json:"feishu"`
	Email struct {
		Enabled  bool   `json:"enabled"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		From     string `json:"from"`
		To       string `json:"to"`
	} `json:"email"`
}

// SetConfig 保存通知通道配置并即时 rebuild（无需重启）。秘密字段空串=保留旧值。
func (h *NotificationHandler) SetConfig(c *gin.Context) {
	var req notifyConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	if err := h.provider.ApplySettings(in); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"qrcode": qrcode, "qrcode_url": qrcodeURL})
}

// WeChatQRStatus 查询扫码状态；登录成功自动绑定 token/accountID（provider.SetWeChatCredentials）。
func (h *NotificationHandler) WeChatQRStatus(c *gin.Context) {
	qrcode := c.Query("qrcode")
	if qrcode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing qrcode"})
		return
	}
	baseURL := h.provider.LoadSettings().WeChat.BaseURL
	r, err := notification.PollQRStatus(c.Request.Context(), baseURL, qrcode)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	loggedIn := false
	if r.Success && r.Token != "" {
		if err := h.provider.SetWeChatCredentials(r.Token, r.AccountID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": r.Status})
			return
		}
		loggedIn = true
	}
	c.JSON(http.StatusOK, gin.H{"status": r.Status, "logged_in": loggedIn, "account_id": r.AccountID, "message": r.Message})
}
