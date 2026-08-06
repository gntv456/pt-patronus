package chain

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// username+password+pid 打包成 JSON 单串（适配器 parseAvistazCred 拆），其余站点用 cookie。
// 集中 schema 感知，未来更多凭据类型可在此扩展。
func (c *SiteChain) credential(s *models.Site, def *site.SiteMetadata) string {
	if def != nil {
		switch def.Schema {
		case "mteam":
			if k, err := c.sites.APIKey(s); err == nil {
				return k
			}
			return ""
		case "yemapt":
			if k, err := c.sites.APIKey(s); err == nil && k != "" {
				return k
			}
			cookie, _ := c.sites.Cookie(s)
			return cookie
		case "gazelle_jsonapi":
			if def.ID == "gazellegames" {
				if k, err := c.sites.APIKey(s); err == nil && k != "" {
					return k
				}
			}
			cookie, _ := c.sites.Cookie(s)
			return cookie
		case "f3nix":
			apiKey, _ := c.sites.APIKey(s)
			rssKey := c.sites.DecryptedPasskey(s)
			cookie, _ := c.sites.Cookie(s)
			if apiKey == "" && rssKey == "" {
				return cookie
			}
			b, _ := json.Marshal(map[string]string{
				"api_key": apiKey,
				"rsskey":  rssKey,
				"cookie":  cookie,
			})
			return string(b)
		case "fsm":
			token, _ := c.sites.APIKey(s)
			passkey := c.sites.DecryptedPasskey(s)
			cookie, _ := c.sites.Cookie(s)
			if token == "" && passkey == "" {
				return cookie
			}
			b, _ := json.Marshal(map[string]string{
				"token":   token,
				"passkey": passkey,
				"cookie":  cookie,
			})
			return string(b)
		case "rousi":
			if pk := c.sites.DecryptedPasskey(s); pk != "" {
				return pk
			}
			if k, err := c.sites.APIKey(s); err == nil && k != "" {
				return k
			}
			cookie, _ := c.sites.Cookie(s)
			return cookie
		case "avistaz":
			// Avistaz 搜索用 username/password/pid；AnimeZ 用户资料可 best-effort 复用 cookie 抓网页 profile。
			pwd, _ := c.sites.Password(s)
			pid := c.sites.DecryptedPasskey(s)
			cookie, _ := c.sites.Cookie(s)
			b, _ := json.Marshal(map[string]string{
				"username": s.Username,
				"password": pwd,
				"pid":      pid,
				"cookie":   cookie,
			})
			return string(b)
		}
	}
	cookie, _ := c.sites.Cookie(s)
	return cookie
}

// dispatchAdapter 找到匹配 schema 的适配器并执行 fn。
// 区分两种情况：无适配器匹配返回 ErrNoAdapter；匹配但 fn 失败则返回该真实错误。
func (c *SiteChain) dispatchAdapter(ctx context.Context, schema string, fn func(site.Adapter) error) error {
	var (
		matched bool
		fnErr   error
	)
	RunModule(ctx, &c.Base, func(m module.Module) (struct{}, module.Control) {
		a, ok := m.(site.Adapter)
		if !ok || a.Schema() != schema {
			return struct{}{}, module.Skip
		}
		matched = true
		fnErr = fn(a)
		return struct{}{}, module.Stop
	})
	if !matched {
		return ErrNoAdapter
	}
	return fnErr
}

// ErrNoAdapter 表示没有已注册的适配器能处理该 schema。
var ErrNoAdapter = errors.New("no registered adapter for site schema")

// ErrNotSupported 表示适配器不支持该操作（如签到）。与 site.ErrNotSupported 同一 sentinel，
// 便于适配器返回 site.ErrNotSupported 时 chain 用 errors.Is 识别并跳过落库。
var ErrNotSupported = site.ErrNotSupported
