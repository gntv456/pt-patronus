// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 多站幸运转盘/抽奖（纯 Cookie）：UB(wof.php) / LuckyDraw 插件(PANDA+蟹黄堡) / March(/lucky-wheel) / 不可躺(omnibot lottery)。

// ── UB 幸运大转盘（ubits.club） ────────────────────────────
// GET wof.php?app=lottery_json&count=N&csrf_token=T → {rid,rname}（rid=0 谢谢参与）。
// csrf 从 <meta name="csrf-token">；余额抠整页「U币」。
type UbWheelGame struct{}

func (UbWheelGame) ID() string   { return "ub_wheel" }
func (UbWheelGame) Name() string { return "幸运大转盘" }

func (UbWheelGame) draw(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	page := joinPath(baseURL, "wof.php")
	pr, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	logResp("ub_wheel page", page, pr)
	csrf := firstGroup(pr.Body, `(?s)name="csrf-token"\s+content="([^"]+)"`)
	balance := novaHDNum(firstGroup(pr.Body, `U币[^0-9-]*([\d,.]+)`))

	u := page + "?app=lottery_json"
	if count > 1 {
		u += "&count=" + strconv.Itoa(count)
	}
	if csrf != "" {
		u += "&csrf_token=" + csrf
	}
	resp, err := hc.Do(ctx, http.MethodGet, u, nil, map[string]string{"Referer": page, "X-Requested-With": "XMLHttpRequest"}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("ub_wheel draw", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "解析抽奖结果失败"}, nil
	}
	rid := int(longptNum(m["rid"]))
	rname := strField(m, "rname")
	res := &PlayResult{Success: true, PrizeID: rid, Prize: rname, Balance: balance, Raw: m}
	if rid == 0 || strings.Contains(rname, "谢谢") || strings.Contains(rname, "参与") {
		res.Prize, res.Message = "未中奖", "很遗憾，未中奖"
	} else {
		res.Message = "中奖：" + rname
	}
	return res, nil
}

func (UbWheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return UbWheelGame{}.draw(ctx, hc, baseURL, cr, 1)
}

func (UbWheelGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	if count > maxMultiCount {
		count = maxMultiCount
	}
	return UbWheelGame{}.draw(ctx, hc, baseURL, cr, count)
}

func (UbWheelGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "wof.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: novaHDNum(firstGroup(resp.Body, `U币[^0-9-]*([\d,.]+)`))}, nil
}

// ── March 幸运大转盘（duckboobee.org） ────────────────────
// POST /lucky-wheel/spin → {success, results:[{...}], remaining_bonus, message}。
type MarchWheelGame struct{}

func (MarchWheelGame) ID() string   { return "march_wheel" }
func (MarchWheelGame) Name() string { return "幸运大转盘" }

