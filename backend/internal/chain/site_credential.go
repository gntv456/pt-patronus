package chain

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// credentialWithErr 打包站点凭据；任一解密失败都返回可判别的错误。
//
// #377 从前这个函数是 `cookie, _ := c.sites.Cookie(s)`（9 处）——错误被丢掉、
// 返回值退化成 ""，于是 20 个调用方（消息/H&R/签到/搜索/种子/快照…）拿着**空凭据**
// 真的去请求站点。主密钥换过或密文损坏时，用户看到的是「消息拉不出来」「签到失败」
// 「搜索无结果」，而真相是本地解不开凭据 —— 全被误读成站外行为。
//
// 同一包的娱乐域早就修过这个形状（entertainment_capability.go:20 的注释点名了
// `cookie, _ := siteOp.Cookie(s)`），那次只覆盖 4 个出口；这里是它的通用版本。
//
// 关键前提（沿用那边的论证）：Decrypt("") 归一成 ("", nil)，
// 所以非 nil error 一定是真故障，不会把「本来就没配凭据」的站点误判成故障 ——
// 反向划界见 TestCredentialNoCookieIsNotFailure。
//
// username+password+pid 打包成 JSON 单串（适配器 parseAvistazCred 拆），其余站点用 cookie。
// 集中 schema 感知，未来更多凭据类型可在此扩展。
func (c *SiteChain) credentialWithErr(s *models.Site, def *site.SiteMetadata) (string, error) {
	if def != nil {
		switch def.Schema {
		case "mteam":
			if k, err := c.sites.APIKey(s); err == nil {
				return k, nil
			}
			return "", siteCredUnreadable(s.Name, "APIKey", errors.New("APIKey 解密失败"))
		case "piggo":
			// 猪猪：SRP 加密登录需 username+password（无 Cookie/passkey 通道）；打包 JSON
			// 单串供适配器 parsePiggoCred 拆。
			pwd, perr := c.sites.Password(s)
			if perr != nil {
				return "", siteCredUnreadable(s.Name, "密码", perr)
			}
			b, _ := json.Marshal(map[string]string{
				"username": s.Username,
				"password": pwd,
			})
			return string(b), nil
		case "yemapt":
			if k, err := c.sites.APIKey(s); err == nil && k != "" {
				return k, nil
			}
			return c.siteCookie(s)
		case "gazelle_jsonapi":
			if def.ID == "gazellegames" {
				if k, err := c.sites.APIKey(s); err == nil && k != "" {
					return k, nil
				}
			}
			return c.siteCookie(s)
		case "f3nix":
			apiKey, aerr := c.sites.APIKey(s)
			if aerr != nil {
				return "", siteCredUnreadable(s.Name, "APIKey", aerr)
			}
			rssKey, perr := c.sitePasskey(s)
			if perr != nil {
				return "", perr
			}
			cookie, cerr := c.sites.Cookie(s)
			if cerr != nil {
				return "", siteCredUnreadable(s.Name, "Cookie", cerr)
			}
			if apiKey == "" && rssKey == "" {
				return cookie, nil
			}
			b, _ := json.Marshal(map[string]string{
				"api_key": apiKey,
				"rsskey":  rssKey,
				"cookie":  cookie,
			})
			return string(b), nil
		case "fsm":
			token, terr := c.sites.APIKey(s)
			if terr != nil {
				return "", siteCredUnreadable(s.Name, "APIKey", terr)
			}
			passkey, perr := c.sitePasskey(s)
			if perr != nil {
				return "", perr
			}
			cookie, cerr := c.sites.Cookie(s)
			if cerr != nil {
				return "", siteCredUnreadable(s.Name, "Cookie", cerr)
			}
			if token == "" && passkey == "" {
				return cookie, nil
			}
			b, _ := json.Marshal(map[string]string{
				"token":   token,
				"passkey": passkey,
				"cookie":  cookie,
			})
			return string(b), nil
		case "rousi":
			// passkey 解不开必须报出来，不许 `perr == nil && pk != ""` 那样把它悄悄当「没配」
			// 再退到 APIKey —— 那样只配了 passkey 的站会一路退到 siteCookie()、回空凭据 + nil。
			pk, perr := c.sitePasskey(s)
			if perr != nil {
				return "", perr
			}
			if pk != "" {
				return pk, nil
			}
			if k, err := c.sites.APIKey(s); err == nil && k != "" {
				return k, nil
			}
			return c.siteCookie(s)
		case "fluxtorrent":
			// FluxTorrent：搜索/用户信息走 fxo_ Token（APIKey 列），下载链走 passkey；
			// cookie 对兼容层不可用，仅打包透传（适配器忽略）。
			// token 缺失也打包成 JSON 而非裸返回 passkey——否则适配器 parse 会把 passkey
			// 当 Token 使（`Authorization: Token <passkey>`），用户看到的是「未登录/凭据失效」
			// 而不是「请填写 API Token」。
			token, terr := c.sites.APIKey(s)
			if terr != nil {
				return "", siteCredUnreadable(s.Name, "APIKey", terr)
			}
			passkey, perr := c.sitePasskey(s)
			if perr != nil {
				return "", perr
			}
			b, _ := json.Marshal(map[string]string{
				"token":   token,
				"passkey": passkey,
			})
			return string(b), nil
		case "avistaz":
			// Avistaz 搜索用 username/password/pid；AnimeZ 用户资料可 best-effort 复用 cookie 抓网页 profile。
			pwd, perr := c.sites.Password(s)
			if perr != nil {
				return "", siteCredUnreadable(s.Name, "密码", perr)
			}
			pid, perr := c.sitePasskey(s)
			if perr != nil {
				return "", perr
			}
			cookie, cerr := c.sites.Cookie(s)
			if cerr != nil {
				return "", siteCredUnreadable(s.Name, "Cookie", cerr)
			}
			b, _ := json.Marshal(map[string]string{
				"username": s.Username,
				"password": pwd,
				"pid":      pid,
				"cookie":   cookie,
			})
			return string(b), nil
		}
	}
	return c.siteCookie(s)
}

