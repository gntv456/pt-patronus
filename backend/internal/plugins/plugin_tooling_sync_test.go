// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginDeveloperToolingCopiesStayInSync(t *testing.T) {
	root := ptpRoot(t)
	pairs := [][2]string{
		{"backend/tools/pluginctl.py", "sdk/python/src/ptpatronus_plugin_sdk/pluginctl.py"},
		{"backend/tools/plugin-manifest.schema.json", "sdk/python/src/ptpatronus_plugin_sdk/plugin-manifest.schema.json"},
	}
	for _, pair := range pairs {
		left, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pair[0])))
		if err != nil {
			t.Fatal(err)
		}
		right, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pair[1])))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(left, right) {
			t.Fatalf("plugin developer tooling copies differ: %s and %s", pair[0], pair[1])
		}
	}
}