func (MarchWheelGame) spin(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	u := joinPath(baseURL, "lucky-wheel/spin")
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte("{}"), map[string]string{
		"Content-Type":     "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "luckywheel.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("march spin", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	res := &PlayResult{Success: boolField(m, "success"), Balance: numField(m, "remaining_bonus"), Message: strField(m, "message"), Raw: m}
	if results, ok := m["results"].([]any); ok {
		res.Prizes = make([]PrizeOutcome, 0, len(results))
		for _, r := range results {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			name := firstNonEmpty(strField(rm, "name"), strField(rm, "prize_name"), strField(rm, "text"))
			res.Prizes = append(res.Prizes, PrizeOutcome{PrizeName: name, Amount: numField(rm, "amount"), IsWin: name != "" && !strings.Contains(name, "谢谢")})
		}
		if len(res.Prizes) > 0 {
			res.Prize = res.Prizes[0].PrizeName
		}
	}
	if res.Success && res.Prize == "" {
		res.Prize = "中奖"
	}
	return res, nil
}

func (MarchWheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return MarchWheelGame{}.spin(ctx, hc, baseURL, cr)
}

// PlayMulti 连抽：March spin 端点单次即返回 results 数组（站点自身聚合）；直接返回其结果。
// count 上限由调用方在更上层裁剪，此处站点单次聚合故不下发。
func (MarchWheelGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	_ = count
	return MarchWheelGame{}.spin(ctx, hc, baseURL, cr)
}

// Stats 余额：抠 luckywheel.php 的 #user-bonus-amount（"魔力1000"）。
func (MarchWheelGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "luckywheel.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return &StatsResult{}, nil
	}
	return &StatsResult{Balance: novaHDNum(firstGroup(resp.Body, `id="user-bonus-amount"[^>]*>[^0-9-]*([\d,.]+)`))}, nil
}

// ── 不可躺 抽奖（www.tangpt.top） ─────────────────────────
// POST /web/omnibot/lottery/draw form {count} → {results, draw_count, user_bonus_after}。
type TangptLotteryGame struct{}

func (TangptLotteryGame) ID() string   { return "tangpt_lottery" }
func (TangptLotteryGame) Name() string { return "幸运抽奖" }

func (TangptLotteryGame) draw(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	u := joinPath(baseURL, "web/omnibot/lottery/draw")
	if count < 1 {
		count = 1
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte("count="+strconv.Itoa(count)), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "omnibot_lottery.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("tangpt draw", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	res := &PlayResult{Success: true, Balance: numField(m, "user_bonus_after"), Message: strField(m, "message"), Raw: m}
	if results, ok := m["results"].([]any); ok {
		res.Prizes = make([]PrizeOutcome, 0, len(results))
		for _, r := range results {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			name := firstNonEmpty(strField(rm, "name"), strField(rm, "prize_name"))
			o := PrizeOutcome{PrizeName: name, Amount: numField(rm, "amount"), IsWin: name != "" && !strings.Contains(name, "谢谢")}
			res.Prizes = append(res.Prizes, o)
			if o.IsWin && res.Prize == "" {
				res.Prize = name
			}
		}
	}
	return res, nil
}

func (TangptLotteryGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return TangptLotteryGame{}.draw(ctx, hc, baseURL, cr, 1)
}

func (TangptLotteryGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count > maxMultiCount {
		count = maxMultiCount
	}
	return TangptLotteryGame{}.draw(ctx, hc, baseURL, cr, count)
}

// Stats 余额：omnibot_lottery.php 是 SPA，余额多在 bootstrap JSON（user_bonus）或页面「魔力：N」。
// 无离线样本，按多个模式兜底；都没命中返回 0（前端 hero 会在首次 draw 后用 last.balance 兜底）。
func (TangptLotteryGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "omnibot_lottery.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return &StatsResult{}, nil
	}
	body := resp.Body
	for _, p := range []string{`"user_bonus"[: ]+([\d.]+)`, `userBonus[ =:]+([\d.]+)`, `当前[^0-9-]{0,6}魔力[：: ]+([\d,.]+)`} {
		if v := novaHDNum(firstGroup(body, p)); v > 0 {
			return &StatsResult{Balance: v}, nil
		}
	}
	return &StatsResult{}, nil
}

// History 抽奖记录：omnibot SPA，记录端点未在离线样本确认，按常见路径 best-effort 尝试
// （/web/omnibot/lottery/records 或 /history），失败返空（前端记录区不显示，无副作用）。
func (TangptLotteryGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	for _, path := range []string{"web/omnibot/lottery/records", "web/omnibot/lottery/history"} {
		u := joinPath(baseURL, path)
		resp, err := hc.Do(ctx, http.MethodGet, u, nil, map[string]string{
			"X-Requested-With": "XMLHttpRequest",
			"Referer":          joinPath(baseURL, "omnibot_lottery.php"),
		}, cr.Cookie, true)
		if err != nil || resp.StatusCode != 200 {
			continue
		}
		m, err := parseRaw(resp.Body)
		if err != nil {
			continue
		}
		items := recordItems(m)
		if items == nil {
			if arr, ok := m["results"].([]any); ok {
				items = make([]map[string]any, 0, len(arr))
				for _, e := range arr {
					if em, ok := e.(map[string]any); ok {
						items = append(items, em)
					}
				}
			}
		}
		if len(items) == 0 {
			continue
		}
		logResp("tangpt records", u, resp)
		out := make([]GameRecord, 0, len(items))
		for _, it := range items {
			name := firstNonEmpty(strField(it, "name"), strField(it, "prize"), strField(it, "result"))
			ts := firstNonEmpty(strField(it, "created_at"), strField(it, "time"), strField(it, "created_time"))
			if name == "" && ts == "" {
				continue
			}
			out = append(out, GameRecord{PrizeName: name, Amount: numField(it, "amount"), CreatedAt: ts})
		}
		return out, nil
	}
	return []GameRecord{}, nil
}

// TangptLotteryState 躺平抽奖全景（页面 qqbotLotteryState 直读）。
type TangptLotteryState struct {
	Enabled      bool             `json:"enabled"`
	CostPerDraw  float64          `json:"cost_per_draw"`
	DailyRemain  int              `json:"daily_remaining"`
	MaxDrawCount int              `json:"max_draw_count"`
	Items        []TangptPrizeDef `json:"items"`
}

// TangptPrizeDef 奖品（含概率与站点配色）。
type TangptPrizeDef struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Amount      float64 `json:"amount,omitempty"`
	Type        string  `json:"type,omitempty"` // bonus/upload/card…
	Probability float64 `json:"probability"`    // 百分比
	Color       string  `json:"color,omitempty"`
}

