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
	in.Bark.Key = "" // 保留 old-bark
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
