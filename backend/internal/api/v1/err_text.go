// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"gorm.io/gorm"
)

// userErrText 是软失败面（HTTP 200 + {ok:false,error}，以及每站一行的结果表）的翻译口。
// 实现住在 internal/errtext：链层往结果结构体里写 Error 时用的是同一个函数，
// 免得 HTTP 面翻好了、逐条结果面还在弹 "record not found" / "ssrf: …" 这种原文。
func userErrText(err error) string { return errtext.Text(err) }

// respondReasonErr 硬失败面里「链层已经把原因说清楚」的出口。
//
// WithErr 只把底层错误挂到 Err 上（供日志），一个字都不动 Message；而 Respond 把
// message/error 两个字段都写成码的默认文案。于是链层那句可操作的中文（例如 #324 的
// 「下载器（编号 2）里没有这颗种子…」）被整段丢弃，界面只弹一句通用话术——
// 用户既不知道该做什么，也以为服务坏了。这是本仓反复出现的「把原因钉成一个猜测」
// （同族：#214/#217/#335）。
//
// 这里用 WithDetail：原因保留在 Message 里给用户看，同时留在 Err 里给日志定位。
// 只对本仓自己写给用户的自产错误使用——站点适配器/下载器客户端的原文仍走 WithErr
// 原样透传（那是用户唯一的线索，不该被二次加工）。
//
// notFound 是「资源不存在」那一档的码：调用方按自己的域给（历史不存在用
// TransferHistoryNotFound，种子不在下载器上用 NotFound），本函数只负责三段归类：
//  1. gorm 的 record not found → 404 + 该域专属码（界面上该刷新列表，不是服务端坏了）；
//  2. errtext 认得的哨兵（下载器连不上 / 超时 / SSRF…）→ 各自的可操作中文，带进响应体；
//  3. 其余 → 该域的兜底码 + 原因（WithDetail）。
//
// 兜底档刻意保留原因而不是清空：链层那句中文是用户唯一的下一步指引。
// respondSiteInspectErr 站点校准诊断的失败出口。
//
// #366 从前这里只有一句 `errcode.Respond(c, errcode.BadGateway.WithErr(err))`：
// 原因整段丢进日志，界面只显示「上游服务不可用，请稍后重试」。而 Inspect 是
// 站点管理里的**排障入口** —— 用它的人正是遇到问题的人，它的报错是整条链上
// 最该指路的那一条。四种最常见失败全都能当场指出下一步（文案单一来源在 errtext）：
//
//	ErrBlocked（内网/环回被 SSRF 拦）→ 改公网地址或设 ALLOW_PRIVATE_SITES
//	缺协议前缀                    → 补 http:// 或 https://（最常见的笔误）
//	连不上 / DNS 解析不了           → 核对地址、端口、本机 DNS
//	超时                          → 稍后重试（这一档「稍后重试」才是对的）
//
// 兜底档仍是 BadGateway，但改用 WithDetail 带上原因 —— 否则认不出来的上游故障
// 又回到本仓反复出现的「原因只进日志」。刻意不新造翻译表：errtext 已是单一来源。
func respondSiteInspectErr(c *gin.Context, err error) {
	// 注意这两档用 WithErr 而不是 WithDetail：WithDetail 无条件把 Message 换成
	// err.Error()（errcode.go:56），会把码自带的中文文案冲掉、退回 Go 原文
	// （实测 got "ssrf: blocked private/internal address: 10.0.0.5"）。
	// 这两档的文案已经写在码上、又与 errtext 同源，原文只留给日志定位即可。
	switch {
	case errors.Is(err, httpclient.ErrBlocked):
		errcode.Respond(c, errcode.SSRFBlocked.WithErr(err))
	case errors.Is(err, context.DeadlineExceeded):
		errcode.Respond(c, errcode.UpstreamTimeout.WithErr(err))
	default:
		reason := userErrText(err)
		// errtext 只翻本仓声明的哨兵；其余（缺协议前缀 / 连不上 / DNS）已在兜底档里
		// 变成可操作中文，而站点/下载器自己的原文原样返回 —— 此时直接采用，不改写。
		if reason == err.Error() {
			errcode.Respond(c, errcode.BadGateway.WithDetail(errors.New(reason)))
			return
		}
		errcode.Respond(c, errcode.SiteInspectUnreachable.WithDetail(errors.New(reason)))
	}
}

// respondDirCfgErr 整理配置写入口（目录标签 / 目录同步任务）的失败出口。
//
// #368 链层在这两个 Save 里逐条写好了「该改哪个」——「标签名重复: 电影」
// 「任务 t 同步方式非法: bogus」——而接口层一律 `InvalidInput.WithErr(err)`，
// 原因整段丢进日志，界面只剩「没能指出具体是哪一项」，用户只能一个个试。
//
// 这里用 InvalidInput 档 + WithDetail 带上链层原话：400 + 具体原因，两者都要。
// 库写失败（kv.Set 报错）同样走这里 —— 此时原话来自存储层，仍比通用话术可读。
// 刻意不新造翻译表：链层那句就是本仓自产的单一来源。
func respondDirCfgErr(c *gin.Context, err error) {
	errcode.Respond(c, errcode.InvalidInput.WithDetail(err))
}
func respondReasonErr(c *gin.Context, err error, notFound, fallback *errcode.Error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		errcode.Respond(c, notFound.WithDetail(err))
		return
	}
	// errtext 只翻译本仓声明的哨兵；站点/下载器自己的原文原样返回，此时直接采用，
	// 不改写（改写会抹掉用户唯一的线索）。
	if reason := userErrText(err); reason != err.Error() {
		errcode.Respond(c, fallback.WithDetail(errors.New(reason)))
		return
	}
	errcode.Respond(c, fallback.WithDetail(err))
}
