package chain

import (
	"encoding/json"
	"sync"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notification"
)

// notifySettingsKey 是 SystemConfig 中存放全部通知通道配置的 JSON blob。
// 单 key 存整体（而非每字段一 key）：通道内多字段原子更新，且 enabled/秘密回填语义更简单。
const notifySettingsKey = "notify.settings"

// NotificationSettings 是全部通知通道的可配置项。整体序列化为 JSON 存 SystemConfig。
// 每通道：Enabled + 各凭据；秘密字段在 ApplySettings 时空串=保留旧值（UI 不重输秘密时回填）。
type NotificationSettings struct {
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
	// —— 批次6 补全渠道（11 个，与既有 8 个并列；秘密字段空串=保留旧值）——
	PushPlus struct {
		Enabled bool   `json:"enabled"`
		Token   string `json:"token"` // 秘密
	} `json:"pushplus"`
	PushDeer struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server"`
		Pushkey string `json:"pushkey"` // 秘密
	} `json:"pushdeer"`
	Ntfy struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server"`
		Topic   string `json:"topic"`
		Token   string `json:"token"` // 秘密（私有/受保护 topic）
	} `json:"ntfy"`
	Gotify struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server"`
		Token   string `json:"token"` // 秘密
	} `json:"gotify"`
	Apprise struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server"`
		Apikey  string `json:"apikey"` // 秘密
	} `json:"apprise"`
	WorkWechat struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url"` // webhook url（含 key；按既有 DingTalk/Feishu 惯例不脱敏）
	} `json:"workwechat"`
	WxPusher struct {
		Enabled bool   `json:"enabled"`
		AppToken string `json:"app_token"` // 秘密
		UID      string `json:"uid"`
		TopicID  string `json:"topic_id"`
	} `json:"wxpusher"`
	Lark struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url"`
	} `json:"lark"`
	Discord struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url"`
	} `json:"discord"`
	MQTT struct {
		Enabled   bool   `json:"enabled"`
		Broker    string `json:"broker"`
		Topic     string `json:"topic"`
		Username  string `json:"username"`
		Password  string `json:"password"` // 秘密
		ClientID  string `json:"client_id"`
	} `json:"mqtt"`
	ServerChan struct {
		Enabled bool   `json:"enabled"`
		Key     string `json:"key"` // 秘密（SendKey）
	} `json:"serverchan"`
}

// NotificationSettingsProvider 按 SystemConfig（回退环境变量默认 defaults）构建通知通道，
// 并 hot-swap 到共享的 *notification.Manager。ApplySettings/SetWeChatCredentials 后 rebuild，
// 使 UI 改动/扫码登录即时生效（无需重启）。
//
// 设计与 MediaSourceProvider 同构：通道把凭据烤进构造器，故「写后即生效」靠 rebuild 整个
// 切片 + Manager.Replace 实现。Manager 与 NotificationChain、notice.Service 共用同一指针，
// 故一处替换全局生效。
type NotificationSettingsProvider struct {
	cfg      *oper.SystemConfigOper
	defaults NotificationSettings
	mgr      *notification.Manager
	mu       sync.Mutex // 序列化 ApplySettings/SetWeChatCredentials 的 读改写+rebuild
}

// NewNotificationSettingsProvider 构造；立即按当前 SystemConfig + 默认构建一次通道。
func NewNotificationSettingsProvider(cfg *oper.SystemConfigOper, defaults NotificationSettings, mgr *notification.Manager) *NotificationSettingsProvider {
	p := &NotificationSettingsProvider{cfg: cfg, defaults: defaults, mgr: mgr}
	p.rebuild()
	return p
}

// LoadSettings 读 notify.settings；DB 无记录 → 返回 defaults（env 引导值，向后兼容 env-only 用户）。
func (p *NotificationSettingsProvider) LoadSettings() NotificationSettings {
	raw, err := p.cfg.Get(notifySettingsKey)
	if err != nil || raw == "" {
		return p.defaults
	}
	var s NotificationSettings
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return p.defaults
	}
	return s
}

// ApplySettings 合并保存（秘密字段空串=保留旧值）后 rebuild。enabled 与非敏感字段整体覆盖。
func (p *NotificationSettingsProvider) ApplySettings(in NotificationSettings) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	merged := mergeNotifySettings(p.LoadSettings(), in)
	blob, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	if err := p.cfg.Set(notifySettingsKey, string(blob)); err != nil {
		return err
	}
	p.rebuildLocked(merged)
	return nil
}

// SetWeChatCredentials 扫码登录成功后调用：仅更新微信 token/accountID 并置 Enabled=true
// （保留 target/baseURL 等其它字段），rebuild。
func (p *NotificationSettingsProvider) SetWeChatCredentials(botToken, accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.LoadSettings()
	s.WeChat.Enabled = true
	if botToken != "" {
		s.WeChat.BotToken = botToken
	}
	if accountID != "" {
		s.WeChat.AccountID = accountID
	}
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := p.cfg.Set(notifySettingsKey, string(blob)); err != nil {
		return err
	}
	p.rebuildLocked(s)
	return nil
}

