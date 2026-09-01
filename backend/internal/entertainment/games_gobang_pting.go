package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 蜂巢（pting.club）五子棋人机挑战 ─────────────────────────
// GET /api/gobang → {code,message,data:{matches,summary,leaderboards}}（全景快照）
// POST /api/gobang {action,...} → {code,message,data:{match?,summary?}}（动作结果，随后客户端重拉全景）
// 动作：create{difficulty} / choose-color{matchId,color} / move{matchId,x,y} /
//
//	undo{matchId} / forfeit{matchId} / retry-ai{matchId}。
//
// 契约逆自 pting.club/funs/gobang 前端 chunk（04_q3pnpwr6at.js）；站点键为 camelCase，
// 解析用 strField/numField 从 map 转换，本文件结构体（snake_case）即对外 API 契约。
// 鉴权：flarum_session cookie（同 Flarum 站点适配器）。未登录 → {code:401,message:"请先登录"}。
type PtingGobangGame struct{}

func (PtingGobangGame) ID() string   { return "pting_gobang" }
func (PtingGobangGame) Name() string { return "五子棋" }

// GobangMove 一步落子（x=列 0..14，y=行 0..14）。
type GobangMove struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// GobangMatch 一局对局（ONGOING 进行中 / FINISHED 已结束）。
type GobangMatch struct {
	ID                     string       `json:"id"`
	Status                 string       `json:"status"` // ONGOING | FINISHED
	Difficulty             string       `json:"difficulty"`
	Board                  [][]int      `json:"board"` // 15x15：0 空 / 1 黑 / 2 白
	Moves                  []GobangMove `json:"moves,omitempty"`
	PlayerColor            string       `json:"player_color,omitempty"`
	CurrentSide            string       `json:"current_side,omitempty"` // PLAYER | AI
	OpeningChoicePending   bool         `json:"opening_choice_pending,omitempty"`
	OpeningChoiceDeadline  string       `json:"opening_choice_deadline_at,omitempty"`
	OpeningMoveCount       int          `json:"opening_move_count,omitempty"`
	TicketCharged          bool         `json:"ticket_charged,omitempty"`
	TicketCost             float64      `json:"ticket_cost,omitempty"`
	WinReward              float64      `json:"win_reward,omitempty"`
	UndoCost               float64      `json:"undo_cost,omitempty"`
	UndoUsed               bool         `json:"undo_used,omitempty"`
	Ranked                 bool         `json:"ranked,omitempty"`
	WinnerID               *int         `json:"winner_id,omitempty"` // 0=AI 胜；null=平局；缺省进行中
	FinishReason           string       `json:"finish_reason,omitempty"`
	PayoutAmount           float64      `json:"payout_amount,omitempty"`
	InterruptionVoidedAt   string       `json:"interruption_voided_at,omitempty"`
	MoveDeadlineAt         string       `json:"move_deadline_at,omitempty"`  // 噩梦每手限时
	TotalDeadlineAt        string       `json:"total_deadline_at,omitempty"` // 噩梦整局限时
	CreditSettlementEnable bool         `json:"credit_settlement_enabled,omitempty"`
	FinishedAt             string       `json:"finished_at,omitempty"`
	UpdatedAt              string       `json:"updated_at,omitempty"`
}

// GobangSummary 蜂蜜余额与挑战次数汇总。
type GobangSummary struct {
	Points             int     `json:"points"`
	PointName          string  `json:"point_name"`
	LegacyPointName    string  `json:"legacy_point_name,omitempty"`
	TotalLimit         int     `json:"total_limit"`
	TotalUsed          int     `json:"total_used"`
	TotalRemaining     int     `json:"total_remaining"`
	WarmupRemaining    int     `json:"warmup_remaining"`
	NightmareRemaining int     `json:"nightmare_remaining,omitempty"`
	DailyNetGain       float64 `json:"daily_net_gain"`
	DailyNetGainCap    float64 `json:"daily_net_gain_cap"`
}

// GobangRankEntry 周榜一行。
type GobangRankEntry struct {
	UserID                 int    `json:"user_id"`
	Rank                   int    `json:"rank"`
	Username               string `json:"username"`
	Nickname               string `json:"nickname"`
	AvatarPath             string `json:"avatar_path,omitempty"`
	Games                  int    `json:"games"`
	Wins                   int    `json:"wins"`
	Losses                 int    `json:"losses"`
	WinRate                int    `json:"win_rate"`
	AverageDurationSeconds int    `json:"average_duration_seconds"`
	AverageMoves           int    `json:"average_moves"`
	PlayerFirstGames       int    `json:"player_first_games"`
	AIFirstGames           int    `json:"ai_first_games"`
	Score                  int    `json:"score"`
}

// GobangState 全景快照（GET /api/gobang data 段）。
type GobangState struct {
	Matches      []GobangMatch                `json:"matches"`
	Summary      *GobangSummary               `json:"summary,omitempty"`
	Leaderboards map[string][]GobangRankEntry `json:"leaderboards,omitempty"`
}

