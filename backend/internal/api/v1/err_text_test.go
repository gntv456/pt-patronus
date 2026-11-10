// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"gorm.io/gorm"
)

// TestUserErrText 软失败面（200 + {ok:false,error}）的文案口径：
// 底层技术错误要翻成能照着做的话，而站点/下载器自己给的原文不许被改写。
func TestUserErrText(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"空错误", nil, ""},
		{"裸 record not found", gorm.ErrRecordNotFound, "目标不存在"},
		{"包装过的 record not found", fmt.Errorf("oper: %w", gorm.ErrRecordNotFound), "目标不存在"},
		{"SSRF 拦截", fmt.Errorf("fetch: %w", httpclient.ErrBlocked), errcode.SSRFBlocked.Message},
		{"超时", context.DeadlineExceeded, "请求超时"},
		{"站点原文照传", errors.New("该站点登录态已失效"), "该站点登录态已失效"},
		{"下载器原文照传", errors.New("qBittorrent: 端口被占用"), "qBittorrent: 端口被占用"},
	}
	for _, tc := range cases {
		got := userErrText(tc.err)
		if !strings.Contains(got, tc.want) {
			t.Fatalf("%s: got %q，应含 %q", tc.name, got, tc.want)
		}
	}
	if got := userErrText(httpclient.ErrBlocked); !strings.Contains(got, "ALLOW_PRIVATE_SITES") {
		t.Fatalf("SSRF 软失败也要给可操作的开关提示，got %q", got)
	}
	// 不许把未知错误洗成通用话术：站点原文是用户唯一的线索。
	if got := userErrText(errors.New("cookie 已过期")); got != "cookie 已过期" {
		t.Fatalf("未知错误应原样透传，got %q", got)
	}
}

// TestOpResultSoftFailureIsChinese opResult 是全仓 48 处写操作软失败的唯一出口，
// 前端把它的 error 直接弹成 toast；修复前这里是 "record not found"。
func TestOpResultSoftFailureIsChinese(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	opResult(c, gorm.ErrRecordNotFound)
	if w.Code != http.StatusOK {
		t.Fatalf("软失败仍应是 200，got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"ok":false`) {
		t.Fatalf("应保留 ok:false 形状，got %s", body)
	}
	if !strings.Contains(body, "目标不存在") {
		t.Fatalf("error 应是中文可操作提示，got %s", body)
	}
	if strings.Contains(body, "record not found") {
		t.Fatalf("不得把 gorm 原文弹给用户，got %s", body)
	}
}