// passkey 取站点 passkey 并把解密失败变成可判别的错误。
//
// #383 从前这里用的是 oper 层的 DecryptedPasskey（`pk, _ := o.Passkey(s)`）：
// 错误被丢掉，返回值退化成 ""，于是「没配 passkey」与「passkey 解不开」在
// credentialWithErr 里完全同形。后果是**只配了 passkey 的站点**（FluxTorrent /
// Rousi 就是这个用法：搜索要 token、下载要 passkey）在换过主密钥后：
//
//	f3nix / fsm / rousi   → err=nil, cred=""
//	fluxtorrent           → err=nil, cred=`{"passkey":"","token":""}`
//	avistaz               → err=nil, cred=`{"cookie":"","password":"","pid":"","username":""}`
//
// 空凭据照发请求，用户看到的是「搜索无结果」「下载链接拿不到」—— 与 #377 修掉的那批
// 症状一字不差。#377 之所以没抓到这一格：它配齐了 cookie+apiKey+passkey 写判据，
// Cookie/APIKey 会替我们报错，把 passkey 这列的漏洞盖住了；而真实用户里
// 「只填 passkey」恰恰是最常见的最小配置。
//
// 划界与 Cookie 那一列同源：Passkey(s) 对 s.PasskeyEnc=="" 返回 ("", nil)
// （Decrypt("") 归一成 ("", nil)），所以非 nil error 一定是真故障。
func (c *SiteChain) sitePasskey(s *models.Site) (string, error) {
	pk, err := c.sites.Passkey(s)
	if err != nil {
		return "", siteCredUnreadable(s.Name, "Passkey", err)
	}
	return pk, nil
}

// siteCookie 取站点 cookie 并把解密失败变成可判别的错误。
// 「没配 cookie」（Decrypt("") → ("", nil)）照旧返回空串、不算故障。
func (c *SiteChain) siteCookie(s *models.Site) (string, error) {
	cookie, err := c.sites.Cookie(s)
	if err != nil {
		return "", siteCredUnreadable(s.Name, "Cookie", err)
	}
	return cookie, nil
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