// ptingGobangEnvelope 站点响应外壳：错误时 code!=0 + message。
type ptingGobangEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// parseGobangMatch camelCase map → GobangMatch。
func parseGobangMatch(m map[string]any) *GobangMatch {
	g := &GobangMatch{
		ID:                    strField(m, "id"),
		Status:                strField(m, "status"),
		Difficulty:            strField(m, "difficulty"),
		PlayerColor:           strField(m, "playerColor"),
		CurrentSide:           strField(m, "currentSide"),
		OpeningChoicePending:  boolField(m, "openingChoicePending"),
		OpeningChoiceDeadline: firstNonEmpty(strField(m, "openingChoiceDeadlineAt")),
		OpeningMoveCount:      int(longptNum(m["openingMoveCount"])),
		TicketCharged:         boolField(m, "ticketCharged"),
		TicketCost:            numField(m, "ticketCost"),
		WinReward:             numField(m, "winReward"),
		UndoCost:              numField(m, "undoCost"),
		UndoUsed:              boolField(m, "undoUsed"),
		Ranked:                boolField(m, "ranked"),
		FinishReason:          strField(m, "finishReason"),
		PayoutAmount:          numField(m, "payoutAmount"),
		InterruptionVoidedAt:  strField(m, "interruptionVoidedAt"),
		MoveDeadlineAt:        strField(m, "moveDeadlineAt"),
		TotalDeadlineAt:       strField(m, "totalDeadlineAt"),
		FinishedAt:            strField(m, "finishedAt"),
		UpdatedAt:             strField(m, "updatedAt"),
	}
	g.CreditSettlementEnable = boolField(m, "creditSettlementEnabled")
	if rows, ok := m["board"].([]any); ok {
		g.Board = make([][]int, 0, len(rows))
		for _, row := range rows {
			cells, _ := row.([]any)
			line := make([]int, 0, len(cells))
			for _, c := range cells {
				line = append(line, int(longptNum(c)))
			}
			g.Board = append(g.Board, line)
		}
	}
	if mv, ok := m["moves"].([]any); ok {
		g.Moves = make([]GobangMove, 0, len(mv))
		for _, it := range mv {
			mm, ok := it.(map[string]any)
			if !ok {
				continue
			}
			g.Moves = append(g.Moves, GobangMove{X: int(longptNum(mm["x"])), Y: int(longptNum(mm["y"]))})
		}
	}
	// winnerId：缺省（进行中）保持 nil；null（平局）与非 0（用户 id）都要区分。
	if w, ok := m["winnerId"]; ok && w != nil {
		id := int(longptNum(w))
		g.WinnerID = &id
	}
	return g
}

// parseGobangSummary camelCase map → GobangSummary。
func parseGobangSummary(m map[string]any) *GobangSummary {
	return &GobangSummary{
		Points:             int(numField(m, "points")),
		PointName:          firstNonEmpty(strField(m, "pointName"), "蜂蜜"),
		LegacyPointName:    strField(m, "legacyPointName"),
		TotalLimit:         int(numField(m, "totalLimit")),
		TotalUsed:          int(numField(m, "totalUsed")),
		TotalRemaining:     int(numField(m, "totalRemaining")),
		WarmupRemaining:    int(numField(m, "warmupRemaining")),
		NightmareRemaining: int(numField(m, "nightmareRemaining")),
		DailyNetGain:       numField(m, "dailyNetGain"),
		DailyNetGainCap:    numField(m, "dailyNetGainCap"),
	}
}

// parseGobangRankEntry camelCase map → GobangRankEntry。
func parseGobangRankEntry(m map[string]any) GobangRankEntry {
	return GobangRankEntry{
		UserID:                 int(longptNum(m["userId"])),
		Rank:                   int(longptNum(m["rank"])),
		Username:               strField(m, "username"),
		Nickname:               strField(m, "nickname"),
		AvatarPath:             strField(m, "avatarPath"),
		Games:                  int(longptNum(m["games"])),
		Wins:                   int(longptNum(m["wins"])),
		Losses:                 int(longptNum(m["losses"])),
		WinRate:                int(longptNum(m["winRate"])),
		AverageDurationSeconds: int(longptNum(m["averageDurationSeconds"])),
		AverageMoves:           int(longptNum(m["averageMoves"])),
		PlayerFirstGames:       int(longptNum(m["playerFirstGames"])),
		AIFirstGames:           int(longptNum(m["aiFirstGames"])),
		Score:                  int(longptNum(m["score"])),
	}
}

