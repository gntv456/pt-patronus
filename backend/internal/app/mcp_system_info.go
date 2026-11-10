// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/version"
)

// systemInfoPayload 组装 MCP 工具 `system.info` 的账面。两处注册面（进程内 AI 助手的
// agent 工具、对外 MCP server）共用这一份，避免同一个判据抄两遍后各自漂移。
//
// 数量读不出来时写 null + `<key>_error`，而不是 0（#259）：从前这里是
// `sites, _ := siteOp.List()`，一次查询失败被讲成 "sites": 0，助手照着说
// 「你还没配置站点」——用户被告知去配站点，实际坏的是数据库读取。
func systemInfoPayload(siteOp *oper.SiteOper, subCh *chain.SubscribeChain, dlOp *oper.DownloaderOper) map[string]any {
	sites, sitesErr := siteOp.List()
	subs, subsErr := subCh.List()
	dls, dlsErr := dlOp.List()
	out := map[string]any{"app": "PTPatronus", "version": version.Version}
	putMCPCount(out, "sites", len(sites), sitesErr)
	putMCPCount(out, "subscriptions", len(subs), subsErr)
	putMCPCount(out, "downloaders", len(dls), dlsErr)
	return out
}

// putMCPCount 写一个数量类账面：读失败留 null 与原因，成功才留数字。
func putMCPCount(out map[string]any, key string, n int, err error) {
	if err != nil {
		out[key] = nil
		out[key+"_error"] = errtext.Text(err)
		return
	}
	out[key] = n
}
