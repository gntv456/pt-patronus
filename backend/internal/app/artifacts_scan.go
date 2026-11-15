// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

// 产物托管自动挂载（更新方式改造）：<DataDir>/downloads/ 里的规范产物
// （<platform>-<version>.<ext>）自动算 sha256 挂进自托管清单——CI scp 后
// 最迟 1 小时生效，无需人工回填 url。机制见 internal/update/artifacts.go。

import (
	"log/slog"
	"path/filepath"
	"time"

	"github.com/hxpt/ptpatronus/internal/update"
)

// startArtifactMountLoop 启动即扫一次 + 每小时兜底重扫。
// 失败只记日志——清单挂载失败不影响其它功能，下轮重试。
func startArtifactMountLoop(updateSvc *update.Service, dataDir string) {
	go func() {
		dl := filepath.Join(dataDir, "downloads")
		for {
			if err := updateSvc.ApplyArtifacts(dl); err != nil {
				slog.Warn("update artifacts scan failed", "dir", dl, "err", err)
			}
			time.Sleep(time.Hour)
		}
	}()
}
