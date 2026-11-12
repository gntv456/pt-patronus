// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// TestCheckNoUpdateWhenPlatformTargetEmpty 该平台 target 未发布下载（url/image 全空）
// 时不得报「发现新版本」：移动端更新框的「浏览器下载」按钮依赖 DownloadURL，
// 空按钮既不像故障也没有指引（OTA manifest 各端 url 曾全空）。
func TestCheckNoUpdateWhenPlatformTargetEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"latestVersion": "9.9.9",
			"targets": {
				"android": {"sha256": "` + string(make([]byte, 0)) + `0000000000000000000000000000000000000000000000000000000000000000"},
				"docker": {"image": "ghcr.io/x/y:9.9.9"}
			}
		}`))
	}))
	defer srv.Close()
	// AllowPrivate：httptest 服务器是 127.0.0.1，默认 SSRF 守卫会拦。
	hc, err := httpclient.New(httpclient.Options{Timeout: 5 * time.Second, AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{hc: hc, manifestURL: srv.URL}
	res, err := s.Check(context.Background(), "android", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if res.HasUpdate {
		t.Fatalf("android target 无 url 不得报更新（死按钮），got HasUpdate=true, latest=%s", res.LatestVersion)
	}
	// docker target 有 image：照常报更新。
	res2, err := s.Check(context.Background(), "docker", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.HasUpdate {
		t.Fatal("docker target 有 image 应照常报更新")
	}
}
