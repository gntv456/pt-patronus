package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 蜂巢五子棋·动作层：POST /api/gobang {action,...} 的请求与动作结果映射。
// 动作：create{difficulty} / choose-color{matchId,color} / move{matchId,x,y} /
// undo{matchId} / forfeit{matchId} / retry-ai{matchId}。
// 站点限速 ~1s/手：过快 → 站点返回 code 429「落子过于频繁」，此处转为失败结果透传 message。

// ptingGobangPost 执行动作请求。返回 message + 动作后最新局 + summary。
// data 里 match（单局）与 matches（全景数组）都可能出——create 返回新局在 matches 内，
// move/undo/forfeit/retry-ai 返回 match；两处都解析，matches 取优先进行中局。
func ptingGobangPost(ctx context.Context, hc *httpclient.Client, baseURL, cookie, action string, params map[string]any) (string, *GobangMatch, *GobangSummary, error) {
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
		return "", nil, nil, err
	}
	logResp("pting_gobang "+action, u, resp)
	var env ptingGobangEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return "", nil, nil, fmt.Errorf("响应解析失败: %w", err)
	}
	if env.Code != 0 && env.Code != 200 {
		return "", nil, nil, fmt.Errorf("%s", firstNonEmpty(env.Message, "操作失败"))
	}
	var match *GobangMatch
	var summary *GobangSummary
	var data map[string]any
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &data); err == nil {
			if m, ok := data["match"].(map[string]any); ok {
				match = parseGobangMatch(m)
			}
			if arr, ok := data["matches"].([]any); ok && match == nil {
				for _, it := range arr {
					if m, ok := it.(map[string]any); ok {
						mm := parseGobangMatch(m)
						// 优先进行中局；全结束则取第一局（刚结束的）
						if match == nil || (mm.Status == "ONGOING" && match.Status != "ONGOING") {
							match = mm
						}
					}
				}
			}
			if s, ok := data["summary"].(map[string]any); ok {
				summary = parseGobangSummary(s)
			}
		}
	}
	return firstNonEmpty(env.Message, "操作完成"), match, summary, nil
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
	msg, m, _, err := ptingGobangPost(ctx, hc, baseURL, cr.Cookie, action, params)
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
