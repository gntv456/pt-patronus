// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package subauto

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/subtitle"
)

// fakeSearcher 内存字幕源（无网络）：固定返回预设结果，记录调用次数。
type fakeSearcher struct {
	mu         sync.Mutex
	subs       []subtitle.Subtitle
	data       []byte
	name       string
	searchHits int
	dlHits     int
	searchErr  error
}

func (f *fakeSearcher) Search(context.Context, string, string) ([]subtitle.Subtitle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchHits++
	return f.subs, f.searchErr
}

func (f *fakeSearcher) Download(context.Context, subtitle.Subtitle) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dlHits++
	return f.data, f.name, nil
}

func (f *fakeSearcher) stats() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.searchHits, f.dlHits
}

// memKV 内存 KVStore（替 oper.SystemConfigOper，无需 DB）。
type memKV struct{ m map[string]string }

func (k *memKV) Get(key string) (string, error) { return k.m[key], nil }
func (k *memKV) Set(key, v string) error        { k.m[key] = v; return nil }

func writeVideo(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestAutomationDownloadsSubtitle 配置开启 + 搜索命中 → 字幕落盘到同目录同名。
func TestAutomationDownloadsSubtitle(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "Movie (2020) - 1080p.mkv")
	fs := &fakeSearcher{
		subs: []subtitle.Subtitle{{Source: "subhd", SourceID: "1", LangCode: "chs", Ext: ".srt"}},
		data: []byte("1\n00:00:01 --> 00:00:02\n测试字幕\n"),
		name: "sub.srt",
	}
	kv := &memKV{m: map[string]string{}}
	if err := Save(kv, Config{Enabled: true, Languages: []string{"zh"}}); err != nil { // zh→chs 归一
		t.Fatal(err)
	}
	s := New(nil, fs, kv, nil)
	s.process(MediaInfo{Title: "Movie", Year: 2020, Type: "movie", Files: []string{video}})

	target := filepath.Join(dir, "Movie (2020) - 1080p.chs.srt")
	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("字幕未落盘: %v", err)
	}
	if !bytes.Contains(b, []byte("测试字幕")) {
		t.Errorf("字幕内容异常: %q", b)
	}
	sh, dh := fs.stats()
	if sh != 1 || dh != 1 {
		t.Errorf("调用次数 search=%d download=%d, want 1/1", sh, dh)
	}
}

// TestAutomationSkipsExisting 幂等：视频目录已有同 basename 字幕 → OnlyIfMissing 下跳过，不覆盖。
func TestAutomationSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "Movie.mkv")
	exist := filepath.Join(dir, "Movie.srt")
	if err := os.WriteFile(exist, []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &fakeSearcher{
		subs: []subtitle.Subtitle{{Source: "subhd", SourceID: "1", LangCode: "chs"}},
		data: []byte("NEW"),
		name: "x.srt",
	}
	kv := &memKV{m: map[string]string{}}
	_ = Save(kv, Config{Enabled: true, Languages: []string{"chs"}, OnlyIfMissing: true})
	New(nil, fs, kv, nil).process(MediaInfo{Title: "Movie", Type: "movie", Files: []string{video}})

	if _, dh := fs.stats(); dh != 0 {
		t.Errorf("已存在字幕不应下载, got %d calls", dh)
	}
	b, _ := os.ReadFile(exist)
	if string(b) != "OLD" {
		t.Errorf("已有字幕被覆盖: %q", b)
	}
}

// TestAutomationDisabledNoOp 配置关闭时不动作（不搜索、不下载、不落盘）。
func TestAutomationDisabledNoOp(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "Movie.mkv")
	fs := &fakeSearcher{
		subs: []subtitle.Subtitle{{Source: "subhd", SourceID: "1", LangCode: "chs"}},
		data: []byte("X"),
		name: "x.srt",
	}
	kv := &memKV{m: map[string]string{}} // 默认 Enabled=false
	New(nil, fs, kv, nil).process(MediaInfo{Title: "Movie", Files: []string{video}})

	if sh, dh := fs.stats(); sh != 0 || dh != 0 {
		t.Errorf("关闭状态下不应调用, search=%d download=%d", sh, dh)
	}
	if _, err := os.Stat(filepath.Join(dir, "Movie.chs.srt")); err == nil {
		t.Error("关闭状态下不应落盘字幕")
	}
}

// TestAutomationEventWiring 端到端：事件总线 Publish transfer_done → 后台 goroutine 落盘字幕。
func TestAutomationEventWiring(t *testing.T) {
	bus := event.New()
	dir := t.TempDir()
	video := writeVideo(t, dir, "Show S01E02.mkv")
	fs := &fakeSearcher{
		subs: []subtitle.Subtitle{{Source: "assrt", SourceID: "9", LangCode: "chs", Ext: ".srt"}},
		data: []byte("event-driven"),
		name: "a.srt",
	}
	kv := &memKV{m: map[string]string{}}
	_ = Save(kv, Config{Enabled: true, Languages: []string{"chs"}})
	s := New(bus, fs, kv, nil)
	s.Start()

	bus.Publish(context.Background(), EventTransferDone, map[string]any{
		"title": "Show", "type": "tv", "files": []string{video},
	})

	target := filepath.Join(dir, "Show S01E02.chs.srt")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(target); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("事件未触发字幕下载: %v", err)
	}
	if !bytes.Contains(b, []byte("event-driven")) {
		t.Errorf("字幕内容异常: %q", b)
	}
}

// TestAutomationOnlyIfMissingFalseFillsLanguage OnlyIfMissing=false 时：不同语言已存在仍补齐缺失语言。
func TestAutomationOnlyIfMissingFalseFillsLanguage(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "Movie.mkv")
	// 已有英文字幕，但缺中文
	if err := os.WriteFile(filepath.Join(dir, "Movie.eng.srt"), []byte("EN"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &fakeSearcher{
		subs: []subtitle.Subtitle{{Source: "assrt", SourceID: "1", LangCode: "chs", Ext: ".srt"}},
		data: []byte("ZH"),
		name: "a.srt",
	}
	kv := &memKV{m: map[string]string{}}
	_ = Save(kv, Config{Enabled: true, Languages: []string{"chs"}, OnlyIfMissing: false})
	New(nil, fs, kv, nil).process(MediaInfo{Title: "Movie", Type: "movie", Files: []string{video}})

	target := filepath.Join(dir, "Movie.chs.srt")
	if _, err := os.Stat(target); err != nil {
		t.Errorf("应补齐中文字幕但未落盘: %v", err)
	}
}
