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
	return out
}
