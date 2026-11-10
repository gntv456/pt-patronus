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

// 不可躺猜大小：snapshot 快照解析（复用 httptest，不触真站）。
func TestTangptBigSmallState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"state": map[string]any{
					"balance": 219122213.0,
					"current_issue": map[string]any{
						"issue_no": "202609030634", "betting_open": true, "status": "open",
						"betting_seconds": 55.0, "min_bet": 1000.0, "odds": "0.9",
						"total_big": 12000.0, "total_small": 800.0,
					},
					"recent_issues": []any{
						map[string]any{"result_side": "big"},
						map[string]any{"result_side": "small"},
					},
				},
				"my_bets": map[string]any{"items": []any{}},
			},
		})
	}))
	defer srv.Close()
	hc := newTestClient(t)
	st, err := (TangptBigSmallGame{}).BigSmallState(context.Background(), hc, srv.URL, Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Balance != 219122213 || st.Round.GameNo != "202609030634" || st.Round.Status != "betting" {
		t.Fatalf("state parse wrong: %+v", st)
	}
	if st.MinBet != 1000 || len(st.RecentResults) != 2 {
		t.Fatalf("min_bet/recent wrong: %v %v", st.MinBet, st.RecentResults)
	}
	// Bet 非法 side 拒绝
	bad, _ := (TangptBigSmallGame{}).Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Selection: "middle"})
	if bad == nil || bad.Success {
		t.Fatalf("invalid side should fail")
	}
}

// 不可躺老虎机：stats 只读 + draw 缺 token 校验。
func TestTangptSlotPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/web/omnibot/slot-machine/stats":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"config":                     map[string]any{"base_cost": 5000.0},
				"user_bonus":                 219122213.0,
				"jackpot_pool":               808000.0,
				"daily_play_remaining":       10000.0,
				"daily_free_spins_remaining": 2.0,
				"spin_token":                 "abc123",
				"daily_play_limit":           10000.0,
				"daily_play_used":            0.0,
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}
	}))
	defer srv.Close()
	hc := newTestClient(t)
	res, err := (TangptSlotGame{}).Play(context.Background(), hc, srv.URL, Creds{})
	if err != nil || res == nil || !res.Success {
		t.Fatalf("play failed: %v %+v", err, res)
	}
	if res.Balance != 219122213 {
		t.Fatalf("balance wrong: %v", res.Balance)
	}
	st, err := (TangptSlotGame{}).Stats(context.Background(), hc, srv.URL, Creds{})
	if err != nil || st.Balance != 219122213 || st.MaxDaily != 10000 {
		t.Fatalf("stats wrong: %+v %v", st, err)
	}
}
