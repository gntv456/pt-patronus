// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"testing"
)

func TestSampleEvenly(t *testing.T) {
	files := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	got := sampleEvenly(files, 3)
	if len(got) != 3 {
		t.Fatalf("want 3 samples, got %d (%v)", len(got), got)
	}
	if got[0] != "a" || got[1] != "d" || got[2] != "g" {
		t.Errorf("evenly sampled wrong: %v", got)
	}
	if got := sampleEvenly([]string{"a", "b"}, 3); len(got) != 2 {
		t.Errorf("n<=k should return all, got %v", got)
	}
}

// stubRT 仅满足 isShortDrama 的 Runtime 入参（只用 Log）。
type stubRT struct{}

func (stubRT) Config() map[string]any                          { return nil }
func (stubRT) Log(string, string, string, map[string]any)      {}
func (stubRT) Notice(context.Context, NoticeInput) error       { return nil }
func (stubRT) Publish(context.Context, string, map[string]any) {}
func (stubRT) KVGet(string, any) error                         { return nil }
func (stubRT) KVSet(string, any) error                         { return nil }
func (stubRT) KVDelete(string) error                           { return nil }
func (stubRT) SiteList() []SiteRef                             { return nil }
func (stubRT) SiteCookie(uint) (string, error)                 { return "", nil }
func (stubRT) SetRuntimeConfig(string, any) error              { return nil }

// 空目录（无视频）应返回 nil（非视频目录，不判为短剧也不判为非短剧）。
func TestIsShortDrama_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	got := isShortDrama(context.Background(), dir, parseVideoExts(""), 3, 480, stubRT{})
	if got != nil {
		t.Errorf("empty dir should return nil verdict, got %v", got)
	}
}
