// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestParseCmdItems(t *testing.T) {
	raw := "# 注释\n清理|rm -rf /tmp/x|30\n备份|tar czf /b.tgz /data\n空名||10\n  \nbad"
	items := parseCmdItems(raw)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d (%+v)", len(items), items)
	}
	if items[0].Name != "清理" || items[0].Command != "rm -rf /tmp/x" || items[0].Timeout != 30 {
		t.Fatalf("bad item0: %+v", items[0])
	}
	if items[1].Name != "备份" || items[1].Timeout != 0 {
		t.Fatalf("bad item1: %+v", items[1])
	}
}

func TestAtoiOr(t *testing.T) {
	if atoiOr("42", 1) != 42 {
		t.Fatal("atoiOr digits")
	}
	if atoiOr("x", 7) != 7 {
		t.Fatal("atoiOr fallback")
	}
	if atoiOr("", 9) != 9 {
		t.Fatal("atoiOr empty fallback")
	}
}

// fakeRuntime 收集 Notice 调用，便于断言推送策略。
type ccFakeRT struct {
	cfg     map[string]any
	notices []string
}

func (f *ccFakeRT) Config() map[string]any                         { return f.cfg }
func (f *ccFakeRT) Log(level, ev, msg string, data map[string]any) {}
func (f *ccFakeRT) Notice(ctx context.Context, in NoticeInput) error {
	f.notices = append(f.notices, in.Title+":"+in.Level)
	return nil
}
func (f *ccFakeRT) Publish(context.Context, string, map[string]any) {}
func (f *ccFakeRT) KVGet(string, any) error                         { return nil }
func (f *ccFakeRT) KVSet(string, any) error                         { return nil }
func (f *ccFakeRT) KVDelete(string) error                           { return nil }
func (f *ccFakeRT) SiteList() []SiteRef                             { return nil }
func (f *ccFakeRT) SiteCookie(uint) (string, error)                 { return "", nil }
func (f *ccFakeRT) SetRuntimeConfig(string, any) error              { return nil }

// 注意：custom_command.runOne 期望的是 *plugins Runtime 接口（具体类型在 plugins 包）。
// runOne 接收 Runtime（plugins 包内接口），ccFakeRT 实现之。
func TestCustomCommandRunOneSuccessAndFail(t *testing.T) {
	p := &customCommandPlugin{}
	rt := &ccFakeRT{cfg: map[string]any{"timeout": 30}}

	ok := p.runOne(context.Background(), rt, cmdItem{Name: "echo", Command: "echo hello"})
	if !ok.Succeeded || ok.ExitCode != 0 || !strings.Contains(ok.Output, "hello") {
		t.Fatalf("success case bad: %+v", ok)
	}
	failCommand := "exit 3"
	if runtime.GOOS == "windows" {
		failCommand = "exit /b 3"
	}
	fail := p.runOne(context.Background(), rt, cmdItem{Name: "bad", Command: failCommand})
	if fail.Succeeded || fail.ExitCode != 3 {
		t.Fatalf("fail case bad: %+v", fail)
	}
}

func TestCustomCommandRunOneTimeout(t *testing.T) {
	p := &customCommandPlugin{}
	rt := &ccFakeRT{cfg: map[string]any{"timeout": 1}}
	sleepCommand := "sleep 10"
	if runtime.GOOS == "windows" {
		sleepCommand = "ping -n 6 127.0.0.1 >NUL"
	}
	r := p.runOne(context.Background(), rt, cmdItem{Name: "sleep", Command: sleepCommand, Timeout: 1})
	if !r.TimedOut {
		t.Fatalf("expect timeout, got %+v", r)
	}
}