// rebuild 按当前 LoadSettings 重建通道切片并 Replace 到 Manager。
func (p *NotificationSettingsProvider) rebuild() {
	p.rebuildLocked(p.LoadSettings())
}

// rebuildLocked 由已持锁路径调用；按给定设置构造通道并 Replace。
func (p *NotificationSettingsProvider) rebuildLocked(s NotificationSettings) {
	var chs []notification.Channel
	if s.Bark.Enabled && s.Bark.Key != "" {
		chs = append(chs, notification.NewBark(s.Bark.Server, s.Bark.Key))
	}
	if s.Telegram.Enabled && s.Telegram.Token != "" && s.Telegram.ChatID != "" {
		chs = append(chs, notification.NewTelegram(s.Telegram.Token, s.Telegram.ChatID, s.Telegram.APIURL))
	}
	if s.QQ.Enabled && s.QQ.AppID != "" && s.QQ.AppSecret != "" {
		chs = append(chs, notification.NewQQ(s.QQ.AppID, s.QQ.AppSecret, s.QQ.OpenID, s.QQ.GroupOpenID))
	}
	if s.WeChat.Enabled && s.WeChat.BotToken != "" {
		chs = append(chs, notification.NewWeChatClawBot(s.WeChat.BaseURL, s.WeChat.BotToken, s.WeChat.AccountID, s.WeChat.Target))
	}
	if s.Webhook.Enabled && s.Webhook.URL != "" {
		chs = append(chs, notification.NewWebhook(s.Webhook.URL))
	}
	if s.DingTalk.Enabled && s.DingTalk.URL != "" {
		chs = append(chs, notification.NewDingTalk(s.DingTalk.URL))
	}
	if s.Feishu.Enabled && s.Feishu.URL != "" {
		chs = append(chs, notification.NewFeishu(s.Feishu.URL))
	}
	if s.Email.Enabled && s.Email.Host != "" && s.Email.From != "" && s.Email.To != "" {
		chs = append(chs, notification.NewEmail(s.Email.Host, s.Email.Port, s.Email.Username, s.Email.Password, s.Email.From, s.Email.To))
	}
	// 批次6 补全渠道：按 enabled + 必填非空构建。
	if s.PushPlus.Enabled && s.PushPlus.Token != "" {
		chs = append(chs, notification.NewPushPlus("", s.PushPlus.Token))
	}
	if s.PushDeer.Enabled && s.PushDeer.Pushkey != "" {
		chs = append(chs, notification.NewPushDeer(s.PushDeer.Server, s.PushDeer.Pushkey))
	}
	if s.Ntfy.Enabled && s.Ntfy.Topic != "" {
		chs = append(chs, notification.NewNtfy(s.Ntfy.Server, s.Ntfy.Topic, s.Ntfy.Token))
	}
	if s.Gotify.Enabled && s.Gotify.Server != "" && s.Gotify.Token != "" {
		chs = append(chs, notification.NewGotify(s.Gotify.Server, s.Gotify.Token))
	}
	if s.Apprise.Enabled && s.Apprise.Apikey != "" {
		chs = append(chs, notification.NewApprise(s.Apprise.Server, s.Apprise.Apikey))
	}
	if s.WorkWechat.Enabled && s.WorkWechat.URL != "" {
		chs = append(chs, notification.NewWorkWechat(s.WorkWechat.URL))
	}
	if s.WxPusher.Enabled && s.WxPusher.AppToken != "" && (s.WxPusher.UID != "" || s.WxPusher.TopicID != "") {
		chs = append(chs, notification.NewWxPusher("", s.WxPusher.AppToken, s.WxPusher.UID, s.WxPusher.TopicID))
	}
	if s.Lark.Enabled && s.Lark.URL != "" {
		chs = append(chs, notification.NewLark(s.Lark.URL))
	}
	if s.Discord.Enabled && s.Discord.URL != "" {
		chs = append(chs, notification.NewDiscord(s.Discord.URL))
	}
	if s.MQTT.Enabled && s.MQTT.Broker != "" && s.MQTT.Topic != "" {
		chs = append(chs, notification.NewMQTT(s.MQTT.Broker, s.MQTT.Topic, s.MQTT.Username, s.MQTT.Password, s.MQTT.ClientID))
	}
	if s.ServerChan.Enabled && s.ServerChan.Key != "" {
		chs = append(chs, notification.NewServerChan("", s.ServerChan.Key))
	}
	p.mgr.Replace(chs)
}

