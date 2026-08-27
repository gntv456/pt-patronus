package chain

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notification"
	"gorm.io/gorm"
)

func newNotifyProviderDB(t *testing.T) *oper.SystemConfigOper {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/notify.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return oper.NewSystemConfigOper(db)
}

// 默认值：TG + Bark 启用，其余关闭。
func notifyDefaults() NotificationSettings {
	var d NotificationSettings
	d.Bark.Enabled = true
	d.Bark.Key = "bark-key"
	d.Telegram.Enabled = true
	d.Telegram.Token = "tg-token"
	d.Telegram.ChatID = "tg-chat"
	return d
}

// TestNotifyProviderLoadDefaults：DB 无记录 → 返回 defaults；rebuild 已注册 TG+Bark。
func TestNotifyProviderLoadDefaults(t *testing.T) {
	cfgOp := newNotifyProviderDB(t)
	mgr := notification.NewManager()
	p := NewNotificationSettingsProvider(cfgOp, notifyDefaults(), mgr)
	s := p.LoadSettings()
	if !s.Bark.Enabled || s.Telegram.Token != "tg-token" {
		t.Errorf("defaults not loaded: %+v", s)
	}
	got := mgr.Channels()
	if len(got) != 2 {
		t.Fatalf("expected bark+telegram registered, got %v", got)
	}
}

// TestNotifyProviderApplyPreservesSecret：空 token → 保留旧值；非秘密字段取新值。
func TestNotifyProviderApplyPreservesSecret(t *testing.T) {
	cfgOp := newNotifyProviderDB(t)
	mgr := notification.NewManager()
	p := NewNotificationSettingsProvider(cfgOp, notifyDefaults(), mgr)

	var in NotificationSettings
	in.Telegram.Enabled = true
	in.Telegram.Token = ""          // 空 → 保留旧 token
	in.Telegram.ChatID = "new-chat" // 非秘密 → 覆盖
	if err := p.ApplySettings(in); err != nil {
		t.Fatal(err)
	}
	s := p.LoadSettings()
	if s.Telegram.Token != "tg-token" {
		t.Errorf("secret not preserved: %q", s.Telegram.Token)
	}
	if s.Telegram.ChatID != "new-chat" {
		t.Errorf("plain not updated: %q", s.Telegram.ChatID)
	}
}

// TestNotifyProviderApplyRebuild：禁用 TG → mgr 不再含 telegram。
func TestNotifyProviderApplyRebuild(t *testing.T) {
	cfgOp := newNotifyProviderDB(t)
	mgr := notification.NewManager()
	p := NewNotificationSettingsProvider(cfgOp, notifyDefaults(), mgr)

	var in NotificationSettings
	in.Telegram.Enabled = false
	in.Telegram.Token = "x"
	in.Telegram.ChatID = "y"
	if err := p.ApplySettings(in); err != nil {
		t.Fatal(err)
	}
	for _, n := range mgr.Channels() {
		if n == "telegram" {
			t.Errorf("telegram should be disabled/removed: %v", mgr.Channels())
		}
	}
}

// TestNotifyProviderApplyEnablesQQ：填齐 QQ 凭据 → 注册 qq。
func TestNotifyProviderApplyEnablesQQ(t *testing.T) {
	cfgOp := newNotifyProviderDB(t)
	mgr := notification.NewManager()
	p := NewNotificationSettingsProvider(cfgOp, NotificationSettings{}, mgr)

	var in NotificationSettings
	in.QQ.Enabled = true
	in.QQ.AppID = "appid"
	in.QQ.AppSecret = "secret"
	in.QQ.OpenID = "openid"
	if err := p.ApplySettings(in); err != nil {
		t.Fatal(err)
	}
	has := false
	for _, n := range mgr.Channels() {
		if n == "qq" {
			has = true
		}
	}
	if !has {
		t.Errorf("qq not registered: %v", mgr.Channels())
	}
}

// TestNotifyProviderSetWeChatCredentials：仅更新 wechat token/accountID 并启用，不动其它通道。
func TestNotifyProviderSetWeChatCredentials(t *testing.T) {
	cfgOp := newNotifyProviderDB(t)
	mgr := notification.NewManager()
	p := NewNotificationSettingsProvider(cfgOp, notifyDefaults(), mgr)

	if err := p.SetWeChatCredentials("wx-token", "wx-acct"); err != nil {
		t.Fatal(err)
	}
	s := p.LoadSettings()
	if !s.WeChat.Enabled || s.WeChat.BotToken != "wx-token" || s.WeChat.AccountID != "wx-acct" {
		t.Errorf("wechat creds not set: %+v", s.WeChat)
	}
	// 其它通道仍保留
	if !s.Bark.Enabled || !s.Telegram.Enabled {
		t.Errorf("other channels disturbed: %+v", s)
	}
}

// TestMergeNotifySettings：逐通道秘密保留 + 非秘密覆盖。
func TestMergeNotifySettings(t *testing.T) {
	var cur NotificationSettings
	cur.Bark.Key = "old-bark"
	cur.QQ.AppSecret = "old-qq"
	cur.WeChat.BotToken = "old-wx"

	var in NotificationSettings
	in.Bark.Enabled = true
	in.Bark.Key = ""     // 保留 old-bark
	in.QQ.AppSecret = "" // 保留 old-qq
	in.QQ.AppID = "new-appid"
	in.WeChat.BotToken = "new-wx" // 覆盖

	out := mergeNotifySettings(cur, in)
	if out.Bark.Key != "old-bark" {
		t.Errorf("bark key: %q", out.Bark.Key)
	}
	if out.QQ.AppSecret != "old-qq" {
		t.Errorf("qq secret: %q", out.QQ.AppSecret)
	}
	if out.QQ.AppID != "new-appid" {
		t.Errorf("qq appid: %q", out.QQ.AppID)
	}
	if out.WeChat.BotToken != "new-wx" {
		t.Errorf("wx token: %q", out.WeChat.BotToken)
	}
}

