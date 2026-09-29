package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 蓝影五子棋：httptest mock /api/gobang，验证 Play 全流程（create→自动落子→终局胜负解析）
// 与 Stats 的 RSC initialSummary 解析。
func TestHdblueGobangPlay(t *testing.T) {
	const playerID = 1938
	var board [15][15]int
	finished := false
	var winner *int
	playerMoves := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/gobang" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Action  string `json:"action"`
			MatchID string `json:"matchId"`
			X       int    `json:"x"`
			Y       int    `json:"y"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Action {
		case "create":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "message": "已开始免费挑战",
				"data": map[string]any{
					"matches": []any{map[string]any{
						"id": "m1", "creatorId": playerID, "status": "ONGOING",
						"ticketCost": 0, "winReward": 20, "challengeMode": "FREE",
						"firstHand": "PLAYER", "currentSide": "PLAYER", "moves": []any{},
						"board": board,
					}},
				},
			})
		case "move":
			mv := req
			if finished {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "message": "对局已结束"})
				return
			}
			board[mv.Y][mv.X] = 1
			playerMoves++
			// mock AI：直接认输（第 3 手后判玩家胜）——胜负逻辑由真实 AI 决定，这里只测解析
			if playerMoves >= 2 {
				finished = true
				wp := playerID
				winner = &wp
			} else {
				board[7][6] = 2 // AI 随便下一手
			}
			st := "ONGOING"
			if finished {
				st = "FINISHED"
			}
			side := "PLAYER"
			if !finished {
				side = "PLAYER"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "message": "落子成功",
				"data": map[string]any{
					"match": map[string]any{
						"id": "m1", "creatorId": playerID, "status": st,
						"winnerId": winner, "ticketCost": 0, "winReward": 20,
						"challengeMode": "FREE", "firstHand": "PLAYER",
						"currentSide": side, "moves": []any{map[string]any{"playerId": playerID, "step": 1, "x": mv.X, "y": mv.Y}},
						"board": board,
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "message": "未知 action"})
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := HdblueGobangGame{}
	out, err := g.Play(context.Background(), hc, srv.URL, Creds{Cookie: "bbs_session=x"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Success {
		t.Fatalf("play failed: %s", out.Message)
	}
	if !strings.Contains(out.Prize, "胜利") {
		t.Errorf("prize = %q, want 胜利（mock 第 3 手判玩家胜）", out.Prize)
	}
	if out.Amount != 20 {
		t.Errorf("amount = %v, want 20", out.Amount)
	}
}

func TestHdblueGobangStats(t *testing.T) {
	rsc := `<script>self.__next_f.push([1,"43:I[...,{\"initialSummary\":{\"pointName\":\"影币\",\"points\":40,\"freeTotal\":1,\"freeUsed\":1,\"freeRemaining\":0,\"paidTotal\":2,\"freeUsedCopy\":0,\"paidRemaining\":2,\"challengeStatus\":\"not_started\"}}]"])</script>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(rsc))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	st, err := HdblueGobangGame{}.Stats(context.Background(), hc, srv.URL, Creds{Cookie: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Balance != 40 {
		t.Errorf("balance = %v, want 40", st.Balance)
	}
	if st.MaxDaily != 3 {
		t.Errorf("maxDaily = %v, want 3 (1免费+2付费)", st.MaxDaily)
	}
}

func TestHdblueYinyangPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/yinyang-contract" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "挑战已创建",
			"data": map[string]any{
				"summary": map[string]any{
					"userId": 1938, "pointName": "影币", "points": 0,
					"dailyCreateLimit": 5, "createdToday": 1,
				},
			},
		})
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	out, err := HdblueYinyangGame{}.Play(context.Background(), hc, srv.URL, Creds{Cookie: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Success {
		t.Fatalf("play failed: %s", out.Message)
	}
	if !strings.Contains(out.Prize, "阴阳契") {
		t.Errorf("prize = %q", out.Prize)
	}
	if !strings.Contains(out.Message, "1/5") {
		t.Errorf("message = %q, want 含今日已发起 1/5", out.Message)
	}
}

// 贪心 AI 冒烟：能成五连（对全空盘自弈玩家侧应产生合理落子）
func toSlice(b *[15][15]int) [][]int {
	out := make([][]int, len(b))
	for i := range b {
		out[i] = b[i][:]
	}
	return out
}

func TestGobangBestMove(t *testing.T) {
	var b [15][15]int
	// 玩家已有横向活三 (7,3)(7,4)(7,5)，最优应为两端延伸
	b[7][3], b[7][4], b[7][5] = 1, 1, 1
	x, y, ok := gobangBestMove(toSlice(&b), 1, 2)
	if !ok {
		t.Fatal("no move")
	}
	if y != 7 || (x != 2 && x != 6) {
		t.Errorf("move = (%d,%d), want (2,7) 或 (6,7) 延长活三", x, y)
	}
	// 对方冲四必堵：AI 侧 (0,0..3) 竖四冲四，玩家应堵 (0,4) 或 (0,-)
	var b2 [15][15]int
	for i := 0; i < 4; i++ {
		b2[i][0] = 2
	}
	x2, y2, ok := gobangBestMove(toSlice(&b2), 1, 2)
	if !ok {
		t.Fatal("no move 2")
	}
	// 必堵点：竖四上端 (x=0列, y=4行)（下端出界）
	if x2 != 0 || y2 != 4 {
		t.Errorf("block move = (%d,%d), want (0,4) 封堵冲四", x2, y2)
	}
}

// 阴阳契智能模式：大厅有他人 OPEN 挑战时走 accept（选 A），无挑战时降级 create。
func TestHdblueYinyangAcceptFirst(t *testing.T) {
	rsc := `<script>self.__next_f.push([1,"7:{\"initialData\":{\"summary\":{\"userId\":1938,\"pointName\":\"影币\",\"points\":50,\"winCount\":0,\"loseCount\":0,\"todayProfitPoints\":0,\"todayLossPoints\":0,\"totalProfitPoints\":0,\"totalLossPoints\":0,\"dailyCreateLimit\":5,\"dailyAcceptLimit\":10,\"createdToday\":0,\"acceptedToday\":0},\"openChallenges\":[{\"id\":\"ch-1\",\"creatorId\":5,\"creatorName\":\"lu\",\"status\":\"OPEN\",\"question\":\"sky color?\",\"optionA\":\"blue\",\"optionB\":\"red\",\"correctOption\":null,\"stakePoints\":10,\"rewardPoints\":9,\"taxPoints\":1,\"winnerId\":null,\"loserId\":null}],\"recentChallenges\":[]}}"])</script>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/funs/yinyang-contract":
			_, _ = w.Write([]byte(rsc))
		case "/api/yinyang-contract":
			var req struct {
				Action         string `json:"action"`
				ChallengeID    string `json:"challengeId"`
				SelectedOption string `json:"selectedOption"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Action != "accept" {
				t.Errorf("action = %q, want accept（有公开挑战时应优先应战）", req.Action)
			}
			if req.ChallengeID != "ch-1" || req.SelectedOption != "A" {
				t.Errorf("accept body = %+v", req)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "message": "应战成功",
				"data": map[string]any{"summary": map[string]any{"userId": 1938, "points": 60}},
			})
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	out, err := HdblueYinyangGame{}.Play(context.Background(), hc, srv.URL, Creds{Cookie: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Success || !strings.Contains(out.Prize, "已应战") {
		t.Errorf("out = %+v, want 已应战分支", out)
	}
	if out.Balance != 60 {
		t.Errorf("balance = %v, want 60", out.Balance)
	}
}

// 阴阳契：第一个挑战应战业务失败（如已被他人应战）→ 自动换下一个；
// 全部失败才带出最后一次原因，不再盲目降级发起（发起还要押影币）。
func TestHdblueYinyangAcceptRetriesNext(t *testing.T) {
	rsc := `<script>self.__next_f.push([1,"7:{\"initialData\":{\"summary\":{\"userId\":7,\"pointName\":\"影币\",\"points\":100,\"dailyCreateLimit\":5,\"dailyAcceptLimit\":10,\"createdToday\":0,\"acceptedToday\":0},\"openChallenges\":[{\"id\":\"c1\",\"creatorId\":9,\"status\":\"OPEN\",\"question\":\"题一\",\"optionA\":\"A1\",\"optionB\":\"B1\",\"stakePoints\":10},{\"id\":\"c2\",\"creatorId\":9,\"status\":\"OPEN\",\"question\":\"题二\",\"optionA\":\"A2\",\"optionB\":\"B2\",\"stakePoints\":10}]}}"])</script>`
	accepts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/funs/yinyang-contract":
			_, _ = w.Write([]byte(rsc))
		case "/api/yinyang-contract":
			accepts++
			if accepts == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 409, "message": "该挑战已被应战"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "message": "应战成功",
				"data": map[string]any{"summary": map[string]any{"userId": 7, "points": 90}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	out, err := HdblueYinyangGame{}.Play(context.Background(), hc, srv.URL, Creds{Cookie: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Success || !strings.Contains(out.Prize, "题二") {
		t.Fatalf("第一个失败后应自动换下一个挑战: %+v", out)
	}
	if accepts != 2 {
		t.Errorf("accept 次数 = %d, want 2", accepts)
	}
}

// 转义形态（RSC flight 流 \"key\":…）+ 字符串内容含花括号：须先还原外层转义再配平，
// 否则 \" 被当转义符吞掉、字符串永不闭合，内容里的 { } 会破坏配平。
func TestHdblueExtractJSONEscapedBraces(t *testing.T) {
	body := `2:{\"summary\":{\"userId\":7,\"note\":\"玩法{a}说明\",\"odd\":3}}:end`
	got := hdblueExtractJSON(body, `\"summary\":{\"userId\"`)
	if got == nil {
		t.Fatal("转义形态应能提取")
	}
	var m struct {
		UserID int    `json:"userId"`
		Note   string `json:"note"`
		Odd    int    `json:"odd"`
	}
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("提取段应为合法 JSON: %v (%s)", err, got)
	}
	if m.UserID != 7 || m.Note != "玩法{a}说明" || m.Odd != 3 {
		t.Errorf("字段不完整: %+v", m)
	}
}

// 裸形态、key 中段含对象起始 '{'（"summary":{"userId"）：起点应落在该 '{'，
// 且字符串内容里的花括号不得破坏配平。
func TestHdblueExtractJSONRawMidKeyBrace(t *testing.T) {
	body := `xx"summary":{"userId":7,"note":"a{b}c"}yy`
	got := hdblueExtractJSON(body, `"summary":{"userId"`)
	if got == nil {
		t.Fatal("裸形态应能提取")
	}
	if want := `{"userId":7,"note":"a{b}c"}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
