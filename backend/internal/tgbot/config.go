// Package tgbot 实现入站 Telegram bot：getUpdates 长轮询收消息 → 斜杠命令或自然语言 →
// 复用 AI Agent（经 TalkFunc 注入，避免 tgbot→aiagent 依赖）→ 回帖。
// 与出站推送 notification/telegram.go 分离（bot 不是 Channel）。对标 MoviePilot telegram + chat-ops。
package tgbot

import "encoding/json"

// KVStore 抽象 oper.SystemConfigOper，与 aiagent/llmgateway 同构。
type KVStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Crypto 抽象 security.Crypto 的加解密。
type Crypto interface {
	Decrypt(enc string) (string, error)
	Encrypt(plain string) (string, error)
}

// Config bot 配置（存 SystemConfig `tgbot.config`，BotToken AES-GCM 加密）。
type Config struct {
	Enabled        bool    `json:"enabled"`
	BotTokenEnc    string  `json:"bot_token_enc"` // 加密的 BotFather token
	AllowedUserIDs []int64 `json:"allowed_user_ids"`
	AllowGroupChat bool    `json:"allow_group_chat"`
	APIURL         string  `json:"api_url"` // 自定义 Bot API 域名（国内反代）；空→官方
}

// ConfigKey SystemConfig 存储键。
const ConfigKey = "tgbot.config"

// DefaultConfig 默认配置。
func DefaultConfig() Config { return Config{Enabled: false, APIURL: ""} }

// Load 从 KV 读配置；缺失/损坏回退默认。
func Load(kv KVStore) Config {
	def := DefaultConfig()
	if kv == nil {
		return def
	}
	raw, err := kv.Get(ConfigKey)
	if err != nil || raw == "" {
		return def
	}
	var c Config
	if json.Unmarshal([]byte(raw), &c) != nil {
		return def
	}
	return c
}

// Save 写配置到 KV。
func Save(kv KVStore, c Config) error {
	if kv == nil {
		return nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return kv.Set(ConfigKey, string(b))
}

// Redacted 返回脱敏配置（token 仅留 _configured）。
func Redacted(c Config) map[string]any {
	ids := c.AllowedUserIDs
	if ids == nil {
		ids = []int64{}
	}
	return map[string]any{
		"enabled":              c.Enabled,
		"bot_token_configured": c.BotTokenEnc != "",
		"allowed_user_ids":     ids,
		"allow_group_chat":     c.AllowGroupChat,
		"api_url":              c.APIURL,
	}
}

// BotToken 解密 BotFather token；未配置/失败返空。
func BotToken(c Config, cr Crypto) string {
	if cr == nil || c.BotTokenEnc == "" {
		return ""
	}
	if t, err := cr.Decrypt(c.BotTokenEnc); err == nil {
		return t
	}
	return ""
}
