// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package httpclient

import (
	"context"
	"strings"
	"testing"
	"time"
)

// SSRF 激活时，拨号到环回地址必须被拦截（首跳与重定向后续跳都走同一路径）。
func TestSSRFDialBlocksPrivateWhenActive(t *testing.T) {
	ctx := context.WithValue(context.Background(), ssrfCtxKey{}, true)
	_, err := ssrfDialContext(ctx, "tcp", "127.0.0.1:9")
	if err == nil {
		t.Fatal("expected dial to loopback to be blocked under SSRF")
	}
	if !strings.Contains(err.Error(), "ssrf") {
		t.Fatalf("expected ssrf error, got %v", err)
	}
}

// SSRF 未激活（ssrf=false 的调用，如内网测试/下载器）应退化为普通拨号，不拦截私网。
func TestSSRFDialPassthroughWhenInactive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := ssrfDialContext(ctx, "tcp", "127.0.0.1:1")
	if err == nil {
		t.Fatal("expected dial error to a closed port")
	}
	if strings.Contains(err.Error(), "ssrf") {
		t.Fatalf("inactive SSRF should fall through to plain dial, got ssrf error: %v", err)
	}
}
