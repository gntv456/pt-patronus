// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// CBG（cangbao.ge）娱乐：幸运大转盘 / 魔力老虎机（骰宝见批2 games_bet.go）。
// 共享风格：POST api/api_<game>.php，响应 {status:"success"|"error", new_magic_point, message, ...}；
// 余额页面 #current-magic-point；记录 api/api_<game>_log.php。纯 Cookie 鉴权。

// cbgApiPost POST api 端点（JSON body）。
func cbgApiPost(ctx context.Context, hc *httpclient.Client, baseURL, apiPath, refererPage, cookie string, body any) (map[string]any, error) {
	u := joinPath(baseURL, apiPath)
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, payload, map[string]string{
		"Content-Type":     "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, refererPage),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("cbg "+apiPath, u, resp)
	return parseRaw(resp.Body)
}

// cbgApiGet GET api 端点（记录等）。
func cbgApiGet(ctx context.Context, hc *httpclient.Client, baseURL, apiPath, refererPage, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, apiPath)
	resp, err := hc.Do(ctx, http.MethodGet, u, nil, map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, refererPage),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("cbg "+apiPath, u, resp)
	return parseRaw(resp.Body)
}

// cbgBalance 抠页面余额（#current-magic-point）。
func cbgBalance(body []byte) float64 {
	return novaHDNum(firstGroup(body, `(?s)current-magic-point[^>]*>([\d,.]+)`))
}

// ── 幸运大转盘 ────────────────────────────────────────────

// CbgWheelGame 幸运大转盘：POST api/api_luckywheel.php {count} → {status,new_magic_point,prize_ids,extra_message}。
type CbgWheelGame struct{}

func (CbgWheelGame) ID() string   { return "cbg_wheel" }
func (CbgWheelGame) Name() string { return "幸运大转盘" }

func (CbgWheelGame) draw(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	m, err := cbgApiPost(ctx, hc, baseURL, "api/api_luckywheel.php", "luckywheel.php", cr.Cookie, map[string]any{"count": count})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	res := &PlayResult{
		Success: strField(m, "status") == "success",
		Balance: numField(m, "new_magic_point"),
		Prize:   strField(m, "extra_message"),
		Message: strField(m, "message"),
		Raw:     m,
	}
	if ids, ok := m["prize_ids"].([]any); ok {
		res.Prizes = make([]PrizeOutcome, 0, len(ids))
		for _, id := range ids {
			res.Prizes = append(res.Prizes, PrizeOutcome{PrizeID: int(longptNum(id)), IsWin: true})
		}
	}
	if res.Prize == "" && len(res.Prizes) > 0 {
		res.Prize = "中奖"
	}
	if !res.Success && res.Message == "" {
		res.Message = "抽奖失败"
	}
	return res, nil
}

func (CbgWheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return CbgWheelGame{}.draw(ctx, hc, baseURL, cr, 1)
}

func (CbgWheelGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	if count > maxMultiCount {
		count = maxMultiCount
	}
	return CbgWheelGame{}.draw(ctx, hc, baseURL, cr, count)
}

func (CbgWheelGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "luckywheel.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: cbgBalance(resp.Body)}, nil
}

func (CbgWheelGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	m, err := cbgApiGet(ctx, hc, baseURL, "api/api_luckywheel_log.php?page=1", "luckywheel.php", cr.Cookie)
	if err != nil || m == nil {
		return []GameRecord{}, nil
	}
	logs, _ := m["logs"].([]any)
	out := make([]GameRecord, 0, len(logs))
	for _, l := range logs {
		lm, ok := l.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, GameRecord{
			PrizeName: strField(lm, "name"),
			CreatedAt: strField(lm, "time"),
		})
	}
	return out, nil
}

// ── 魔力老虎机 ────────────────────────────────────────────

// CbgSlotGame 魔力老虎机：POST api/api_slotmachine.php → {status,results_index:[i,j,k],message}。
type CbgSlotGame struct{}

func (CbgSlotGame) ID() string   { return "cbg_slot" }
func (CbgSlotGame) Name() string { return "魔力老虎机" }

func (CbgSlotGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := cbgApiPost(ctx, hc, baseURL, "api/api_slotmachine.php", "slotmachine.php", cr.Cookie, nil)
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	res := &PlayResult{
		Success: strField(m, "status") == "success",
		Balance: numField(m, "new_magic_point"),
		Message: strField(m, "message"),
		Raw:     m,
	}
	if idx, ok := m["results_index"].([]any); ok {
		parts := make([]string, 0, len(idx))
		for _, x := range idx {
			parts = append(parts, strconv.FormatFloat(longptNum(x), 'f', -1, 64))
		}
		if len(parts) > 0 {
			res.Prize = "转出: " + strings.Join(parts, " | ")
		}
	}
	return res, nil
}

