// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ptingGobangFixture GET /api/gobang 全景样例（2026-09 真实联调抓取的字段结构，camelCase）。
const ptingGobangStateFixture = `{"code":0,"message":"","data":{
  "matches":[
    {"id":"m1","status":"ONGOING","difficulty":"STANDARD",
     "board":[[0,0,0],[1,2,0]],
     "moves":[{"id":"v1","step":1,"x":0,"y":0,"playerId":7,"createdAt":"2026-09-01T10:00:00Z"},
              {"id":"v2","step":2,"x":1,"y":0,"playerId":0,"createdAt":"2026-09-01T10:00:02Z"}],
     "playerColor":"BLACK","currentSide":"PLAYER","firstHand":"PLAYER","rule":"FREESTYLE",
     "openingChoicePending":false,"openingMoveCount":0,
     "ticketCharged":true,"ticketCost":2,"winReward":3,"undoCost":1,"undoUsed":false,
     "ranked":true,"rankScore":0},
    {"id":"m0","status":"FINISHED","difficulty":"WARMUP",
     "board":[[1,1,1,1,1],[0,0,0,0,0]],
     "moves":[],"playerColor":"BLACK",
     "winnerId":7,"finishReason":"FIVE","payoutAmount":1,"finishedAt":"2026-09-01T10:00:00Z"}
  ],
  "summary":{"pointName":"蜂蜜","legacyPointName":"蜂蜜","points":42,"totalLimit":30,"totalUsed":3,
    "totalRemaining":27,"warmupRemaining":1,"nightmareRemaining":10,
    "freeTotal":5,"freeUsed":4,"freeRemaining":1,"paidTotal":25,"paidUsed":0,"paidRemaining":25,
    "dailyNetGain":1.5,"dailyNetGainCap":8,"economyEnabled":true,"creditSettlementEnabled":true},
  "leaderboards":{"STANDARD":[
    {"userId":7,"rank":1,"username":"alice","nickname":"Alice","games":5,"wins":5,"losses":0,
     "winRate":100,"averageDurationSeconds":226,"averageMoves":47,
     "playerFirstGames":3,"aiFirstGames":2,"score":15}
  ]}
}}`

// TestPtingGobangState 全景解析：进行中/已结束对局（含 moves 元数据）、summary、周榜。
func TestPtingGobangState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/gobang" {
			_, _ = io.WriteString(w, ptingGobangStateFixture)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	state, err := PtingGobangGame{}.GobangState(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "flarum_session=x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(state.Matches))
	}
	ongoing := state.Matches[0]
	if ongoing.ID != "m1" || ongoing.Status != "ONGOING" || ongoing.CurrentSide != "PLAYER" {
		t.Errorf("ongoing = %+v", ongoing)
	}
	if ongoing.FirstHand != "PLAYER" || ongoing.Rule != "FREESTYLE" {
		t.Errorf("firstHand/rule = %q/%q", ongoing.FirstHand, ongoing.Rule)
	}
	if len(ongoing.Board) != 2 || ongoing.Board[1][0] != 1 || ongoing.Board[1][1] != 2 {
		t.Errorf("board = %+v", ongoing.Board)
	}
	// moves 元数据：step 递增；playerId 0=AI、7=用户（真实联调确认）
	if len(ongoing.Moves) != 2 {
		t.Fatalf("moves = %+v", ongoing.Moves)
	}
	if ongoing.Moves[0].Step != 1 || ongoing.Moves[0].PlayerID != 7 || ongoing.Moves[0].X != 0 || ongoing.Moves[0].Y != 0 {
		t.Errorf("move0 = %+v", ongoing.Moves[0])
	}
	if ongoing.Moves[1].PlayerID != 0 || ongoing.Moves[1].X != 1 {
		t.Errorf("move1 = %+v", ongoing.Moves[1])
	}
	fin := state.Matches[1]
	if fin.WinnerID == nil || *fin.WinnerID != 7 || fin.PayoutAmount != 1 || fin.FinishReason != "FIVE" {
		t.Errorf("finished = %+v", fin)
	}
	s := state.Summary
	if s == nil || s.Points != 42 || s.PointName != "蜂蜜" || s.TotalRemaining != 27 ||
		s.FreeRemaining != 1 || s.PaidRemaining != 25 || s.DailyNetGainCap != 8 {
		t.Errorf("summary = %+v", s)
	}
	lb := state.Leaderboards["STANDARD"]
	if len(lb) != 1 || lb[0].Nickname != "Alice" || lb[0].Score != 15 {
		t.Errorf("leaderboard = %+v", lb)
	}
}

