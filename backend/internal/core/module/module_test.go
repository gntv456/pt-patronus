// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package module

import (
	"context"
	"reflect"
	"testing"
)

type mockMod struct {
	name string
	prio int
}

func (m mockMod) Name() string  { return m.name }
func (m mockMod) Priority() int { return m.prio }

func TestListSortedByPriority(t *testing.T) {
	m := New()
	m.Register(mockMod{"low", 90}, mockMod{"high", 1}, mockMod{"mid", 50})
	got := m.List()
	want := []string{"high", "mid", "low"}
	for i, w := range want {
		if got[i].Name() != w {
			t.Errorf("pos %d = %s, want %s", i, got[i].Name(), w)
		}
	}
}

func TestRunCollectAndStopSemantics(t *testing.T) {
	m := New()
	m.Register(
		mockMod{"a", 1},
		mockMod{"b", 2},
		mockMod{"c", 3},
	)

	// Continue = 扇出收集全部
	got := Run(context.Background(), m, func(mod Module) (string, Control) {
		return mod.Name(), Continue
	})
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("collect = %v", got)
	}

	// Stop = 命中即停
	got = Run(context.Background(), m, func(mod Module) (string, Control) {
		if mod.Name() == "b" {
			return mod.Name(), Stop
		}
		return "", Skip
	})
	if !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("stop = %v", got)
	}

	// Skip 全跳过 → 空
	got = Run(context.Background(), m, func(mod Module) (string, Control) {
		return "", Skip
	})
	if len(got) != 0 {
		t.Errorf("skip-all = %v", got)
	}
}

func TestRunRespectsContextCancel(t *testing.T) {
	m := New()
	m.Register(mockMod{"a", 1}, mockMod{"b", 2})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 提前取消
	got := Run(ctx, m, func(mod Module) (string, Control) {
		return mod.Name(), Continue
	})
	if len(got) != 0 {
		t.Errorf("cancelled run should yield empty, got %v", got)
	}
}
