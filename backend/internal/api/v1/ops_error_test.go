// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// TestOpsTargetMissingIs404Not502 运维中心选了已被删除的主机：runner 底层抛 gorm 的
// record not found，应 404 说清「运维目标不存在」，而不是 502「上游服务不可达」把用户
// 支去查网络、查 agent、查 Docker 服务。同一条里钉住「其它失败仍是 502」，
// 防止把超时/远端故障一起吞进 404。
func TestOpsTargetMissingIs404Not502(t *testing.T) {
	cases := map[string]error{
		"裸错误":     gorm.ErrRecordNotFound,
		"包装过的错误":  fmt.Errorf("monitor: %w", gorm.ErrRecordNotFound),
		"再包一层的链式": fmt.Errorf("run command: %w", fmt.Errorf("store get: %w", gorm.ErrRecordNotFound)),
	}
	for name, err := range cases {
		h := NewDockerHandler(&mockRunner{err: err})
		w := doJSONReq(t, newDockerTestRouter(h), http.MethodGet, "/list/1", "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s: 目标不存在应 404，got %d body=%s", name, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "monitor_target_not_found") {
			t.Fatalf("%s: 应带 monitor_target_not_found，got %s", name, w.Body.String())
		}
	}
	h2 := NewDockerHandler(&mockRunner{err: context.DeadlineExceeded})
	if w2 := doJSONReq(t, newDockerTestRouter(h2), http.MethodGet, "/list/1", ""); w2.Code != http.StatusBadGateway {
		t.Fatalf("超时仍应 502，got %d", w2.Code)
	}
}
