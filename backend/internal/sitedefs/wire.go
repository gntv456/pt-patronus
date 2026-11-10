// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package sitedefs

// Wire 把远端定义源同步接进宿主进程：构造 Syncer + 注册 cron（每 6h），
// 并把实例挂到包级 Default（手动同步 API 读）。返回 Syncer 供 Deps 注入路由。
// 依赖用窄接口声明（scheduler/SystemConfigOper 形状），包依赖保持单向：
// app → sitedefs，杜绝 app→api→v1 与 sitedefs 的环。
// enabled=false 时 cron 空转；remote_url 每次现读设置（热改源不重启）。

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// CronRegister 是 scheduler.AddFunc 的形状（窄接口，app 侧实现天然满足）。
type CronRegister interface {
	AddFunc(name, spec string, fn func()) error
}

// URLGetter 是 SystemConfigOper.Get 的形状。
type URLGetter interface {
	Get(key string) (string, error)
}

var (
	wireMu      sync.Mutex
	wiredSyncer *Syncer
)

// Wire 构造 Syncer 并注册 cron。重复调用（测试）会覆盖 Default。
func Wire(sched CronRegister, cfgOp URLGetter, hc *httpclient.Client,
	registry *site.Registry, dataDir, appVersion string,
) *Syncer {
	// 外部手动目录（DEFINITIONS_DIR）叠加进热加载；未设则仅远端缓存。
	var extraDirs []string
	if env := strings.TrimSpace(os.Getenv("DEFINITIONS_DIR")); env != "" {
		extraDirs = []string{env}
	}
	s := NewSyncer(hc, registry, dataDir, appVersion, extraDirs, func() string {
		v, _ := cfgOp.Get("sitedefs.remote_url")
		return v
	})
	wireMu.Lock()
	wiredSyncer = s
	wireMu.Unlock()

	if err := sched.AddFunc("sitedefs-sync", "0 */6 * * *", func() {
		if en, _ := cfgOp.Get("sitedefs.enabled"); en == "false" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if _, err := s.Sync(ctx); err != nil {
			slog.Warn("site definitions remote sync failed; keep last cache", "err", err)
		}
	}); err != nil {
		slog.Error("cron register FAILED — job will NOT run",
			"name", "sitedefs-sync", "err", err)
	}
	return s
}

// Default 返回 Wire 挂载的实例（手动同步 API；nil=未装配/safe-mode）。
func Default() *Syncer {
	wireMu.Lock()
	defer wireMu.Unlock()
	return wiredSyncer
}
