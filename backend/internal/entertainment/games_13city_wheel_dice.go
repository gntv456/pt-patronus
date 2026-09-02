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
