// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 麒麟双色球：GET /nodeapi/union-lotto.php 状态解析 + 选号校验 + POST 落点。
func TestHdkylinLottoStateAndBet(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "message": "查询本期购买信息成功",
				"data": map[string]any{
					"time": "2026-09-03 21:15:00", "list": []any{},
					"totalAmount": 3457000, "UnionLottoInceptionAmount": 1000000,
				},
			})
			return
		}
		gotPath = r.URL.Path
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "购买成功"})
	}))
	defer srv.Close()
	hc := newTestClient(t)
	g := HdkylinLottoGame{}

	res, err := g.Play(context.Background(), hc, srv.URL, Creds{})
	if err != nil || res == nil || !res.Success {
		t.Fatalf("play failed: %v %+v", err, res)
	}
	if !stringContains(res.Message, "3457000") {
		t.Fatalf("pool not shown as plain number: %q", res.Message)
	}

	// 非法选号（蓝球越界）
	bad, _ := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Selection: "1,2,3,4,5,6|17"})
	if bad == nil || bad.Success {
		t.Fatalf("blue 17 should be rejected")
	}
	// 合法选号落到 /nodeapi 前缀 + json nums
	ok, _ := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Selection: "1,2,3,4,5,6|7"})
	if ok == nil || !ok.Success {
		t.Fatalf("valid bet should succeed: %+v", ok)
	}
	if !strings.HasPrefix(gotPath, "/nodeapi/union-lotto.php") || !strings.Contains(gotBody, "nums") {
		t.Fatalf("bet posted wrong: path=%s body=%s", gotPath, gotBody)
	}
}
