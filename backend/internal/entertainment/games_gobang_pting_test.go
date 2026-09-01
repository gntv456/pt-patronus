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

// ptingGobangFixture GET /api/gobang 全景样例（结构与 pting.club 线上一致，字段名 camelCase）。
const ptingGobangStateFixture = `{"code":0,"message":"","data":{
  "matches":[
    {"id":"m1","status":"ONGOING","difficulty":"STANDARD",
     "board":[[0,0,0],[1,2,0]],
     "moves":[{"x":0,"y":0},{"x":1,"y":0}],
     "playerColor":"BLACK","currentSide":"PLAYER",
     "openingChoicePending":false,"openingMoveCount":0,
     "ticketCharged":true,"ticketCost":2,"winReward":3,"undoCost":1,"undoUsed":false,"ranked":true},
    {"id":"m0","status":"FINISHED","difficulty":"WARMUP",
     "board":[[1,1,1,1,1],[0,0,0,0,0]],
     "moves":[],"playerColor":"BLACK",
     "winnerId":7,"finishReason":"FIVE_IN_ROW","payoutAmount":1,"finishedAt":"2026-09-01T10:00:00Z"}
  ],
  "summary":{"points":42,"pointName":"蜂蜜","legacyPointName":"金币","totalLimit":10,"totalUsed":3,
    "totalRemaining":7,"warmupRemaining":1,"nightmareRemaining":1,
    "dailyNetGain":1.5,"dailyNetGainCap":8},
  "leaderboards":{"STANDARD":[
    {"userId":7,"rank":1,"username":"alice","nickname":"Alice","games":5,"wins":5,"losses":0,
     "winRate":100,"averageDurationSeconds":226,"averageMoves":47,
     "playerFirstGames":3,"aiFirstGames":2,"score":15}
  ]}
}}`

// TestPtingGobangState 全景解析：进行中/已结束对局、summary、周榜。
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
	if len(ongoing.Board) != 2 || ongoing.Board[1][0] != 1 || ongoing.Board[1][1] != 2 {
		t.Errorf("board = %+v", ongoing.Board)
	}
	if len(ongoing.Moves) != 2 || ongoing.Moves[1].X != 1 {
		t.Errorf("moves = %+v", ongoing.Moves)
	}
	fin := state.Matches[1]
	if fin.WinnerID == nil || *fin.WinnerID != 7 || fin.PayoutAmount != 1 {
		t.Errorf("finished = %+v", fin)
	}
	if s := state.Summary; s == nil || s.Points != 42 || s.PointName != "蜂蜜" || s.TotalRemaining != 7 {
		t.Errorf("summary = %+v", state.Summary)
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
				"winnerId":7,"finishReason":"FIVE_IN_ROW","payoutAmount":3,
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
	if st.Balance != 42 || st.TodaySpins != 3 || st.MaxDaily != 10 {
		t.Errorf("stats = %+v", st)
	}
}
