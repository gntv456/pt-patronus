// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/downloader"
)

// TestKeepSeedConnectFailureIsNotParamError 辅种/查漏/处理候选在「下载器根本没开」时报的是
// 400「请求参数有误，请检查后重试」——用户回头改表单，实际该去「下载器」页看客户端在不在跑。
// 实测链路：qB 指向 127.0.0.1:9，POST /keepseed/reseed 只会在登录那一步失败。
func TestKeepSeedConnectFailureIsNotParamError(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondKeepSeedErr(c, fmt.Errorf(
		"%w: Post %q: dial tcp 127.0.0.1:9: connectex: actively refused",
		downloader.ErrCannotConnect, "http://127.0.0.1:9/api/v2/auth/login"))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502（响应体：%s）", rec.Code, rec.Body.String())
	}
	code, msg := errEnvelope(t, rec)
	if code != "downloader_conn_failed" {
		t.Errorf("code = %q, want downloader_conn_failed", code)
	}
	if !strings.Contains(msg, "连不上") || !strings.Contains(msg, "下载器") {
		t.Errorf("msg = %q, want 说清「下载器连不上」并给出下一步", msg)
	}
	if strings.Contains(msg, "参数有误") {
		t.Errorf("msg = %q, 连不上不是用户的参数问题", msg)
	}
	for _, leak := range []string{"dial tcp", "connectex", "cannot connect"} {
		if strings.Contains(msg, leak) {
			t.Errorf("msg 泄露 Go 拨号原文（%q）：%q", leak, msg)
		}
	}
}

// TestKeepSeedOtherErrorsKeepOldClass 划界：只有下载器连不上这一类改判，
// 其余错误仍按原来的 400 归类，且不外泄内部原文。
func TestKeepSeedOtherErrorsKeepOldClass(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondKeepSeedErr(c, errors.New("some internal chain failure"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400（响应体：%s）", rec.Code, rec.Body.String())
	}
	if _, msg := errEnvelope(t, rec); strings.Contains(msg, "some internal") {
		t.Errorf("msg = %q, 内部错误原文不得进界面", msg)
	}
}

// TestKeepSeedMissingPreconditionReachesUser 「未配置 IYUU token」「未指定目标站点」是链层
// 写给用户看的下一步，从前一律压成「请求参数有误，请检查后重试」，用户只能瞎猜缺了哪一项。
func TestKeepSeedMissingPreconditionReachesUser(t *testing.T) {
	for _, err := range []error{chain.ErrMissingNoToken, chain.ErrMissingNoTarget} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		respondKeepSeedErr(c, err)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400（err=%v）", rec.Code, err)
		}
		if _, msg := errEnvelope(t, rec); msg != err.Error() {
			t.Errorf("msg = %q, want 原样给出原因 %q", msg, err.Error())
		}
	}
}
