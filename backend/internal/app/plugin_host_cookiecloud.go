// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"context"
	"time"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// registerCookieCloudTools 把 CookieCloud 同步能力暴露给插件（#34 二梯队：进程内
// MaybeSync cron 迁 ptp-cookiecloud-sync 插件；解密/站点匹配语义留在宿主链）。
func registerCookieCloudTools(
	reg *plugins.HostToolRegistry, cc *chain.CookieCloudChain,
) {
	if cc == nil {
		return
	}
	reg.Register(plugins.HostTool{
		Name: "site_sync_cookies", Category: "site", Permission: "site:action",
		Description: "Run a CookieCloud sync round: pull encrypted payload, " +
			"decrypt, update stale site cookies (+auto-add matched built-in " +
			"definitions); optionally push local cookies back.",
		InputSchema: objSchema(map[string]any{
			"push": boolProp("also push local cookies to the cloud (default false)"),
		}, nil),
		Timeout: 120 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		report, err := cc.Sync(ctx)
		if err != nil {
			return nil, mcpErr(err)
		}
		if argBool(a, "push") {
			pushed, perr := cc.Push(ctx)
			if perr != nil {
				report.Errors = append(report.Errors, "push: "+perr.Error())
			} else {
				report.Pushed = pushed.Pushed
			}
		}
		return report, nil
	})
	reg.Register(plugins.HostTool{
		Name: "site_sync_settings", Category: "site", Permission: "site:read",
		Description: "Read CookieCloud sync settings " +
			"(server url/mode/interval; password presence only).",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return cc.GetSettings(), nil
	})
}
