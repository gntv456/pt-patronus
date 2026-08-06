package chain

import (
	"strings"
)

// IYUUTokenKey 是 SystemConfig 中 IYUU token 的键。
const IYUUTokenKey = "iyuu_token"

// Torznab 配置键（Jackett/Prowlarr 端点，可选辅种候选源）。
const (
	TorznabURLKey    = "torznab_url"
	TorznabAPIKeyKey = "torznab_apikey"
)

// GetToken 读 IYUU token（空表示未配置）。
func (c *KeepSeedChain) GetToken() (string, error) { return c.cfgOp.Get(IYUUTokenKey) }

// SetToken 写 IYUU token。
func (c *KeepSeedChain) SetToken(token string) error { return c.cfgOp.Set(IYUUTokenKey, token) }

// GetTorznab 读 Torznab 端点配置。
func (c *KeepSeedChain) GetTorznab() (baseURL, apiKey string, err error) {
	baseURL, err = c.cfgOp.Get(TorznabURLKey)
	if err != nil {
		return "", "", err
	}
	apiKey, err = c.cfgOp.Get(TorznabAPIKeyKey)
	return baseURL, apiKey, err
}

// SetTorznab 写 Torznab 端点配置。
// url 空：清空 url+key；url 非空且 apiKey 空：只更新 url、保留已有 key（避免前端仅改 URL 时把 key 冲掉）。
func (c *KeepSeedChain) SetTorznab(baseURL, apiKey string) error {
	baseURL = strings.TrimSpace(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	if err := c.cfgOp.Set(TorznabURLKey, baseURL); err != nil {
		return err
	}
	if apiKey != "" {
		return c.cfgOp.Set(TorznabAPIKeyKey, apiKey)
	}
	if baseURL == "" {
		return c.cfgOp.Set(TorznabAPIKeyKey, "")
	}
	return nil // 保留旧 key
}
