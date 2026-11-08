package app

// 站点定义启动加载（从 app_setup.go 拆出，行数门禁 400）。
// 加载顺序（后加载者覆盖同 ID）：内置 embed → 远端缓存 → DEFINITIONS_DIR。
// 外部/远端定义带版本门控：min_app_version 高于本版的定义跳过并记升级诊断。

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	bundledDefinitions "github.com/hxpt/ptpatronus/definitions"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/scheduler"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/sitedefs"
	"github.com/hxpt/ptpatronus/internal/version"
)

func loadSiteDefinitions(
	registry *site.Registry, defsDir string, externalExplicit bool,
	appVersion, dataDir string,
) {
	if err := registry.LoadDefinitionsFS(bundledDefinitions.FS, "."); err != nil {
		slog.Warn("load bundled site definitions failed", "err", err)
	} else {
		slog.Info("loaded bundled site definitions", "count", len(registry.Definitions()))
	}

	// 远端定义源缓存（DATA_DIR/site-definitions）：上次同步的产物在启动期即生效，
	// 不必等下一个 cron 周期。带版本门控（min_app_version 高于本版的定义跳过）。
	if dataDir != "" {
		cacheDir := filepath.Join(dataDir, "site-definitions")
		err := registry.LoadDefinitionsFSVersioned(os.DirFS(cacheDir), ".", appVersion)
		if err == nil {
			slog.Info("loaded remote-cached site definitions",
				"dir", cacheDir, "total", len(registry.Definitions()))
		}
	}

	if strings.TrimSpace(defsDir) == "" {
		return
	}
	if err := registry.LoadDefinitionsFSVersioned(
		os.DirFS(defsDir), ".", appVersion); err != nil {
		if !externalExplicit && errors.Is(err, os.ErrNotExist) {
			slog.Info("external site definitions directory not found; using bundled definitions", "dir", defsDir, "count", len(registry.Definitions()))
			return
		}
		slog.Warn("load external site definitions failed; using bundled definitions", "dir", defsDir, "err", err)
		return
	}
	slog.Info("loaded external site definitions", "dir", defsDir, "total", len(registry.Definitions()))
}

// wireSiteSync 构造站点定义远端源 Syncer 并挂 cron（每 6h）+ 包级 Default
// （手动同步 API 经 sitedefs.Default() 取）。站点适配不绑发版的核心接线。
func wireSiteSync(sched *scheduler.Scheduler, cfgOp *oper.SystemConfigOper,
	hc *httpclient.Client, registry *site.Registry, dataDir string) {
	sitedefs.Wire(sched, cfgOp, hc, registry, dataDir, version.Version)
}
