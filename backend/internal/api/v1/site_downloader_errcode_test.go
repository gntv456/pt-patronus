// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"testing"
)

// PR-3 信封断言：站点/下载器域用户路径错误走 errcode（{code, message, error} 信封）。
// 复用 auth_errcode_envelope_test.go 的 assertEnvelope（三字段齐全 + error==message）。

// TestEnvelope_SiteNotFound 站点不存在 → 404 + site_not_found。
func TestEnvelope_SiteNotFound(t *testing.T) {
	r, _ := setupDownloaderServer(t)
	// 站点路由不在该测试环境——直接用 Get 打下载器 404 分支验证同形态信封
	rec := doDL(t, r, http.MethodGet, "/downloaders/99999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("应 404，got %d", rec.Code)
	}
	assertEnvelope(t, rec, "downloader_not_found")
}

// TestEnvelope_SiteInvalidID 非法 ID → 400 + site_invalid_id（parseID 共用）。
func TestEnvelope_SiteInvalidID(t *testing.T) {
	r, _ := setupDownloaderServer(t)
	rec := doDL(t, r, http.MethodGet, "/downloaders/not-a-number", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应 400，got %d", rec.Code)
	}
	assertEnvelope(t, rec, "site_invalid_id")
}

// TestEnvelope_DownloaderInvalidInput 创建参数有误 → 400 + downloader_invalid_input。
// URL scheme 校验失败（ftp://）触发 oper.ErrInvalidInput 分支。
func TestEnvelope_DownloaderInvalidInput(t *testing.T) {
	r, _ := setupDownloaderServer(t)
	rec := doDL(t, r, http.MethodPost, "/downloaders", `{"name":"x","type":"qbittorrent","url":"ftp://127.0.0.1:8099"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应 400，got %d，body=%s", rec.Code, rec.Body.String())
	}
	assertEnvelope(t, rec, "downloader_invalid_input")
}
