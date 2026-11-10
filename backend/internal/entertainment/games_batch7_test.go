// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// OpenCD 21点：idle 页（含可对抗赌局表单）解析。
func TestOpenCDBlackjackParse(t *testing.T) {
	page := `<html><body>魔力值 : 1,129,573.9
	<form method="post" action=/blackjack.php><input type="hidden" name="game" value="start"><input type="submit" value="賭這一局"><input type="hidden" name="gameid" value="651388"></form>
	<form method="post" action="/blackjack.php"><input type="hidden" name="game" value="start"><input type="text" size="8" name="bonus" value="500" /><input type="submit" value="開始新遊戲"></form>
	</body></html>`
	res := opencdBjParse(page)
	if res.Stage != "idle" {
		t.Fatalf("stage=%s want idle", res.Stage)
	}
	if !strings.Contains(res.Message, "1 个可对抗赌局") {
		t.Fatalf("message should mention 1 challengeable game: %q", res.Message)
	}
	if res.Balance < 1129573 {
		t.Fatalf("balance parse failed: %v", res.Balance)
	}
}

// OpenCD 21点：进行中页（hit/stand 动作表单，无 start=yes）。
func TestOpenCDBlackjackPlaying(t *testing.T) {
	page := `<html><body>♠10 ♥7 点数: 17
	<form method="post" action=/blackjack.php><input type="hidden" name="game" value="hit"><input type="submit" value="要牌"></form>
	</body></html>`
	res := opencdBjParse(page)
	if res.Stage != "playing" {
		t.Fatalf("stage=%s want playing", res.Stage)
	}
	if len(res.PlayerCards) < 2 {
		t.Fatalf("cards not parsed: %v", res.PlayerCards)
	}
}

// PANDA 好友买卖：InitOrHome 状态解析 + Bet 动作校验。
func TestPandaFriendTrade(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/ajax.php" {
			_, _ = w.Write([]byte(`{"ret":0,"msg":"OK","data":{"me":{"uid":1},"my_assets":[],"my_profile_price":"90000","claimable_income":"12","my_owner":{"uid":2,"username":"boss","redeem_price":"108000"}}}`))
			return
		}
		_, _ = w.Write([]byte(`<script>normalizeFriendTradeHome({"me":{"uid":1,"bonus":500}}); new Vue({})</script>`))
	}))
	defer srv.Close()
	hc := newTestClient(t)
	g := PandaFriendTradeGame{}

	res, err := g.Play(context.Background(), hc, srv.URL, Creds{})
	if err != nil || res == nil || !res.Success {
		t.Fatalf("play failed: %v %+v", err, res)
	}
	for _, want := range []string{"90000", "12", "boss", "108000"} {
		if !stringContains(res.Message, want) {
			t.Fatalf("message missing %s: %q", want, res.Message)
		}
	}

	st, err := g.Stats(context.Background(), hc, srv.URL, Creds{})
	if err != nil || st == nil || st.Balance != 500 {
		t.Fatalf("stats failed: %v %+v", err, st)
	}

	bad, _ := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Selection: "buy"})
	if bad == nil || bad.Success {
		t.Fatalf("unsupported action should fail")
	}
}
