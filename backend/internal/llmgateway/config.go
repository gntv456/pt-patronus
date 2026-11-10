// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package llmgateway 把现有 AI Agent 以 OpenAI / Anthropic 兼容协议对外暴露，
// 供 Cursor / Claude Desktop / continue.dev 等外部客户端直接驱动 PTPatronus 的工具。
// 本包只做协议转换（agent 事件流 → SSE chunk）+ 配置存取，不引外部 SDK。
package llmgateway

import (
	"encoding/json"
	"strings"
)

// KVStore 抽象 oper.SystemConfigOper（Get/Set by key），与 aiagent.KVStore 同构。
type KVStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Crypto 抽象 security.Crypto 的加解密，与 aiagent.Crypto 同构。
type Crypto interface {
	Decrypt(enc string) (string, error)
	Encrypt(plain string) (string, error)
}

// Config 是网关配置（存 SystemConfig `llmgateway.config`，访问令牌 AES-GCM 加密）。
type Config struct {
	Enabled        bool     `json:"enabled"`
	AccessTokenEnc string   `json:"access_token_enc"` // 加密的访问令牌；运行时解密比对
	AllowedModels  []string `json:"allowed_models"`   // 空切片=接受任意 model
	MaxConcurrency int      `json:"max_concurrency"`  // 并发代理上限（0=不限）
}

// ConfigKey 是 SystemConfig 中的存储键。
const ConfigKey = "llmgateway.config"

// DefaultConfig 返回默认配置。
func DefaultConfig() Config {
	return Config{Enabled: false, MaxConcurrency: 4}
}

// Load 从 KV 读配置；缺失/损坏回退默认。
func Load(kv KVStore) Config {
	def := DefaultConfig()
	if kv == nil {
		return def
	}
	raw, err := kv.Get(ConfigKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return def
	}
	var c Config
	if json.Unmarshal([]byte(raw), &c) != nil {
		return def
	}
	if c.MaxConcurrency <= 0 {
		c.MaxConcurrency = def.MaxConcurrency
	}
	if c.AllowedModels == nil {
		c.AllowedModels = []string{}
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

// Redacted 返回脱敏配置（令牌仅留 _configured 布尔）供 GET。
func Redacted(c Config) map[string]any {
	return map[string]any{
		"enabled":                 c.Enabled,
		"allowed_models":          c.AllowedModels,
		"max_concurrency":         c.MaxConcurrency,
		"access_token_configured": strings.TrimSpace(c.AccessTokenEnc) != "",
	}
}

// ModelAllowed 校验请求 model 是否被接受：AllowedModels 空=全放行。
func (c Config) ModelAllowed(model string) bool {
	if len(c.AllowedModels) == 0 {
		return true
	}
	for _, m := range c.AllowedModels {
		if m == model {
			return true
		}
	}
	return false
}

// AccessToken 解密访问令牌；未配置/解密失败返空。
func AccessToken(c Config, cr Crypto) string {
	if cr == nil || strings.TrimSpace(c.AccessTokenEnc) == "" {
		return ""
	}
	if t, err := cr.Decrypt(c.AccessTokenEnc); err == nil {
		return t
	}
	return ""
}
