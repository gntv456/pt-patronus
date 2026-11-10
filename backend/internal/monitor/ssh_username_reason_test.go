// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// 探活结果里的 error 会直接展示给用户（200+{ok:false,error}）。
// 缺用户名是用户可自行处理的输入问题，必须是能照着做的中文，不能是英文原文。
func TestDialSSHMissingUsernameIsChinese(t *testing.T) {
	_, err := dialSSH(nil, &models.MonitorTarget{Type: "host", Host: "127.0.0.1"}, Credentials{}, time.Second)
	if err == nil {
		t.Fatal("缺用户名应报错")
	}
	if !strings.Contains(err.Error(), "SSH 用户名") {
		t.Fatalf("原因是写给用户看的中文, got %q", err.Error())
	}
}
