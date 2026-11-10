// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// nxpBJIdleFixture 无局页面：规则说明 + 开局表单（game=hit&start=yes，SSD 另带 join/amount）。
const nxpBJIdleFixture = `<html><body>
<h1>21点</h1><p>传统的21点游戏，和对手对抗。输局警告：系统每次提取赢家。</p>
<form method="post" action="blackjack.php">
<input type="hidden" name="game" value="hit" readonly="readonly"/>
<input type="hidden" name="start" value="yes" readonly="readonly"/>
<input type="submit" value="开牌"/>
</form>
<p>bonus 12,345.6</p></body></html>`

// nxpBJPlayingFixture 进行中：动作表单无 start=yes，牌面为 pic/cards/*.gif。
const nxpBJPlayingFixture = `<html><body>
<table><tr><td><img src="pic/cards/as.gif"/><img src="pic/cards/10h.gif"/></td></tr></table>
<form method="post" action="blackjack.php">
<input type="hidden" name="game" value="hit" readonly="readonly"/>
<input type="submit" value="要牌"/>
</form>
<form method="post" action="blackjack.php">
<input type="hidden" name="game" value="stand" readonly="readonly"/>
<input type="submit" value="停牌"/>
</form>
</body></html>`

// TestNxpBlackjackIdle 无局：stage=idle，消息提示网页开局（不自动下注）。
func TestNxpBlackjackIdle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, nxpBJIdleFixture)
	}))
	defer srv.Close()
	res, err := NxpBlackjackGame{}.Blackjack(context.Background(), newTestClient(t), srv.URL, Creds{}, "new")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "idle" {
		t.Errorf("stage = %q, want idle", res.Stage)
	}
	if res.Balance != 12345.6 {
		t.Errorf("balance = %v", res.Balance)
	}
}

// TestNxpBlackjackPlaying 进行中：动作表单（无 start=yes）→ playing，牌面从图片名解析（as→黑桃A、10h→红桃10）。
func TestNxpBlackjackPlaying(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, nxpBJPlayingFixture)
	}))
	defer srv.Close()
	res, err := NxpBlackjackGame{}.Blackjack(context.Background(), newTestClient(t), srv.URL, Creds{}, "new")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "playing" || res.Result != "playing" {
		t.Errorf("stage/result = %q/%q", res.Stage, res.Result)
	}
	if len(res.PlayerCards) != 2 || res.PlayerCards[0] != "♠A" || res.PlayerCards[1] != "♥10" {
		t.Errorf("cards = %v", res.PlayerCards)
	}
}

// TestNxpBlackjackAction POST hit 透传 game=hit 并解析新页面。
func TestNxpBlackjackAction(t *testing.T) {
	var gotForm string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			gotForm = r.Form.Get("game")
			_, _ = io.WriteString(w, nxpBJPlayingFixture)
			return
		}
		_, _ = io.WriteString(w, nxpBJIdleFixture)
	}))
	defer srv.Close()
	res, err := NxpBlackjackGame{}.Blackjack(context.Background(), newTestClient(t), srv.URL, Creds{}, "hit")
	if err != nil {
		t.Fatal(err)
	}
	if gotForm != "hit" {
		t.Errorf("posted game = %q", gotForm)
	}
	if res.Stage != "playing" {
		t.Errorf("stage = %q", res.Stage)
	}
}
