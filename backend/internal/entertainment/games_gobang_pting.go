package entertainment

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 蜂巢（pting.club）五子棋人机挑战 ─────────────────────────
// GET /api/gobang → {code,message,data:{matches,summary,leaderboards}}（全景快照）
// POST /api/gobang {action,...} → {code,message,data:{...}}（动作结果）：
//
//	create → data:{matches,summary,policy,aiPending,warning}（新局在 matches 内，
//	          message「已创建WARMUP挑战，首次有效落子时扣门票」）；
//	move   → data:{match,winnerId,aiPending,warning,timedOut,protected}；
//	undo/forfeit/retry-ai → 同 move（match 为动作后最新局）。
//	动作过于频繁 → {code:429,message:"落子过于频繁，请稍后再试"}（站点限速 ~1s/手）。
//
// 动作：create{difficulty} / choose-color{matchId,color} / move{matchId,x,y} /
// undo{matchId} / forfeit{matchId} / retry-ai{matchId}。
// 契约经真实联调校准（2026-09，登录态实测 create/move/GET 全景）；
// 站点键为 camelCase，解析用 strField/numField 转换，本文件结构体（snake_case）即对外 API 契约。
// 鉴权：flarum_session cookie（同 Flarum 站点适配器）。未登录 → {code:401,message:"请先登录"}。
type PtingGobangGame struct{}

func (PtingGobangGame) ID() string   { return "pting_gobang" }
func (PtingGobangGame) Name() string { return "五子棋" }

// GobangMove 一步落子（站点原始 moves 项：{id,step,x,y,playerId,createdAt}，
// playerId=用户 id（0=AI）；此处保留 step/playerId 便于前端复盘与最后一手判断）。
type GobangMove struct {
	X        int `json:"x"`
	Y        int `json:"y"`
	Step     int `json:"step,omitempty"`
	PlayerID int `json:"player_id,omitempty"` // 0=AI；否则为用户 id（=winnerId 语义）
}

// GobangMatch 一局对局（ONGOING 进行中 / FINISHED 已结束）。真实联调补充字段：
// firstHand（PLAYER/AI）、rule（FREESTYLE）、rankScore、challengeMode。
type GobangMatch struct {
	ID                     string       `json:"id"`
	Status                 string       `json:"status"` // ONGOING | FINISHED
	Difficulty             string       `json:"difficulty"`
	Board                  [][]int      `json:"board"` // 15x15：0 空 / 1 黑 / 2 白
	Moves                  []GobangMove `json:"moves,omitempty"`
	PlayerColor            string       `json:"player_color,omitempty"`
	CurrentSide            string       `json:"current_side,omitempty"` // PLAYER | AI；结束局为空
	FirstHand              string       `json:"first_hand,omitempty"`   // PLAYER | AI
	Rule                   string       `json:"rule,omitempty"`         // FREESTYLE（无禁手长连有效）
	OpeningChoicePending   bool         `json:"opening_choice_pending,omitempty"`
	OpeningChoiceDeadline  string       `json:"opening_choice_deadline_at,omitempty"`
	OpeningMoveCount       int          `json:"opening_move_count,omitempty"`
	TicketCharged          bool         `json:"ticket_charged,omitempty"`
	TicketCost             float64      `json:"ticket_cost,omitempty"`
	WinReward              float64      `json:"win_reward,omitempty"`
	UndoCost               float64      `json:"undo_cost,omitempty"`
	UndoUsed               bool         `json:"undo_used,omitempty"`
	Ranked                 bool         `json:"ranked,omitempty"`
	RankScore              int          `json:"rank_score,omitempty"`
	WinnerID               *int         `json:"winner_id,omitempty"`     // 0=AI 胜；null=平局；缺省进行中
	FinishReason           string       `json:"finish_reason,omitempty"` // FIVE | TIMEOUT | FORFEIT
	PayoutAmount           float64      `json:"payout_amount,omitempty"`
	InterruptionVoidedAt   string       `json:"interruption_voided_at,omitempty"`
	MoveDeadlineAt         string       `json:"move_deadline_at,omitempty"`  // 噩梦每手限时
	TotalDeadlineAt        string       `json:"total_deadline_at,omitempty"` // 噩梦整局限时
	TurnToken              string       `json:"turn_token,omitempty"`        // 计榜局落子凭证：ranked 时先 select{x,y,turnToken} 换 moveToken 再 move
	CreditSettlementEnable bool         `json:"credit_settlement_enabled,omitempty"`
	FinishedAt             string       `json:"finished_at,omitempty"`
	UpdatedAt              string       `json:"updated_at,omitempty"`
}

// GobangSummary 蜂蜜余额与挑战次数汇总（真实联调：warmup 外另有 free*/paid* 细分计数）。
type GobangSummary struct {
	Points             int     `json:"points"`
	PointName          string  `json:"point_name"`
	LegacyPointName    string  `json:"legacy_point_name,omitempty"`
	TotalLimit         int     `json:"total_limit"`
	TotalUsed          int     `json:"total_used"`
	TotalRemaining     int     `json:"total_remaining"`
	WarmupRemaining    int     `json:"warmup_remaining"`
	NightmareRemaining int     `json:"nightmare_remaining,omitempty"`
	FreeTotal          int     `json:"free_total,omitempty"`
	FreeUsed           int     `json:"free_used,omitempty"`
	FreeRemaining      int     `json:"free_remaining,omitempty"`
	PaidTotal          int     `json:"paid_total,omitempty"`
	PaidUsed           int     `json:"paid_used,omitempty"`
	PaidRemaining      int     `json:"paid_remaining,omitempty"`
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
		FirstHand:             strField(m, "firstHand"),
		Rule:                  strField(m, "rule"),
		OpeningChoicePending:  boolField(m, "openingChoicePending"),
		OpeningChoiceDeadline: strField(m, "openingChoiceDeadlineAt"),
		OpeningMoveCount:      int(longptNum(m["openingMoveCount"])),
		TicketCharged:         boolField(m, "ticketCharged"),
		TicketCost:            numField(m, "ticketCost"),
		WinReward:             numField(m, "winReward"),
		UndoCost:              numField(m, "undoCost"),
		UndoUsed:              boolField(m, "undoUsed"),
		Ranked:                boolField(m, "ranked"),
		TurnToken:             strField(m, "turnToken"),
		RankScore:             int(longptNum(m["rankScore"])),
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
			g.Moves = append(g.Moves, GobangMove{
				X:        int(longptNum(mm["x"])),
				Y:        int(longptNum(mm["y"])),
				Step:     int(longptNum(mm["step"])),
				PlayerID: int(longptNum(mm["playerId"])),
			})
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
		FreeTotal:          int(numField(m, "freeTotal")),
		FreeUsed:           int(numField(m, "freeUsed")),
		FreeRemaining:      int(numField(m, "freeRemaining")),
		PaidTotal:          int(numField(m, "paidTotal")),
		PaidUsed:           int(numField(m, "paidUsed")),
		PaidRemaining:      int(numField(m, "paidRemaining")),
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
