package entertainment

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 13City（13city.org）大转盘 + 幸运骰子（货币=啤酒瓶） ────────────────
// 契约（2026-09 实测页面 JS js/wheel.js / js/beerpoker.js）：
//
//	POST wheel.php    action=get_balance|get_prizes|get_daily_remaining|spin|get_latest_history
//	POST beerpoker.php action=get_balance|get_history|place_bet&bet=N&betType=T|get_rankings
//
// 响应 {success,...}；spin 返回 target_angle（转盘动画）与奖品信息；
// place_bet 返回骰子结果与 newBalance。转盘消耗啤酒瓶；骰子 betType=大小/点数类。
type City13WheelGame struct{}

func (City13WheelGame) ID() string   { return "13city_wheel" }
func (City13WheelGame) Name() string { return "大转盘" }

// City13DiceGame 幸运骰子（同站第二个游戏）。
type City13DiceGame struct{}

func (City13DiceGame) ID() string   { return "13city_dice" }
func (City13DiceGame) Name() string { return "幸运骰子" }

// city13PostCred 带 cookie 的 POST。
func city13PostCred(ctx context.Context, hc *httpclient.Client, baseURL, page, action, cookie string, extra url.Values) (map[string]any, error) {
	u := joinPath(baseURL, page)
	form := url.Values{}
	form.Set("action", action)
	for k, vs := range extra {
		for _, v := range vs {
			form.Set(k, v)
		}
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form.Encode()), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          u,
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("13city_"+action, u, resp)
	return parseRaw(resp.Body)
}

// Play 转盘：action=spin（花啤酒瓶，chain spendGuard 防双击）。
func (City13WheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := city13PostCred(ctx, hc, baseURL, "wheel.php", "spin", cr.Cookie, nil)
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常"}, nil
	}
	res := &PlayResult{Success: boolField(m, "success"), Raw: m}
	res.Message = firstNonEmpty(strField(m, "message"), strField(m, "msg"))
	if res.Success {
		res.Prize = firstNonEmpty(strField(m, "prize_name"), strField(m, "prize_text"), strField(m, "prizeName"))
		res.Amount = numField(m, "prize_amount")
		if res.Prize == "" {
			res.Prize = "中奖"
		}
	}
	if !res.Success && res.Message == "" {
		res.Message = "抽奖失败（啤酒瓶不足或达到今日上限）"
	}
	return res, nil
}

// Stats 转盘余额 + 每日剩余（get_balance / get_daily_remaining）。
func (City13WheelGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	m, _ := city13PostCred(ctx, hc, baseURL, "wheel.php", "get_balance", cr.Cookie, nil)
	st := &StatsResult{}
	if m != nil {
		st.Balance = numField(m, "balance")
	}
	if d, _ := city13PostCred(ctx, hc, baseURL, "wheel.php", "get_daily_remaining", cr.Cookie, nil); d != nil {
		st.TodaySpins = int(numField(d, "used"))
		st.MaxDaily = int(numField(d, "limit"))
	}
	return st, nil
}

// Prizes 转盘奖品列表（get_prizes）。
func (City13WheelGame) Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error) {
	m, err := city13PostCred(ctx, hc, baseURL, "wheel.php", "get_prizes", cr.Cookie, nil)
	if err != nil || m == nil {
		return nil, err
	}
	out := make([]PrizeDef, 0)
	if arr, ok := m["prizes"].([]any); ok {
		for i, it := range arr {
			p, ok := it.(map[string]any)
			if !ok {
				continue
			}
			name := firstNonEmpty(strField(p, "name"), strField(p, "prize_name"), strField(p, "text"))
			out = append(out, PrizeDef{ID: i + 1, Name: name})
		}
	}
	return out, nil
}

// Play 骰子：默认下注走 Bet；Play=查余额（place_bet 花啤酒瓶，不自动）。
func (City13DiceGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := city13PostCred(ctx, hc, baseURL, "beerpoker.php", "get_balance", cr.Cookie, nil)
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常"}, nil
	}
	bal := numField(m, "balance")
	return &PlayResult{Success: true, Balance: bal, Message: fmt.Sprintf("当前啤酒瓶余额 %g（下注请用 Bet）", bal)}, nil
}

// Bet 骰子下注：betType=big/small/点数（站点 JS 契约 place_bet&bet=N&betType=T）。
func (City13DiceGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	if req.Amount <= 0 {
		return &PlayResult{Success: false, Message: "请输入下注金额（啤酒瓶）"}, nil
	}
	if req.Selection == "" {
		return &PlayResult{Success: false, Message: "请选择 betType（big/small/点数）"}, nil
	}
	m, err := city13PostCred(ctx, hc, baseURL, "beerpoker.php", "place_bet", cr.Cookie, url.Values{
		"bet":     {fmt.Sprintf("%g", req.Amount)},
		"betType": {req.Selection},
	})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常"}, nil
	}
	res := &PlayResult{Success: boolField(m, "success"), Raw: m,
		Message: firstNonEmpty(strField(m, "message"), strField(m, "msg"))}
	res.Balance = numField(m, "newBalance")
	if dice, ok := m["dice"].([]any); ok {
		var parts []string
		for _, d := range dice {
			parts = append(parts, fmt.Sprint(longptNum(d)))
		}
		res.Prize = "骰子: " + strings.Join(parts, " ")
	}
	if !res.Success && res.Message == "" {
		res.Message = "下注失败"
	}
	return res, nil
}

