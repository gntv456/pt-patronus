// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/plugins"
)

func TestPluginHostToolCatalogMetadata(t *testing.T) {
	reg := plugins.NewHostToolRegistry()
	registerCloudTools(reg, nil)
	registerBrushFlowTools(reg, nil)
	registerRssTools(reg, nil)
	registerFileTools(reg, nil)

	expected := map[string]struct {
		permission  string
		destructive bool
	}{
		"cloud_create": {"cloud:write", false}, "cloud_update": {"cloud:write", false},
		"cloud_delete": {"cloud:write", true}, "cloud_test": {"cloud:read", false},
		"cloud_rule_create": {"cloud:write", false}, "cloud_rule_update": {"cloud:write", false},
		"cloud_rule_delete": {"cloud:write", true}, "cloud_records": {"cloud:read", false},
		"cloud_record_delete":   {"cloud:write", true},
		"brushflow_task_create": {"brushflow:write", false}, "brushflow_task_update": {"brushflow:write", false},
		"brushflow_task_delete": {"brushflow:write", true},
		"rss_get":               {"rss:read", false}, "rss_create": {"rss:write", false},
		"rss_update": {"rss:write", false}, "rss_delete": {"rss:write", true},
		"file_rename": {"file:write", false}, "file_move": {"file:write", false},
		"file_copy": {"file:write", false}, "file_test": {"file:read", false},
	}

	for name, want := range expected {
		tool, ok := reg.Lookup(name)
		if !ok {
			t.Errorf("tool %q is not registered", name)
			continue
		}
		if tool.Permission != want.permission {
			t.Errorf("tool %q permission = %q, want %q", name, tool.Permission, want.permission)
		}
		if tool.Destructive != want.destructive {
			t.Errorf("tool %q destructive = %v, want %v", name, tool.Destructive, want.destructive)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has no input schema", name)
		}
	}
}