// ptingGobangData 把站点 data 段（{matches,summary,leaderboards}）转 GobangState。
func ptingGobangData(env *ptingGobangEnvelope) (*GobangState, error) {
	state := &GobangState{}
	var data map[string]any
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &data); err != nil || data == nil {
			return state, nil
		}
	}
	if arr, ok := data["matches"].([]any); ok {
		state.Matches = make([]GobangMatch, 0, len(arr))
		for _, it := range arr {
			if m, ok := it.(map[string]any); ok {
				state.Matches = append(state.Matches, *parseGobangMatch(m))
			}
		}
	}
	if s, ok := data["summary"].(map[string]any); ok {
		state.Summary = parseGobangSummary(s)
	}
	if lb, ok := data["leaderboards"].(map[string]any); ok {
		state.Leaderboards = map[string][]GobangRankEntry{}
		for diff, v := range lb {
			arr, _ := v.([]any)
			entries := make([]GobangRankEntry, 0, len(arr))
			for _, it := range arr {
				if m, ok := it.(map[string]any); ok {
					entries = append(entries, parseGobangRankEntry(m))
				}
			}
			state.Leaderboards[diff] = entries
		}
	}
	return state, nil
}

// ptingGobangGet 调 GET /api/gobang，返回解析后的全景。
func ptingGobangGet(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (*GobangState, error) {
	u := joinPath(baseURL, "api/gobang")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{
		"Accept":  "application/json",
		"Referer": joinPath(baseURL, "funs/gobang"),
	}, true)
	if err != nil {
		return nil, err
	}
	logResp("pting_gobang state", u, resp)
	var env ptingGobangEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("响应解析失败: %w", err)
	}
	if env.Code != 0 && env.Code != 200 {
		return nil, fmt.Errorf("%s", firstNonEmpty(env.Message, "请求失败"))
	}
	return ptingGobangData(&env)
}

// ptingGobangPost 调 POST /api/gobang（JSON body：action + 参数）。
// 返回 message 与动作后的对局（data.match；summary 字段站点按动作可选返回，此处不解析——前端动作后重拉全景）。
func ptingGobangPost(ctx context.Context, hc *httpclient.Client, baseURL, cookie, action string, params map[string]any) (string, *GobangMatch, error) {
	u := joinPath(baseURL, "api/gobang")
	body := map[string]any{"action": action}
	for k, v := range params {
		body[k] = v
	}
	raw, _ := json.Marshal(body)
	resp, err := hc.Do(ctx, http.MethodPost, u, raw, map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
		"Referer":      joinPath(baseURL, "funs/gobang"),
	}, cookie, true)
	if err != nil {
		return "", nil, err
	}
	logResp("pting_gobang "+action, u, resp)
	var env ptingGobangEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return "", nil, fmt.Errorf("响应解析失败: %w", err)
	}
	if env.Code != 0 && env.Code != 200 {
		return "", nil, fmt.Errorf("%s", firstNonEmpty(env.Message, "操作失败"))
	}
	var match *GobangMatch
	var data map[string]any
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &data); err == nil {
			if m, ok := data["match"].(map[string]any); ok {
				match = parseGobangMatch(m)
			}
		}
	}
	return firstNonEmpty(env.Message, "操作完成"), match, nil
}

// Gobangable 五子棋能力：全景 + 动作。chain/API 按此接口分发。
type Gobangable interface {
	GobangState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*GobangState, error)
	GobangAction(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string, params map[string]any) (*PlayResult, error)
}

// GobangState 拉全景。
func (PtingGobangGame) GobangState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*GobangState, error) {
	return ptingGobangGet(ctx, hc, baseURL, cr.Cookie)
}

// GobangAction 执行动作。合法动作白名单校验后透传参数；「花钱」动作
// （create 首落子扣门票/move/undo）由 chain 层 spendGuard 防双击。
func (PtingGobangGame) GobangAction(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string, params map[string]any) (*PlayResult, error) {
	switch action {
	case "create", "choose-color", "move", "undo", "forfeit", "retry-ai":
	default:
		return &PlayResult{Success: false, Message: "未知动作: " + action}, nil
	}
	msg, m, err := ptingGobangPost(ctx, hc, baseURL, cr.Cookie, action, params)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: true, Message: msg}
	if m != nil {
		// 结束局带上胜负与返奖；进行中带上轮次提示（PLAYER=轮到玩家）。
		if m.Status == "FINISHED" {
			switch {
			case m.WinnerID != nil && *m.WinnerID != 0:
				res.Prize = "胜利"
				res.Amount = m.PayoutAmount
			case m.WinnerID == nil:
				res.Prize = "平局"
			default:
				res.Prize = firstNonEmpty(m.FinishReason, "AI 获胜")
			}
		} else if m.CurrentSide == "PLAYER" {
			res.Prize = "轮到你落子"
		}
	}
	return res, nil
}

// Play（LotteryGame 兜底）= 开一局热身（免费难度，不花钱）。
func (PtingGobangGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return PtingGobangGame{}.GobangAction(ctx, hc, baseURL, cr, "create", map[string]any{"difficulty": "WARMUP"})
}

// Stats 蜂蜜余额 + 今日次数（全景 summary 字段映射）。
func (PtingGobangGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	state, err := ptingGobangGet(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{}
	if s := state.Summary; s != nil {
		st.Balance = float64(s.Points)
		st.TodaySpins = s.TotalUsed
		st.MaxDaily = s.TotalLimit
	}
	return st, nil
}
