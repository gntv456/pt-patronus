// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"errors"

	"github.com/hxpt/ptpatronus/internal/site"
)

// sunnypt_keepalive.go — GenericJSON 的 sunnypt 专用 KeepAlive（实现 site.KeepAliveCapable）。
//
// Sunny 是 Next.js SPA + Bearer JWT 鉴权，保活语义与 NexusPHP 不同：
//   - AuthSession（NextAuth 会话 cookie，HttpOnly）会被服务端回收 → sunnyptEnsureSession 用
//     accessToken 换一份新 AuthSession 并合并回写（主防御，续「会话层」）。
//   - accessToken（AuthSession payload 内的自包含 JWT）才是打 api.sunnypt.top 的真正凭证。
//     用 sunnyGet 打一次 /user/details/info 验证它仍有效：401/403 → LoggedIn=false。
//
// 返回：
//   - LoggedIn=true  ：accessToken 仍有效，RefreshedCookie=合并后的新 cookie（AuthSession 已轮换）。
//   - LoggedIn=false ：accessToken 已过期，只能靠 CookieCloud 拉浏览器新登录的 cookie 恢复；
//     此时不回写 cookie（RefreshedCookie 留空，避免用旧值污染 DB）。
//
// 非 sunnypt 站返回 ErrNotSupported（GenericJSON 被 tnode/yemapt 等多站共用，勿冒充保活）。

// KeepAlive 实现 site.KeepAliveCapable（仅 sunnypt 分支生效）。
func (g *GenericJSON) KeepAlive(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.KeepAliveResult, error) {

	if def.ID != "sunnypt" {
		return nil, site.ErrNotSupported
	}
	// accessToken 自包含校验：用【原 DB cookie】里的 accessToken 打数据 API，验证它没过期。
	// 必须用原 cookie 而非续会话后的 cookie——sunnyGet 从 cookie 解析 accessToken，而
	// sunnyptEnsureSession 返回的串只保证 AuthSession 干净，不保证 accessToken 可再解析。
	apiBase := sunnyAPIBase(baseURL)
	if _, _, err := g.sunnyGet(ctx, apiBase, cookie, "/api/v1/user/details/info"); err != nil {
		// ErrNotLoggedIn = accessToken 已死；其它网络错误保守视为未掉线（让调用方重试）。
		if errors.Is(err, site.ErrNotLoggedIn) {
			return &site.KeepAliveResult{LoggedIn: false}, nil
		}
		return nil, err
	}

	// accessToken 仍有效 → 刷新 AuthSession 会话层，拿轮换后的 cookie 回写。
	// sunnyptEnsureSession 内部用原 accessToken 换新 AuthSession 并合并；失败则用 clean cookie 兜底。
	refreshed, _ := g.sunnyptEnsureSession(ctx, baseURL, cookie)
	return &site.KeepAliveResult{LoggedIn: true, RefreshedCookie: refreshed}, nil
}
