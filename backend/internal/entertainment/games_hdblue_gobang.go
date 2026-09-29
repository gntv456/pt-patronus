package entertainment

// 蓝影论坛（hdblue.cc，Next.js 自建论坛）五子棋人机对战适配。
// API（2026-09-06 实测，docs/hdblue/api-notes.md）：
//
//	POST /api/gobang {"action":"create"}                 → 开局（免费优先，无免费时扣 10 影币）
//	POST /api/gobang {"action":"move","matchId","x","y"} → 落子（0-based）+ AI 回应
//	POST /api/gobang {"action":"undo","matchId"}         → 悔棋（按钮存在，字段按同构推断）
//
// 响应 {"code":0,"message":..,"data":{"matches":[match]|match?,"summary"?}}，
// match{status ONGOING/FINISHED, winnerId, board[15][15] 0空/1玩家/2AI, moves[], currentSide}。
// Play 语义：开局 → 内置贪心五连 AI 自动弈至终局 → 返回胜负与影币变动（免费胜 +20 / 付费胜返 30）。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

type HdblueGobangGame struct{}

func (HdblueGobangGame) ID() string   { return "hdblue_gobang" }
func (HdblueGobangGame) Name() string { return "五子棋人机对战" }

// hdblueResp 蓝影 JSON API 统一外壳。
type hdblueResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// hdblueMatch 一局对弈。
type hdblueMatch struct {
	ID            string       `json:"id"`
	CreatorID     int          `json:"creatorId"` // 发起人=本玩家（单用户会话）
	Status        string       `json:"status"`    // ONGOING | FINISHED
	WinnerID      *int         `json:"winnerId"`
	TicketCost    float64      `json:"ticketCost"`
	WinReward     float64      `json:"winReward"`
	ChallengeMode string       `json:"challengeMode"` // FREE | PAID
	FirstHand     string       `json:"firstHand"`     // PLAYER | AI
	CurrentSide   string       `json:"currentSide"`
	Moves         []hdblueMove `json:"moves"`
	Board         [][]int      `json:"board"`
}

type hdblueMove struct {
	PlayerID int `json:"playerId"`
	Step     int `json:"step"`
	X        int `json:"x"`
	Y        int `json:"y"`
}

// hdblueSummary 开局响应附带的积分汇总。
type hdblueSummary struct {
	PointName       string  `json:"pointName"`
	Points          float64 `json:"points"`
	FreeTotal       int     `json:"freeTotal"`
	FreeRemaining   int     `json:"freeRemaining"`
	PaidTotal       int     `json:"paidTotal"`
	PaidRemaining   int     `json:"paidRemaining"`
	ChallengeStatus string  `json:"challengeStatus"`
}

// hdblueCall 调一次 /api/gobang。code!=0 视为业务失败（免费次数用尽/积分不足等）。
func hdblueCall(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, body string) (hdblueResp, error) {
	var out hdblueResp
	u := strings.TrimRight(baseURL, "/") + "/api/gobang"
	resp, err := hc.Do(ctx, "POST", u, []byte(body), map[string]string{
		"Content-Type": "application/json",
		"Referer":      strings.TrimRight(baseURL, "/") + "/funs/gobang",
	}, cr.Cookie, true)
	if err != nil {
		return out, err
	}
	logResp("hdblue_gobang", u, resp)
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return out, fmt.Errorf("响应非 JSON（可能未登录或 session 失效）: %w", err)
	}
	return out, nil
}

// createEnvelope create 响应的 data：matches 数组（开局即含进行中对局）。
type createEnvelope struct {
	Matches []hdblueMatch  `json:"matches"`
	Summary *hdblueSummary `json:"summary"`
}

// moveEnvelope move 响应的 data：match 单对象。
type moveEnvelope struct {
	Match   hdblueMatch    `json:"match"`
	Summary *hdblueSummary `json:"summary"`
}

