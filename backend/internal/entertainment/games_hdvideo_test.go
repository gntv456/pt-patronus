// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// HDVIDEO 老虎机：spin 动作解析（reels 三轴）。
func TestHdvideoSlotsBet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/games/slots/" {
			_ = r.ParseForm()
			if r.Form.Get("action") != "spin" || r.Form.Get("bet") == "" {
				_, _ = w.Write([]byte(`{"ok":false,"error":"bad request"}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"reels":["🍒","💎","7️⃣"],"win":2500}`))
			return
		}
		_, _ = w.Write([]byte(`<html>我的电影票：<b id="slBal">35.0</b> 张</html>`))
	}))
	defer srv.Close()
	hc := newTestClient(t)
	g := HdvideoSlotsGame{}
	res, err := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Amount: 100})
	if err != nil || res == nil || !res.Success {
		t.Fatalf("bet failed: %v %+v", err, res)
	}
	if !stringContains(res.Message, "2500") {
		t.Fatalf("win amount missing: %q", res.Message)
	}
}

// HDVIDEO 21点：deal 发牌 + 动作结果解析。
func TestHdvideoBlackjackFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/games/blackjack/" {
			_ = r.ParseForm()
			switch r.Form.Get("action") {
			case "deal":
				_, _ = w.Write([]byte(`{"ok":true,"player":["♥A","♠10"],"dealer":["♦5"],"score":21,"outcome":"blackjack","outcomeLabel":"黑杰克！"}`))
			case "hit":
				_, _ = w.Write([]byte(`{"ok":true,"player":["♥A","♠10","♥3"],"score":14,"outcome":"bust","outcomeLabel":"爆牌"}`))
			default:
				_, _ = w.Write([]byte(`{"ok":false,"error":"unknown action"}`))
			}
			return
		}
		_, _ = w.Write([]byte(`<html>我的电影票：<b>35.0</b></html>`))
	}))
	defer srv.Close()
	hc := newTestClient(t)
	g := HdvideoBlackjackGame{}

	// 发牌即 blackjack → finished/win
	res, err := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Amount: 100})
	if err != nil || res == nil || !res.Success {
		t.Fatalf("deal failed: %v %+v", err, res)
	}
	if !stringContains(res.Message, "黑杰克") {
		t.Fatalf("label missing: %q", res.Message)
	}

	// hit 动作 → bust → finished/lose
	r2, err := g.Blackjack(context.Background(), hc, srv.URL, Creds{}, "hit")
	if err != nil || r2 == nil || r2.Stage != "finished" || r2.Result != "lose" {
		t.Fatalf("hit parse wrong: %+v %v", r2, err)
	}
}

// HDVIDEO 刮刮乐：buy 解析 + PlayMulti 单张。
func TestHdvideoScratchPlayMulti(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "name": "谢谢参与", "reward": "0 票", "type": "none", "balance": 34.0})
	}))
	defer srv.Close()
	hc := newTestClient(t)
	g := HdvideoScratchGame{}
	res, err := g.PlayMulti(context.Background(), hc, srv.URL, Creds{}, 2)
	if err != nil || res == nil || !res.Success {
		t.Fatalf("playMulti failed: %v %+v", err, res)
	}
	if !stringContains(res.Message, "谢谢参与") || res.Balance != 34 {
		t.Fatalf("parse wrong: %+v", res)
	}
}