// TestPtingGobangActionMove move 动作：JSON body 透传 + 胜负映射 PlayResult。
func TestPtingGobangActionMove(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/gobang" {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = io.WriteString(w, `{"code":0,"message":"落子成功","data":{"match":{
				"id":"m1","status":"FINISHED","difficulty":"STANDARD",
				"winnerId":7,"finishReason":"FIVE","payoutAmount":3,
				"playerColor":"BLACK"}}}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	res, err := PtingGobangGame{}.GobangAction(context.Background(), newTestClient(t), srv.URL,
		Creds{Cookie: "flarum_session=x"}, "move", map[string]any{"matchId": "m1", "x": 7, "y": 8})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Prize != "胜利" || res.Amount != 3 || res.Message != "落子成功" {
		t.Errorf("move result = %+v", res)
	}
	if gotBody["action"] != "move" || gotBody["matchId"] != "m1" || gotBody["x"] != float64(7) || gotBody["y"] != float64(8) {
		t.Errorf("post body = %+v", gotBody)
	}
}

// TestPtingGobangActionCreate create 动作：data.matches（而非 match）内找新局 + summary。
func TestPtingGobangActionCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"message":"已创建WARMUP挑战，首次有效落子时扣门票","data":{
			"matches":[{"id":"m2","status":"ONGOING","difficulty":"WARMUP","board":[],
				"moves":[],"playerColor":"BLACK","currentSide":"PLAYER","ticketCost":0,"winReward":1}],
			"summary":{"pointName":"蜂蜜","points":1,"totalLimit":30,"totalUsed":0,"totalRemaining":30,
				"warmupRemaining":4,"dailyNetGain":0,"dailyNetGainCap":8},
			"policy":{},"aiPending":false,"warning":""}}`)
	}))
	defer srv.Close()

	res, err := PtingGobangGame{}.GobangAction(context.Background(), newTestClient(t), srv.URL,
		Creds{}, "create", map[string]any{"difficulty": "WARMUP"})
	if err != nil {
		t.Fatal(err)
	}
	// create 后轮到玩家 → Prize 提示轮次
	if !res.Success || res.Prize != "轮到你落子" {
		t.Errorf("create result = %+v", res)
	}
}

// TestPtingGobangActionRateLimited 站点限速（真实联调确认 ~1s/手）：code 429 → 失败结果透传 message。
func TestPtingGobangActionRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":429,"message":"落子过于频繁，请稍后再试"}`)
	}))
	defer srv.Close()

	res, err := PtingGobangGame{}.GobangAction(context.Background(), newTestClient(t), srv.URL,
		Creds{}, "move", map[string]any{"matchId": "m1", "x": 1, "y": 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || res.Message != "落子过于频繁，请稍后再试" {
		t.Errorf("rate-limited result = %+v", res)
	}
}

// TestPtingGobangActionUnauthorized 未登录（401 envelope）→ 失败结果带站点 message。
func TestPtingGobangActionUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":401,"message":"请先登录"}`)
	}))
	defer srv.Close()

	res, err := PtingGobangGame{}.GobangAction(context.Background(), newTestClient(t), srv.URL,
		Creds{}, "create", map[string]any{"difficulty": "WARMUP"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || res.Message != "请先登录" {
		t.Errorf("unauth result = %+v", res)
	}
}

// TestPtingGobangActionUnknown 未知动作被白名单拦截（不打站点）。
func TestPtingGobangActionUnknown(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(404)
	}))
	defer srv.Close()

	res, err := PtingGobangGame{}.GobangAction(context.Background(), newTestClient(t), srv.URL,
		Creds{}, "hack", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || !strings.Contains(res.Message, "未知动作") || called {
		t.Errorf("unknown action result = %+v called=%v", res, called)
	}
}

// TestPtingGobangStats summary 映射 StatsResult（余额/今日次数/上限）。
func TestPtingGobangStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, ptingGobangStateFixture)
	}))
	defer srv.Close()

	st, err := PtingGobangGame{}.Stats(context.Background(), newTestClient(t), srv.URL, Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Balance != 42 || st.TodaySpins != 3 || st.MaxDaily != 30 {
		t.Errorf("stats = %+v", st)
	}
}

// TestPtingGobangStateCookieExpired 站点 cookie 失效：GET 401「请先登录」→ 错误透传原文案，
// 而非落进 handler 的 InvalidInput 笼统文案（前端据此提示更新站点 cookie）。
func TestPtingGobangStateCookieExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":401,"message":"请先登录"}`)
	}))
	defer srv.Close()

	_, err := PtingGobangGame{}.GobangState(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "flarum_session=stale"})
	if err == nil || !strings.Contains(err.Error(), "请先登录") {
		t.Errorf("want 请先登录 error, got %v", err)
	}
}
