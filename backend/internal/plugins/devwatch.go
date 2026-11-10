// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"github.com/hxpt/ptpatronus/internal/core/safego"
	"hash/fnv"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultDevWatchInterval = time.Second

type devWatchSnapshot struct {
	Fingerprint uint64
}

func (m *Manager) EnableDevWatcher(interval time.Duration) {
	root := strings.TrimSpace(m.PluginRoot())
	if root == "" {
		return
	}
	if interval <= 0 {
		interval = defaultDevWatchInterval
	}
	m.stopDevWatcher()

	ctx, cancel := context.WithCancel(context.Background())
	m.watchMu.Lock()
	m.watchCancel = cancel
	m.watchWG.Add(1)
	m.watchMu.Unlock()

	safego.Go("devwatch:39", func() {
		defer m.watchWG.Done()
		m.devWatchLoop(ctx, root, interval)
	})

	slog.Info("plugin dev watcher started", "root", root, "interval", interval.String())
}

func (m *Manager) stopDevWatcher() {
	m.watchMu.Lock()
	cancel := m.watchCancel
	m.watchCancel = nil
	m.watchMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	m.watchWG.Wait()
	slog.Info("plugin dev watcher stopped")
}

func (m *Manager) devWatchLoop(ctx context.Context, root string, interval time.Duration) {
	// 基线在调用方 goroutine 里同步取（EnableDevWatcher 时刻视为「无插件」）：
	// 若在 watcher goroutine 里首扫，慢调度下扫描可能晚于用户刚建的目录，
	// 该目录进基线后永远不触发「新增」分支（Windows CI 慢机必现）。
	// 第一个 tick 发现的目录都会走 LoadExternalDir，幂等（同 ID replace 重载）。
	previous := map[string]devWatchSnapshot{}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := scanDevWatchRoot(root)
			m.applyDevWatchChanges(previous, current)
			previous = current
		}
	}
}

func (m *Manager) applyDevWatchChanges(previous, current map[string]devWatchSnapshot) {
	for _, dir := range sortedSnapshotKeys(current) {
		currentSnapshot := current[dir]
		previousSnapshot, existed := previous[dir]
		switch {
		case !existed:
			if view, err := m.LoadExternalDir(dir); err != nil {
				slog.Warn("plugin dev watcher failed to load external plugin", "dir", dir, "err", err)
			} else {
				m.log(view.ID, logInfo, "dev.watch", "plugin auto-loaded after directory creation", map[string]any{"dir": dir})
				slog.Info("plugin dev watcher loaded external plugin", "plugin", view.ID, "dir", dir)
			}
		case previousSnapshot.Fingerprint != currentSnapshot.Fingerprint:
			if view, err := m.LoadExternalDir(dir); err != nil {
				slog.Warn("plugin dev watcher failed to reload external plugin", "dir", dir, "err", err)
			} else {
				m.log(view.ID, logInfo, "dev.watch", "plugin auto-reloaded after source change", map[string]any{"dir": dir})
				slog.Info("plugin dev watcher reloaded external plugin", "plugin", view.ID, "dir", dir)
			}
		}
	}

	for _, dir := range sortedSnapshotKeys(previous) {
		if _, ok := current[dir]; ok {
			continue
		}
		if pluginID, ok := m.unloadExternalByDir(dir); ok {
			slog.Info("plugin dev watcher unloaded external plugin", "plugin", pluginID, "dir", dir)
		}
	}
}

func scanDevWatchRoot(root string) map[string]devWatchSnapshot {
	out := map[string]devWatchSnapshot{}
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("plugin dev watcher failed to read plugin root", "root", root, "err", err)
		}
		return out
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if shouldSkipDevWatchName(name) {
			continue
		}
		dir := filepath.Join(root, name)
		fingerprint, err := fingerprintPluginDir(dir)
		if err != nil {
			slog.Warn("plugin dev watcher failed to fingerprint plugin directory", "dir", dir, "err", err)
			continue
		}
		out[dir] = devWatchSnapshot{Fingerprint: fingerprint}
	}
	return out
}

func shouldSkipDevWatchName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return strings.Contains(name, ".bak-")
}

func shouldSkipDevWatchPath(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, part := range parts {
		switch part {
		case "", ".", "__pycache__", ".git", ".venv":
			if part != "." {
				return true
			}
		case "node_modules":
			return true
		}
		if strings.HasPrefix(part, ".install-") {
			return true
		}
	}
	return false
}

func fingerprintPluginDir(dir string) (uint64, error) {
	hasher := fnv.New64a()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if shouldSkipDevWatchPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		_, _ = hasher.Write([]byte(rel))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(info.Mode().String()))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(strconvFormatInt(info.ModTime().UnixNano())))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(strconvFormatInt(info.Size())))
		_, _ = hasher.Write([]byte{0})
		return nil
	})
	if err != nil {
		return 0, err
	}
	return hasher.Sum64(), nil
}

func sortedSnapshotKeys(items map[string]devWatchSnapshot) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (m *Manager) unloadExternalByDir(dir string) (string, bool) {
	pluginID, plugin := m.externalPluginByDir(dir)
	if pluginID == "" || plugin == nil {
		return "", false
	}

	stopPlugin(plugin)

	m.mu.Lock()
	delete(m.plugins, pluginID)
	m.mu.Unlock()

	m.removeSchedules(pluginID)
	if err := m.markPluginUninstalled(pluginID); err != nil {
		// 目录监听这条没有 HTTP 出口可报错，但必须留在插件日志里：
		// 目录已删而状态行没落成未安装，面板仍会显示它在安装中。
		m.log(pluginID, logError, "dev.watch", "插件源码目录已移除，但安装状态没能写入数据库："+err.Error(), nil)
	}
	m.log(pluginID, logInfo, "dev.watch", "plugin unloaded after source directory removal", map[string]any{"dir": dir})
	return pluginID, true
}

func (m *Manager) externalPluginByDir(dir string) (string, Plugin) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, plugin := range m.plugins {
		external, ok := plugin.(*externalHTTPPlugin)
		if !ok {
			continue
		}
		if sameFilePath(external.dir, dir) {
			return id, plugin
		}
	}
	return "", nil
}

func sameFilePath(a, b string) bool {
	aAbs, aErr := filepath.Abs(a)
	bAbs, bErr := filepath.Abs(b)
	if aErr != nil || bErr != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(aAbs), filepath.Clean(bAbs))
	}
	return filepath.Clean(aAbs) == filepath.Clean(bAbs)
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