func (CbgSlotGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "slotmachine.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: cbgBalance(resp.Body)}, nil
}

// ── 批13 富 UI 状态端点 ─────────────────────────────────────────────

// CbgWheelState 转盘全景：余额/单价/10 格奖品表/最近记录。
type CbgWheelState struct {
	Balance float64      `json:"balance"`
	Cost    float64      `json:"cost"` // 每次魔力（页面 COST_MP）
	Prizes  []PrizeDef   `json:"prizes"`
	Recent  []GameRecord `json:"recent,omitempty"`
}

// CbgWheelable 转盘全景能力。
type CbgWheelable interface {
	CbgWheelState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*CbgWheelState, error)
}

// cbgPrizesRe 页面内嵌 INITIAL_PRIZES = [{"name":...,"id":N,...}]。
var cbgPrizesRe = compileRegex(`INITIAL_PRIZES\s*=\s*(\[.*?\]);`)
var cbgPrizeItemRe = compileRegex(`"name":"([^"]+)"[^{}]*?"id":([0-9]+)|"id":([0-9]+)[^{}]*?"name":"([^"]+)"`)

// cbgCostRe 页面 COST_MP = N。
var cbgCostRe = compileRegex(`COST_MP\s*=\s*(\d+)`)

// cbgParsePrizes 从页面 JS 抠奖品表（unicode 转义用 json 还原）。
func cbgParsePrizes(body string) []PrizeDef {
	m := cbgPrizesRe.FindStringSubmatch(body)
	if m == nil {
		return nil
	}
	out := []PrizeDef{}
	for _, sm := range cbgPrizeItemRe.FindAllStringSubmatch(m[1], -1) {
		name, idStr := sm[1], sm[2]
		if name == "" {
			name, idStr = sm[4], sm[3]
		}
		var unquoted string
		if err := json.Unmarshal([]byte(`"`+strings.ReplaceAll(name, `"`, `\"`)+`"`), &unquoted); err == nil {
			name = unquoted
		}
		id, _ := strconv.Atoi(idStr)
		out = append(out, PrizeDef{ID: id, Name: name})
	}
	return out
}

// CbgWheelState 实现：页面（余额+奖品+单价）+ History。
func (g CbgWheelGame) CbgWheelState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*CbgWheelState, error) {
	page := joinPath(baseURL, "luckywheel.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	st := &CbgWheelState{Balance: cbgBalance(resp.Body), Prizes: []PrizeDef{}}
	if m := cbgCostRe.FindStringSubmatch(string(resp.Body)); m != nil {
		st.Cost, _ = strconv.ParseFloat(m[1], 64)
	}
	st.Prizes = cbgParsePrizes(string(resp.Body))
	if recent, err := g.History(ctx, hc, baseURL, cr); err == nil {
		st.Recent = recent
	}
	return st, nil
}

// CbgSlotState 老虎机全景：余额/单价/图案序/最近记录。
type CbgSlotState struct {
	Balance float64      `json:"balance"`
	Cost    float64      `json:"cost"`    // 每次魔力（页面 COST_MP）
	Symbols []int        `json:"symbols"` // 1..9 号图案
	Recent  []GameRecord `json:"recent,omitempty"`
}

// CbgSlotable 老虎机全景能力。
type CbgSlotable interface {
	CbgSlotState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*CbgSlotState, error)
}

// CbgSlotState 实现。
func (g CbgSlotGame) CbgSlotState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*CbgSlotState, error) {
	page := joinPath(baseURL, "slotmachine.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	st := &CbgSlotState{Balance: cbgBalance(resp.Body), Symbols: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}}
	if m := cbgCostRe.FindStringSubmatch(string(resp.Body)); m != nil {
		st.Cost, _ = strconv.ParseFloat(m[1], 64)
	}
	if h, err := cbgApiGet(ctx, hc, baseURL, "api/api_slotmachine_log.php?page=1", "slotmachine.php", cr.Cookie); err == nil && h != nil {
		if logs, ok := h["logs"].([]any); ok {
			for i, l := range logs {
				if i >= 10 {
					break
				}
				lm, ok := l.(map[string]any)
				if !ok {
					continue
				}
				st.Recent = append(st.Recent, GameRecord{
					PrizeName: firstNonEmpty(strField(lm, "reel_result"), strField(lm, "result")),
					Amount:    numField(lm, "reward_mp"),
					CreatedAt: strField(lm, "time"),
				})
			}
		}
	}
	return st, nil
}
