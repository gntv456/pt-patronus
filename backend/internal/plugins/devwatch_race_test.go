// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"time"
)

// TestDevWatcherLoadsDirCreatedBeforeFirstScan 回归：watcher 基线必须在
// EnableDevWatcher 调用方 goroutine 同步取空——若在 watcher goroutine 里首扫，
// 慢调度下扫描晚于用户刚建的目录，该目录进基线后永远不触发「新增」加载
// （Windows CI 慢机必现，v1.03.28 发版夜定位）。
func TestDevWatcherLoadsDirCreatedBeforeFirstScan(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "plugins")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(pluginRoot)
	if err := m.LoadExternal(pluginRoot); err != nil {
		t.Fatal(err)
	}
	// 目录在 EnableDevWatcher 之前就存在（等价于首扫被调度延迟到建目录之后）
	dir := filepath.Join(pluginRoot, "watch-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"watch-demo","name":"Watch Demo","version":"1.0.0","runtime":"external-http","entry":{"base_url":"http://127.0.0.1:19090","health":"/health"}}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.py"), []byte("print(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.EnableDevWatcher(100 * time.Millisecond)
	defer m.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := m.Get("watch-demo"); err == nil {
			t.Log("loaded (no race)")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("RACE CONFIRMED: dir present before first scan is never loaded")
}
