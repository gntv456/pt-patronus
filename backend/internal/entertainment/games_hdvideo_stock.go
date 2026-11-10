// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── HDVIDEO 股票模拟交易（/games/stock/，market.php）────────────────
// 契约（2026-09 实测页面内联 stockState + market.php）：
//
//	GET  /games/market.php?ajax=quotes → {ok,quotes:[{symbol,name,price,percent,tradeable…}]}
//	页面 /games/stock/ 内嵌 stockState：{quotes,portfolio{holdings,market_value,profit},
//	  market{open,label},wallet,token,feeRate…}
//	POST /games/market.php form token+symbol+side=buy|sell+shares+order_key → {ok,message}
//
// 真实 A 股行情、电影票虚拟买卖。Play 只读（行情+持仓+盈亏）；买卖走 Bet 显式。
type HdvideoStockGame struct{ HdvideoBase }

func (g HdvideoStockGame) ID() string   { return "hdvideo_stock" }
func (g HdvideoStockGame) Name() string { return "股票模拟" }

// hdvStockState GET stock 页并提取内嵌 stockState JSON（quotes 后整段）。
func hdvStockState(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, "games/stock/")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{"Referer": u}, true)
	if err != nil {
		return nil, err
	}
	logResp("hdvideo stock page", u, resp)
	body := string(resp.Body)
	i := strings.Index(body, `stockState`)
	if i < 0 {
		return nil, fmt.Errorf("页面无 stockState（可能未登录或改版）")
	}
	j := strings.Index(body[i:], "{")
	if j < 0 {
		return nil, fmt.Errorf("stockState 格式异常")
	}
	raw := body[i+j:]
	depth, end := 0, -1
	for k, ch := range raw {
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = k + 1
			}
		}
		if end > 0 {
			break
		}
	}
	if end <= 0 {
		return nil, fmt.Errorf("stockState JSON 截断")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw[:end]), &m); err != nil {
		return nil, fmt.Errorf("parse stockState: %w", err)
	}
	return m, nil
}

// Play 只读：行情摘要 + 持仓 + 盈亏 + 市场状态。
func (g HdvideoStockGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := hdvStockState(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: true, Raw: m}
	res.Balance = numField(m, "wallet")
	parts := []string{}
	if mk, ok := m["market"].(map[string]any); ok {
		parts = append(parts, fmt.Sprintf("市场%s", strField(mk, "label")))
	}
	if pf, ok := m["portfolio"].(map[string]any); ok {
		mv := numField(pf, "market_value")
		profit := numField(pf, "profit")
		n := 0
		if h, ok := pf["holdings"].([]any); ok {
			n = len(h)
		}
		parts = append(parts, fmt.Sprintf("持仓 %d 只、市值 %.1f、浮盈 %.1f", n, mv, profit))
	}
	if q, ok := m["quotes"].([]any); ok && len(q) > 0 {
		parts = append(parts, fmt.Sprintf("标的 %d 只", len(q)))
	}
	res.Message = "股票模拟已同步：" + strings.Join(parts, "；") + "。买卖走下注面板（Selection=buy|sell:代码）"
	return res, nil
}

// Bet 交易：Selection="buy:SH600519" / "sell:SZ000858"，Amount=股数（100 的整数倍）。
// token 从页面 state 取（防重放）；order_key 由后端生成幂等键。
func (g HdvideoStockGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	sel := strings.TrimSpace(req.Selection)
	side, symbol, ok := strings.Cut(sel, ":")
	if !ok {
		return &PlayResult{Success: false, Message: "格式 buy:SH600519 或 sell:SZ000858"}, nil
	}
	side = strings.ToLower(side)
	if side != "buy" && side != "sell" {
		return &PlayResult{Success: false, Message: "方向仅 buy/sell"}, nil
	}
	if req.Amount < 100 {
		return &PlayResult{Success: false, Message: "股数至少 100（页面 min）"}, nil
	}
	st, err := hdvStockState(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	token := strField(st, "token")
	if token == "" {
		return &PlayResult{Success: false, Message: "未取得交易 token"}, nil
	}
	form := url.Values{}
	form.Set("token", token)
	form.Set("symbol", symbol)
	form.Set("side", side)
	form.Set("shares", strconv.Itoa(int(req.Amount)))
	form.Set("order_key", fmt.Sprintf("ptpatronus-%d", int64(req.Amount)))
	u := joinPath(baseURL, "games/market.php")
	resp, err := hc.Do(ctx, "POST", u, []byte(form.Encode()), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "games/stock/"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("hdvideo stock trade", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "响应解析失败"}, nil
	}
	okRes := boolField(m, "ok")
	return &PlayResult{Success: okRes,
		Message: firstNonEmpty(strField(m, "message"), strField(m, "error")),
		Raw:     m}, nil
}

// Stats 钱包余额。
func (g HdvideoStockGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	m, err := hdvStockState(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: numField(m, "wallet")}, nil
}
