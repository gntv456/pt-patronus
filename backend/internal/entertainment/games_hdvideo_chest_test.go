// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// HDVIDEO 签到宝箱：页面解析（连续天数 + 里程碑状态）。
func TestHdvideoChestParse(t *testing.T) {
	page := `<html><body>我的电影票：35.0 张 当前连续签到：2 天
	<div class="cb-chests">
	<div class="cb-chest "><div class="ico">🔒</div><div class="ms">连续 7 天</div><button type="button" class="cb-btn" disabled>还差 5 天</button></div>
	<div class="cb-chest open"><div class="ico">🎁</div><div class="ms">连续 15 天</div><button type="button" class="cb-btn">立即开启</button></div>
	<div class="cb-chest done"><div class="ico">✅</div><div class="ms">连续 30 天</div><button type="button" class="cb-btn" disabled>已领取</button></div>
	</div></body></html>`
	streak, ms := hdvChestParse(page)
	if streak != 2 {
		t.Fatalf("streak=%d want 2", streak)
	}
	if len(ms) != 3 || ms[0] != "7天:locked" || ms[1] != "15天:open" || ms[2] != "30天:done" {
		t.Fatalf("milestones wrong: %v", ms)
	}
}

// HDVIDEO 签到宝箱：开箱接口 + 里程碑校验。
func TestHdvideoChestBet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseForm()
		if r.Form.Get("action") == "open" && r.Form.Get("m") == "7" {
			_, _ = w.Write([]byte(`{"ok":true,"label":"电影票 ×20","balance":55.0}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":false,"error":"bad"}`))
	}))
	defer srv.Close()
	hc := newTestClient(t)
	g := HdvideoChestGame{}

	bad, _ := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Selection: "3"})
	if bad == nil || bad.Success {
		t.Fatalf("m=3 should be rejected")
	}
	res, err := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Selection: "7"})
	if err != nil || res == nil || !res.Success {
		t.Fatalf("open failed: %v %+v", err, res)
	}
	if res.Balance != 55 {
		t.Fatalf("balance wrong: %v", res.Balance)
	}
}

// HDVIDEO 股票：stockState 提取 + Play 只读。
func TestHdvideoStockPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><script>var stockState = {"quotes":[{"symbol":"SH600519","name":"贵州茅台","price":1295.99}],"portfolio":{"holdings":[],"market_value":0,"profit":0},"market":{"open":true,"label":"交易中"},"wallet":35,"token":"tok123"}; const stockEndpoint='x';</script></html>`))
	}))
	defer srv.Close()
	hc := newTestClient(t)
	g := HdvideoStockGame{}
	res, err := g.Play(context.Background(), hc, srv.URL, Creds{})
	if err != nil || res == nil || !res.Success {
		t.Fatalf("play failed: %v %+v", err, res)
	}
	if res.Balance != 35 {
		t.Fatalf("wallet wrong: %v", res.Balance)
	}
	// Selection 格式校验
	bad, _ := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Selection: "buy"})
	if bad == nil || bad.Success {
		t.Fatalf("missing symbol should fail")
	}
}
