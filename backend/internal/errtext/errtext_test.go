// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package errtext

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"gorm.io/gorm"
)

// TestText 逐条结果面（一键签到/站点连通性/求种/搜索…每站一行的 error 列）与
// HTTP 软失败面共用这一份翻译：底层技术串不许原样进用户界面，
// 而站点/下载器自己给的中文原文一个字都不许改写（那是用户唯一的线索）。
func TestText(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"裸 record not found", gorm.ErrRecordNotFound, NotFound},
		{"包装过的 record not found", fmt.Errorf("oper: %w", gorm.ErrRecordNotFound), NotFound},
		{"链式两层包装", fmt.Errorf("run: %w", fmt.Errorf("store: %w", gorm.ErrRecordNotFound)), NotFound},
		{"SSRF 拦截", fmt.Errorf("fetch: %w", httpclient.ErrBlocked), SSRFHint},
		{"超时", context.DeadlineExceeded, Timeout},
		{"站点原文照传", errors.New("该站点登录态已失效，请更新 Cookie"), "该站点登录态已失效，请更新 Cookie"},
		{"下载器原文照传", errors.New("qBittorrent: 端口被占用"), "qBittorrent: 端口被占用"},
	}
	for _, tc := range cases {
		if got := Text(tc.err); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	if !strings.Contains(SSRFHint, "ALLOW_PRIVATE_SITES") {
		t.Fatal("SSRF 文案必须给出可操作的开关名")
	}
}
