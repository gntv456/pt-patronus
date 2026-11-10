// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestDoSync_PushResponseHTMLIsNotSuccess 是 #216 的**出口级**红证：
// 云端「确实没数据」（GET 回空信封）允许以空 base 起步推送，但 PUT 之后
// 收到非 JSON 的 200（Cloudflare 拦截页/反代错误页）不能算「已推送 N 条」——
// 旧判据 json.Unmarshal(...)==nil 一失败就一路 return 成功，用户以为凭据已上云，
// 实际云端还是旧的，另一台设备下次拉取会把旧 cookie 推回本机。
//
// 判据函数（cookiecloud_push_response_test.go）测不到接线：撤掉这一行的调用它照样全绿。
func TestDoSync_PushResponseHTMLIsNotSuccess(t *testing.T) {
	var posts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			_, _ = w.Write([]byte("<html><title>Just a moment...</title></html>"))
			return
		}
		_, _ = w.Write([]byte(`{"encrypted":"","saved_at":0}`))
	}))
	t.Cleanup(srv.Close)

	ch := ccChainWithLocalCookie(t)
	_, err := ch.doSync(context.Background(), ccSettings(t, srv.URL, "u-html", "pass"))
	if err == nil {
		t.Fatal("推送响应是 HTML 却被报成同步成功（缺陷原形）")
	}
	if !strings.Contains(err.Error(), "未确认成功") || !strings.Contains(err.Error(), "Just a moment") {
		t.Errorf("要说清未能确认并带原文片段, got %q", err.Error())
	}
	if posts.Load() == 0 {
		t.Error("根本没走到 PUT 出口，红不在被验那一步")
	}
}