// mergeNotifySettings 把 incoming 合并进 current：秘密字段 incoming 空→保留 current，
// 其余（含 Enabled、非敏感字段）取 incoming。便于 UI 只改部分项而不必重输全部秘密。
func mergeNotifySettings(cur, in NotificationSettings) NotificationSettings {
	// Bark：Key 是秘密
	out := cur
	out.Bark.Enabled = in.Bark.Enabled
	out.Bark.Server = in.Bark.Server
	if in.Bark.Key != "" {
		out.Bark.Key = in.Bark.Key
	}
	// Telegram：Token 是秘密
	out.Telegram.Enabled = in.Telegram.Enabled
	out.Telegram.ChatID = in.Telegram.ChatID
	out.Telegram.APIURL = in.Telegram.APIURL
	if in.Telegram.Token != "" {
		out.Telegram.Token = in.Telegram.Token
	}
	// QQ：AppSecret 是秘密
	out.QQ.Enabled = in.QQ.Enabled
	out.QQ.AppID = in.QQ.AppID
	out.QQ.OpenID = in.QQ.OpenID
	out.QQ.GroupOpenID = in.QQ.GroupOpenID
	if in.QQ.AppSecret != "" {
		out.QQ.AppSecret = in.QQ.AppSecret
	}
	// WeChat：BotToken 是秘密（扫码绑定时另走 SetWeChatCredentials，这里允许手填覆盖）
	out.WeChat.Enabled = in.WeChat.Enabled
	out.WeChat.AccountID = in.WeChat.AccountID
	out.WeChat.Target = in.WeChat.Target
	out.WeChat.BaseURL = in.WeChat.BaseURL
	if in.WeChat.BotToken != "" {
		out.WeChat.BotToken = in.WeChat.BotToken
	}
	// Webhook：URL 非秘密但允许清空（取 incoming，含空→禁用/清空）
	out.Webhook.Enabled = in.Webhook.Enabled
	out.Webhook.URL = in.Webhook.URL
	out.DingTalk.Enabled = in.DingTalk.Enabled
	out.DingTalk.URL = in.DingTalk.URL
	out.Feishu.Enabled = in.Feishu.Enabled
	out.Feishu.URL = in.Feishu.URL
	out.Email.Enabled = in.Email.Enabled
	out.Email.Host = in.Email.Host
	out.Email.Port = in.Email.Port
	out.Email.Username = in.Email.Username
	out.Email.From = in.Email.From
	out.Email.To = in.Email.To
	if in.Email.Password != "" {
		out.Email.Password = in.Email.Password
	}
	// PushPlus：Token 秘密
	out.PushPlus.Enabled = in.PushPlus.Enabled
	if in.PushPlus.Token != "" {
		out.PushPlus.Token = in.PushPlus.Token
	}
	// PushDeer：Pushkey 秘密，Server 非敏感
	out.PushDeer.Enabled = in.PushDeer.Enabled
	out.PushDeer.Server = in.PushDeer.Server
	if in.PushDeer.Pushkey != "" {
		out.PushDeer.Pushkey = in.PushDeer.Pushkey
	}
	// Ntfy：Token 秘密，Server/Topic 非敏感
	out.Ntfy.Enabled = in.Ntfy.Enabled
	out.Ntfy.Server = in.Ntfy.Server
	out.Ntfy.Topic = in.Ntfy.Topic
	if in.Ntfy.Token != "" {
		out.Ntfy.Token = in.Ntfy.Token
	}
	// Gotify：Token 秘密，Server 非敏感
	out.Gotify.Enabled = in.Gotify.Enabled
	out.Gotify.Server = in.Gotify.Server
	if in.Gotify.Token != "" {
		out.Gotify.Token = in.Gotify.Token
	}
	// Apprise：Apikey 秘密，Server 非敏感
	out.Apprise.Enabled = in.Apprise.Enabled
	out.Apprise.Server = in.Apprise.Server
	if in.Apprise.Apikey != "" {
		out.Apprise.Apikey = in.Apprise.Apikey
	}
	// WorkWechat：URL 非秘密（同 DingTalk/Feishu 惯例，取 incoming 含空=清空/禁用）
	out.WorkWechat.Enabled = in.WorkWechat.Enabled
	out.WorkWechat.URL = in.WorkWechat.URL
	// WxPusher：AppToken 秘密，UID/TopicID 非敏感
	out.WxPusher.Enabled = in.WxPusher.Enabled
	out.WxPusher.UID = in.WxPusher.UID
	out.WxPusher.TopicID = in.WxPusher.TopicID
	if in.WxPusher.AppToken != "" {
		out.WxPusher.AppToken = in.WxPusher.AppToken
	}
	// Lark：URL 非秘密
	out.Lark.Enabled = in.Lark.Enabled
	out.Lark.URL = in.Lark.URL
	// Discord：URL 非秘密
	out.Discord.Enabled = in.Discord.Enabled
	out.Discord.URL = in.Discord.URL
	// MQTT：Password 秘密，其余非敏感
	out.MQTT.Enabled = in.MQTT.Enabled
	out.MQTT.Broker = in.MQTT.Broker
	out.MQTT.Topic = in.MQTT.Topic
	out.MQTT.Username = in.MQTT.Username
	out.MQTT.ClientID = in.MQTT.ClientID
	if in.MQTT.Password != "" {
		out.MQTT.Password = in.MQTT.Password
	}
	// ServerChan：Key 秘密
	out.ServerChan.Enabled = in.ServerChan.Enabled
	if in.ServerChan.Key != "" {
		out.ServerChan.Key = in.ServerChan.Key
	}
	return out
}