// Stats 骰子余额。
func (City13DiceGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	m, _ := city13PostCred(ctx, hc, baseURL, "beerpoker.php", "get_balance", cr.Cookie, nil)
	return &StatsResult{Balance: numField(m, "balance")}, nil
}

// ── 批12 富 UI 状态端点 ─────────────────────────────────────────────

// City13WheelState 转盘全景（余额/今日次数/奖品环/最近开奖），前端转盘动画渲染用。
type City13WheelState struct {
	Balance      float64    `json:"balance"`
	TodayUsed    int        `json:"today_used"`
	DailyLimit   int        `json:"daily_limit"`
	Prizes       []PrizeDef `json:"prizes"`
	RecentPrizes []string   `json:"recent_prizes,omitempty"` // 最近几期结果（站点字段尽力取）
}

// City13Wheelable 转盘全景能力。
type City13Wheelable interface {
	City13WheelState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*City13WheelState, error)
}

// City13WheelState 实现：get_balance + get_daily_remaining + get_prizes + get_latest_history。
func (g City13WheelGame) City13WheelState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*City13WheelState, error) {
	st := &City13WheelState{Prizes: []PrizeDef{}}
	if m, _ := city13PostCred(ctx, hc, baseURL, "wheel.php", "get_balance", cr.Cookie, nil); m != nil {
		st.Balance = numField(m, "balance")
	}
	if d, _ := city13PostCred(ctx, hc, baseURL, "wheel.php", "get_daily_remaining", cr.Cookie, nil); d != nil {
		st.TodayUsed = int(numField(d, "used"))
		st.DailyLimit = int(numField(d, "limit"))
	}
	if p, _ := g.Prizes(ctx, hc, baseURL, cr); p != nil {
		st.Prizes = p
	}
	if h, _ := city13PostCred(ctx, hc, baseURL, "wheel.php", "get_latest_history", cr.Cookie, nil); h != nil {
		if arr, ok := h["history"].([]any); ok {
			for i, it := range arr {
				if i >= 8 {
					break
				}
				if hm, ok := it.(map[string]any); ok {
					st.RecentPrizes = append(st.RecentPrizes,
						firstNonEmpty(strField(hm, "prize_name"), strField(hm, "prize_text"), strField(hm, "prize")))
				}
			}
		}
	}
	return st, nil
}

// City13DiceState 骰子全景（余额/玩法提示/最近开奖记录）。
type City13DiceState struct {
	Balance float64            `json:"balance"`
	Types   []City13DiceType   `json:"types"` // 玩法与赔率（静态约定，站点 JS 同源）
	Recent  []City13DiceRecord `json:"recent,omitempty"`
}

// City13DiceType 玩法定义。
type City13DiceType struct {
	Key    string  `json:"key"` // big/small/2..12
	Label  string  `json:"label"`
	Payout float64 `json:"payout"` // 赔率
}

// City13DiceRecord 一期开奖（dice 三骰 + 大小）。
type City13DiceRecord struct {
	Dice []int `json:"dice"`
	Sum  int   `json:"sum"`
	Big  bool  `json:"big"`
}

// City13Diceable 骰子全景能力。
type City13Diceable interface {
	City13DiceState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*City13DiceState, error)
}

// city13DiceTypes 玩法表（big/small 赔 1:1；点数按概率递增——与站点规则页一致，尽力静态）。
var city13DiceTypes = []City13DiceType{
	{Key: "big", Label: "大 (11-17)", Payout: 1},
	{Key: "small", Label: "小 (4-10)", Payout: 1},
}

// City13DiceState 实现：get_balance + get_history。
func (g City13DiceGame) City13DiceState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*City13DiceState, error) {
	st := &City13DiceState{Types: city13DiceTypes}
	if m, _ := city13PostCred(ctx, hc, baseURL, "beerpoker.php", "get_balance", cr.Cookie, nil); m != nil {
		st.Balance = numField(m, "balance")
	}
	if h, _ := city13PostCred(ctx, hc, baseURL, "beerpoker.php", "get_history", cr.Cookie, nil); h != nil {
		if arr, ok := h["history"].([]any); ok {
			for i, it := range arr {
				if i >= 10 {
					break
				}
				hm, ok := it.(map[string]any)
				if !ok {
					continue
				}
				rec := City13DiceRecord{}
				if dice, ok := hm["dice"].([]any); ok {
					for _, d := range dice {
						rec.Dice = append(rec.Dice, int(longptNum(d)))
						rec.Sum += int(longptNum(d))
					}
				}
				if s := longptNum(hm["sum"]); s > 0 {
					rec.Sum = int(s)
				}
				rec.Big = rec.Sum >= 11
				st.Recent = append(st.Recent, rec)
			}
		}
	}
	return st, nil
}