// tangptLotteryStateRe 页面 window.qqbotLotteryState = {...};
var tangptLotteryStateRe = regexp.MustCompile(`(?s)qqbotLotteryState\s*=\s*(\{.*?\})\s*;`)

// tangptLotteryStateFromPage 解析页面内嵌状态。
func tangptLotteryStateFromPage(body string) (*TangptLotteryState, error) {
	m := tangptLotteryStateRe.FindStringSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("页面无 qqbotLotteryState（未登录或改版）")
	}
	raw := m[1]
	var st TangptLotteryState
	type wire struct {
		EnabledDraw        bool    `json:"enabledDraw"`
		CostPerDraw        float64 `json:"costPerDraw"`
		DailyDrawRemaining int     `json:"dailyDrawRemaining"`
		MaxDrawCount       int     `json:"maxDrawCount"`
		Items              []struct {
			ID          int     `json:"id"`
			Name        string  `json:"name"`
			Type        string  `json:"type"`
			Amount      float64 `json:"amount"`
			Probability float64 `json:"probability"`
			Color       string  `json:"color"`
		} `json:"items"`
	}
	var w wire
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return nil, fmt.Errorf("parse qqbotLotteryState: %w", err)
	}
	st.Enabled = w.EnabledDraw
	st.CostPerDraw = w.CostPerDraw
	st.DailyRemain = w.DailyDrawRemaining
	st.MaxDrawCount = w.MaxDrawCount
	for _, it := range w.Items {
		st.Items = append(st.Items, TangptPrizeDef{
			ID: it.ID, Name: it.Name, Type: it.Type,
			Amount: it.Amount, Probability: it.Probability, Color: it.Color,
		})
	}
	return &st, nil
}

// LotteryState 全景（TangptLotteryable）。
func (g TangptLotteryGame) LotteryState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*TangptLotteryState, error) {
	u := joinPath(baseURL, "omnibot_lottery.php")
	resp, err := hc.GetWithHeaders(ctx, u, cr.Cookie, map[string]string{"Referer": u}, true)
	if err != nil {
		return nil, err
	}
	return tangptLotteryStateFromPage(string(resp.Body))
}

// TangptLotteryable 躺平抽奖全景能力。
type TangptLotteryable interface {
	LotteryState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*TangptLotteryState, error)
}