func (g HdblueGobangGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	// 1. 开局
	resp, err := hdblueCall(ctx, hc, baseURL, cr, `{"action":"create"}`)
	if err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return &PlayResult{Success: false, Message: resp.Message}, nil
	}
	var env createEnvelope
	if err := json.Unmarshal(resp.Data, &env); err != nil || len(env.Matches) == 0 {
		return &PlayResult{Success: false, Message: "开局响应异常：" + resp.Message}, nil
	}
	m := env.Matches[0]
	mode := "免费"
	if m.ChallengeMode == "PAID" {
		mode = "付费"
	}

	// 2. 自动弈至终局（最多 225 手防御）。蓝影 move 是同步接口：玩家落子响应里
	//    已带 AI 回应（moves 成对增长，实测第一步玩家+AI 各 1 手）。
	playerID := m.CreatorID // 发起人=本玩家（单用户会话）
	if playerID == 0 && len(m.Moves) > 0 {
		playerID = m.Moves[0].PlayerID
	}
	for step := 0; step < 225 && m.Status == "ONGOING"; step++ {
		if m.CurrentSide != "PLAYER" {
			break // AI 回应未随 move 返回（异常局面），留待用户在网页端续玩
		}
		x, y, ok := gobangBestMove(m.Board, 1, 2) // board: 1=玩家 2=AI
		if !ok {
			break // 棋盘满
		}
		mv, err := g.move(ctx, hc, baseURL, cr, m.ID, x, y)
		if err != nil {
			return nil, err
		}
		if mv.Code != 0 {
			return &PlayResult{Success: false, Message: "落子失败：" + mv.Message}, nil
		}
		var me moveEnvelope
		if err := json.Unmarshal(mv.Data, &me); err != nil {
			return &PlayResult{Success: false, Message: "落子响应异常"}, nil
		}
		m = me.Match
	}

	// 3. 终局判定
	out := &PlayResult{Success: true, Message: mode + "对局 " + resp.Message}
	switch {
	case m.Status != "FINISHED":
		out.Prize = fmt.Sprintf("对局进行中（%d 手）", len(m.Moves))
	case m.WinnerID != nil && *m.WinnerID == playerID:
		out.Prize = fmt.Sprintf("胜利！%s局 +%g 影币", mode, m.WinReward)
		out.Amount = m.WinReward
		out.Prizes = []PrizeOutcome{{PrizeName: out.Prize, Amount: m.WinReward, IsWin: true}}
	case m.WinnerID == nil:
		out.Prize = "平局"
	default:
		out.Prize = "败北（AI 胜）"
		if m.ChallengeMode == "PAID" {
			out.Message = mode + "对局失败，不退影币"
		}
	}
	if env.Summary != nil {
		out.Balance = env.Summary.Points
	}
	out.Raw = map[string]any{"match_id": m.ID, "moves": len(m.Moves), "status": m.Status}
	return out, nil
}

// move 落一子。
func (HdblueGobangGame) move(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, matchID string, x, y int) (hdblueResp, error) {
	body := fmt.Sprintf(`{"action":"move","matchId":%q,"x":%d,"y":%d}`, matchID, x, y)
	return hdblueCall(ctx, hc, baseURL, cr, body)
}

// Stats RSC 页面抓积分/次数（GET /funs/gobang + RSC 头，正则抽 initialSummary JSON 段）。
func (HdblueGobangGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	u := strings.TrimRight(baseURL, "/") + "/funs/gobang"
	resp, err := hc.GetWithHeaders(ctx, u, cr.Cookie, map[string]string{"RSC": "1"}, true)
	if err != nil {
		return nil, err
	}
	body := string(resp.Body)
	// RSC flight 流内嵌 JSON 可能带反斜杠转义（\"initialSummary\"）或裸键两种形态
	seg := hdblueExtractJSON(body, `"initialSummary":{`)
	if seg == nil {
		seg = hdblueExtractJSON(body, `\"initialSummary\":{`)
	}
	if seg == nil {
		return nil, fmt.Errorf("未取到五子棋状态（可能未登录）")
	}
	seg = unescapeRSC(seg)
	var s hdblueSummary
	if err := json.Unmarshal(seg, &s); err != nil {
		return nil, err
	}
	return &StatsResult{
		Balance:     s.Points,
		TodaySpins:  s.FreeTotal - s.FreeRemaining,
		MaxDaily:    s.FreeTotal + s.PaidTotal,
		CostPerPlay: 10, // 付费局固定 10 影币
	}, nil
}

// hdblueExtractJSON 从 RSC 文本里抠出以 key 起始的完整 JSON 对象（花括号配平）。
// key 支持裸形态（"key":{ / "key":{"sub"）与 RSC 转义形态（\"key\":…）：转义形态先
// 还原外层转义再配平，否则 \" 被当转义符吞掉、字符串永不闭合，字符串内容里的
// 花括号会破坏配平，导致提取失败或截断。
func hdblueExtractJSON(body, key string) []byte {
	i := strings.Index(body, key)
	if i < 0 {
		return nil
	}
	k := strings.IndexByte(body[i:], '{') // 对象起始 '{'：在 key 尾部（"key":{）或 key 中段（"key":{"sub"）
	if k < 0 {
		return nil
	}
	seg := []byte(body[i+k:])
	if strings.HasPrefix(key, `\"`) {
		seg = unescapeRSC(seg)
	}
	depth := 0
	inStr := false
	esc := false
	for j := 0; j < len(seg); j++ {
		c := seg[j]
		if esc {
			esc = false
			continue
		}
		switch c {
		case '\\':
			esc = true
		case '"':
			inStr = !inStr
		case '{':
			if !inStr {
				depth++
			}
		case '}':
			if !inStr {
				depth--
				if depth == 0 {
					return seg[:j+1]
				}
			}
		}
	}
	return nil
}

// unescapeRSC 把 RSC flight 流里转义的 JSON 段（\"key\":value）还原成裸 JSON。
// 顺序：先把 \\ 成对换为占位、再 \" → "，最后占位还原为单个 \。对裸段（无反斜杠）原样返回。
func unescapeRSC(b []byte) []byte {
	const bs = "\\"
	s := string(b)
	if !strings.Contains(s, bs) {
		return b
	}
	const placeholder = "\x00"
	r := strings.NewReplacer(
		bs+bs, placeholder,
		bs+`"`, `"`,
		placeholder, bs,
	)
	return []byte(r.Replace(s))
}