// TestMergeNewChannels：批次6 渠道秘密保留（incoming 空→保留旧），非秘密取新值。
func TestMergeNewChannels(t *testing.T) {
	var cur NotificationSettings
	cur.PushPlus.Token = "old-pp"
	cur.PushDeer.Pushkey = "old-pd"
	cur.Ntfy.Token = "old-ntfy"
	cur.Gotify.Token = "old-gotify"
	cur.Apprise.Apikey = "old-appr"
	cur.WxPusher.AppToken = "old-wxp"
	cur.MQTT.Password = "old-mqtt"
	cur.ServerChan.Key = "old-sc"

	var in NotificationSettings
	// 全部 enabled + 秘密留空（应保留旧）+ 部分非秘密更新。
	in.PushPlus.Enabled = true
	in.PushDeer.Enabled = true
	in.PushDeer.Server = "https://pd.new"
	in.Ntfy.Enabled = true
	in.Ntfy.Topic = "newtopic"
	in.Gotify.Enabled = true
	in.Gotify.Server = "https://gotify.new"
	in.Apprise.Enabled = true
	in.WxPusher.Enabled = true
	in.WxPusher.UID = "uid-new"
	in.MQTT.Enabled = true
	in.MQTT.Broker = "broker.new:1883"
	in.ServerChan.Enabled = true
	in.WorkWechat.Enabled = true
	in.WorkWechat.URL = "https://qyapi/new"
	in.Lark.Enabled = true
	in.Lark.URL = "https://lark/new"
	in.Discord.Enabled = true
	in.Discord.URL = "https://discord/new"

	out := mergeNotifySettings(cur, in)
	// 秘密保留
	if out.PushPlus.Token != "old-pp" {
		t.Errorf("pushplus token: %q", out.PushPlus.Token)
	}
	if out.PushDeer.Pushkey != "old-pd" {
		t.Errorf("pushdeer pushkey: %q", out.PushDeer.Pushkey)
	}
	if out.Ntfy.Token != "old-ntfy" {
		t.Errorf("ntfy token: %q", out.Ntfy.Token)
	}
	if out.Gotify.Token != "old-gotify" {
		t.Errorf("gotify token: %q", out.Gotify.Token)
	}
	if out.Apprise.Apikey != "old-appr" {
		t.Errorf("apprise apikey: %q", out.Apprise.Apikey)
	}
	if out.WxPusher.AppToken != "old-wxp" {
		t.Errorf("wxpusher appToken: %q", out.WxPusher.AppToken)
	}
	if out.MQTT.Password != "old-mqtt" {
		t.Errorf("mqtt password: %q", out.MQTT.Password)
	}
	if out.ServerChan.Key != "old-sc" {
		t.Errorf("serverchan key: %q", out.ServerChan.Key)
	}
	// 非秘密覆盖
	if out.PushDeer.Server != "https://pd.new" {
		t.Errorf("pushdeer server: %q", out.PushDeer.Server)
	}
	if out.Ntfy.Topic != "newtopic" {
		t.Errorf("ntfy topic: %q", out.Ntfy.Topic)
	}
	if out.WorkWechat.URL != "https://qyapi/new" {
		t.Errorf("workwechat url: %q", out.WorkWechat.URL)
	}
}

// TestNotifyProviderApplyEnablesNewChannels：填齐凭据 → pushplus/serverchan/mqtt/discord 注册。
func TestNotifyProviderApplyEnablesNewChannels(t *testing.T) {
	cfgOp := newNotifyProviderDB(t)
	mgr := notification.NewManager()
	p := NewNotificationSettingsProvider(cfgOp, NotificationSettings{}, mgr)

	var in NotificationSettings
	in.PushPlus.Enabled = true
	in.PushPlus.Token = "pp"
	in.ServerChan.Enabled = true
	in.ServerChan.Key = "sc"
	in.Discord.Enabled = true
	in.Discord.URL = "https://discord/webhook"
	in.Slack.Enabled = true
	in.Slack.URL = "https://hooks.slack.com/services/T/B/X"
	in.MQTT.Enabled = true
	in.MQTT.Broker = "broker:1883"
	in.MQTT.Topic = "t"
	if err := p.ApplySettings(in); err != nil {
		t.Fatal(err)
	}
	chs := mgr.Channels()
	want := map[string]bool{"pushplus": false, "serverchan": false, "discord": false, "slack": false, "mqtt": false}
	for _, n := range chs {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for k, got := range want {
		if !got {
			t.Errorf("channel %q not registered; got %v", k, chs)
		}
	}

	// 再禁用 PushPlus（Token 留空→保留，但 Enabled=false）→ 不应再注册。
	in2 := in
	in2.PushPlus.Enabled = false
	if err := p.ApplySettings(in2); err != nil {
		t.Fatal(err)
	}
	for _, n := range mgr.Channels() {
		if n == "pushplus" {
			t.Errorf("pushplus should be removed after disable: %v", mgr.Channels())
		}
	}
}
