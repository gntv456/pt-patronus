// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

// TestRunLocalPowerCarriesStderrReason 本机电源动作的失败原因在 stderr 里
// （`shutdown.exe` 权限不足只回 exit status 5）。旧写法用 Run() 丢掉输出，
// 界面上就只剩「exit status 5」这种没头没尾的话。
func TestRunLocalPowerCarriesStderrReason(t *testing.T) {
	var name string
	var args []string
	if runtime.GOOS == "windows" {
		name = "cmd.exe"
		args = []string{"/c", "echo denied 1>&2 && exit 5"}
	} else {
		name = "sh"
		args = []string{"-c", "echo denied >&2; exit 5"}
	}

	err := runLocalPower(context.Background(), name, args)
	if err == nil {
		t.Fatal("命令应失败")
	}
	msg := err.Error()
	if !strings.Contains(msg, "denied") {
		t.Fatalf("原因必须带上 stderr（旧行为只回 exit status）, got %q", msg)
	}
	if !strings.Contains(msg, "exit status 5") {
		t.Errorf("退出码也要留着, got %q", msg)
	}

	// 正控：成功命令不该报错
	if err := runLocalPower(context.Background(), name, []string{"/c", "exit 0"}); runtime.GOOS == "windows" && err != nil {
		t.Errorf("exit 0 应无错, got %v", err)
	}
}
