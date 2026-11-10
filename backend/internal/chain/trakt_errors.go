// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import "errors"

// 三条「用户自己能修好」的 Trakt 失败。从前接口层把它们压成
// 400「请求参数有误，请检查后重试」——而 /trakt/device-code 与 /trakt/sync 两个 POST
// 根本没有请求体，用户没有任何可检查的输入。
//
// 哨兵只用于判档；给人看的那半句（去哪儿填 / 先点发起授权 / 重新拿码）由 %w 后面现写，
// 接口层 WithDetail 原样送到界面。单独成文件：trakt.go / trakt_oauth.go 各管自己的事。
var (
	// ErrTraktNotConfigured 是没填 client_id/client_secret：这一步只能在 trakt.tv 做，
	// 重试永远不会有用，必须报成「未配置」而不是「上游不可用」。
	ErrTraktNotConfigured = errors.New("Trakt 应用凭据未配置")
	// ErrTraktNotAuthorized 是凭据有了但还没走完设备授权（点同步必然空手而归）。
	ErrTraktNotAuthorized = errors.New("Trakt 尚未完成设备授权")
	// ErrTraktCodeExpired 是 pending 授权码过期：面板还在每 3s 轮询就是白等，
	// 前端要靠这一档停下来。
	ErrTraktCodeExpired = errors.New("Trakt 设备授权码已过期")
)
