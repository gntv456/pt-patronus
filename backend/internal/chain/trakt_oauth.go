// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hxpt/ptpatronus/internal/trakt"
)

// StartDeviceAuth 发起设备授权，返回 device code + 把 pending 状态存库（前端展示 user_code/url）。
func (c *TraktChain) StartDeviceAuth(ctx context.Context) (TraktSettings, error) {
	s := c.GetSettings()
	if s.ClientID == "" || s.ClientSecret == "" {
		return s, fmt.Errorf("%w：请到「设置 → Trakt」填入 client_id/client_secret（先在 trakt.tv 建应用）；本次没有向 Trakt 发请求", ErrTraktNotConfigured)
	}
	dc, err := c.client.DeviceCode(ctx, trakt.Credentials{ClientID: s.ClientID, ClientSecret: s.ClientSecret})
	if err != nil {
		return s, err
	}
	s.PendingDeviceCode = dc.DeviceCode
	s.PendingUserCode = dc.UserCode
	s.PendingVerificationURL = dc.VerificationURL
	s.PendingExpiresAt = time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second).Unix()
	s.PendingInterval = dc.Interval
	s.LastError = ""
	// 只写 pending_*：DeviceCode 是一次跨网络 HTTP，期间用户可能已改过别的设置。
	if err := c.patchSettings(func(cur *TraktSettings) {
		cur.PendingDeviceCode = s.PendingDeviceCode
		cur.PendingUserCode = s.PendingUserCode
		cur.PendingVerificationURL = s.PendingVerificationURL
		cur.PendingExpiresAt = s.PendingExpiresAt
		cur.PendingInterval = s.PendingInterval
		cur.LastError = ""
	}); err != nil {
		return s, err
	}
	return s, nil
}

// PollDeviceStatus 轮询设备授权：若用户已完成授权则换到令牌并落库；未完成返 (false, nil)。
func (c *TraktChain) PollDeviceStatus(ctx context.Context) (bool, error) {
	s := c.GetSettings()
	if s.PendingDeviceCode == "" {
		return s.AccessToken != "", nil
	}
	if time.Now().Unix() > s.PendingExpiresAt {
		_ = c.patchSettings(func(cur *TraktSettings) { cur.PendingDeviceCode = "" })
		return false, fmt.Errorf("%w（授权码有效期到点就作废）——请重新点「发起授权」拿一张新码", ErrTraktCodeExpired)
	}
	tok, err := c.client.PollDeviceToken(ctx, s.PendingDeviceCode,
		trakt.Credentials{ClientID: s.ClientID, ClientSecret: s.ClientSecret})
	if err != nil {
		var dte *trakt.DeviceTokenError
		if errors.As(err, &dte) {
			// authorization_pending / slow_down：未完成，保持 pending
			return false, nil
		}
		return false, err
	}
	s.AccessToken = tok.AccessToken
	s.RefreshToken = tok.RefreshToken
	s.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).Unix()
	s.PendingDeviceCode = ""
	s.PendingUserCode = ""
	s.LastError = ""
	// 令牌与 pending 由授权路径写，其余字段保留库里最新值（授权期间用户可能已改设置）。
	if err := c.patchSettings(func(cur *TraktSettings) {
		cur.AccessToken = s.AccessToken
		cur.RefreshToken = s.RefreshToken
		cur.ExpiresAt = s.ExpiresAt
		cur.PendingDeviceCode = ""
		cur.PendingUserCode = ""
		cur.LastError = ""
	}); err != nil {
		return false, err
	}
	return true, nil
}

// refreshTokenIfNeeded 令牌临近过期则刷新；401 后 force 强制刷新。
// 失败分类处理：服务端明确拒绝（400/401，refresh_token 已死）→ 清令牌要求重新授权；
// 瞬时故障（网络/超时/5xx/429）→ 保留令牌记错误，下次同步重试。
func (c *TraktChain) refreshTokenIfNeeded(ctx context.Context, s *TraktSettings, force bool) error {
	if s.AccessToken == "" {
		return errors.New("未授权，请先完成设备授权")
	}
	if !force && (s.ExpiresAt == 0 || time.Now().Unix() < s.ExpiresAt-60) {
		return nil // 未到期（force=true 时跳过，用于 401 后强制刷新）
	}
	if s.RefreshToken == "" {
		return errors.New("令牌过期且无 refresh_token，请重新授权")
	}
	tok, err := c.client.RefreshToken(ctx, s.RefreshToken, s.RedirectURI,
		trakt.Credentials{ClientID: s.ClientID, ClientSecret: s.ClientSecret})
	if err != nil {
		var re *trakt.RefreshError
		if errors.As(err, &re) && re.AuthDead {
			s.AccessToken = ""
			s.RefreshToken = ""
			s.LastError = "刷新令牌失败：" + err.Error()
			_ = c.patchSettings(func(cur *TraktSettings) {
				cur.AccessToken = ""
				cur.RefreshToken = ""
				cur.LastError = s.LastError
			})
			return errors.New("刷新令牌失败，请重新授权")
		}
		// 瞬时故障：保留令牌稍后重试——此前任何失败都清空令牌，
		// 一次网络抖动即永久停摆、强制用户重新走设备授权。
		s.LastError = "刷新令牌失败（瞬时故障，将重试）：" + err.Error()
		_ = c.patchSettings(func(cur *TraktSettings) { cur.LastError = s.LastError })
		return fmt.Errorf("刷新令牌失败（瞬时故障，已保留令牌稍后重试）：%w", err)
	}
	s.AccessToken = tok.AccessToken
	s.RefreshToken = tok.RefreshToken
	s.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).Unix()
	// Trakt 的 refresh_token 一次一换：新令牌必须立刻落盘，调用方随后写什么都不能把它带走。
	return c.patchSettings(func(cur *TraktSettings) {
		cur.AccessToken = s.AccessToken
		cur.RefreshToken = s.RefreshToken
		cur.ExpiresAt = s.ExpiresAt
		cur.LastError = ""
	})
}
