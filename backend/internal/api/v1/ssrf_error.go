// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// respondFetchError 把「站点抓取失败」按**能不能重试 / 该去做什么**分档。
//
// 它是 dashboard.SyncOne 与 message 四个出口的共同出口（#398），而这五处的上游
// 都会先经 chain.SiteChain.credentialWithErr 取凭据 —— 也就是说「凭据解不开」
// 原本会从这儿一路混进通用话术。
//
// 三个分支：
//
//	ErrSiteCredentialUnreadable → 503 专属码（#398）。主密钥换过/密文损坏，
//	    重试一万次也不会好，而且**本次没向站点发请求**——这是用户判断能否放心
//	    重试、站点那边有没有出事的唯一依据。套「上游不可用」会让人一直重试。
//	ErrBlocked（SSRF 守卫）    → 站点建在内网被拦。这是这套工具的常规用法
//	    （ALLOW_PRIVATE_SITES 开关就是为它存在的），而守卫默认关闭：从前这类请求
//	    只会得到「服务内部错误，请稍后重试」，可重试永远不会有用，用户唯一能做的
//	    动作（换地址或开开关重启）从头到尾没被说出来。
//	其余                        → 调用方给的那一档兜底（BadGateway / Internal）。
func respondFetchError(c *gin.Context, err error, fallback *errcode.Error) {
	if errors.Is(err, chain.ErrSiteCredentialUnreadable) {
		// WithDetail：链层那句本就写给人看（点名站点/凭据/下一步），原样抵达界面。
		errcode.Respond(c, errcode.SiteCredentialUnreadable.WithDetail(err))
		return
	}
	if errors.Is(err, httpclient.ErrBlocked) {
		errcode.Respond(c, errcode.SSRFBlocked.WithErr(err))
		return
	}
	errcode.Respond(c, fallback.WithErr(err))
}
