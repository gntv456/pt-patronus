// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── HDVIDEO（hdvideo.top）/games/ 游戏大厅家族 ──────────────────────
// 全部为 /games/<name>/ 整页表单或同地址 JSON POST（2026-09 实测页面内联 JS）：
//
//	big-small：POST form choice=big|small|triple|straight|number(&bet_number=111-666)&amount
//	slots：    POST form action=spin&bet=N → {ok,reels:[3],...}
//	scratch：  POST form action=buy → {ok,name,reward,type}
//	hilo：     POST form action=start&bet=N / guess&dir=hi|lo / cashout
//	blackjack：POST form action=deal&bet=N / hit / stand / double
//
// 货币为「电影票」。除刮刮乐固定价外均会扣票——下注/转动只走 Bet 显式；hilo 状态机与
// 21点动作机走各自动作。ddz/poker/zjh/sports/stock/chest 为对战/复杂玩法不接。
type HdvideoBase struct{}

// hdvPostForm POST form 到 /games/<game>/ 并解析 JSON。
func hdvPostForm(ctx context.Context, hc *httpclient.Client, baseURL, game, cookie, form string) (map[string]any, error) {
	u := joinPath(baseURL, "games/"+game+"/")
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          u,
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("hdvideo "+game, u, resp)
	return parseRaw(resp.Body)
}

// hdvPage GET 游戏页（提取余额/状态用）。
func hdvPage(ctx context.Context, hc *httpclient.Client, baseURL, game, cookie string) (string, error) {
	u := joinPath(baseURL, "games/"+game+"/")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{"Referer": u}, true)
	if err != nil {
		return "", err
	}
	logResp("hdvideo "+game+" page", u, resp)
	return string(resp.Body), nil
}

func hdvBalance(body string) float64 {
	m := compileRegex(`我的电影票[：:]\s*(?:<b[^>]*>)?\s*([\d,.]+)`).FindStringSubmatch(body)
	if m == nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	return v
}

// ── 压大小（骰子三期制：大/小/豹子/顺子/数字） ──────────────────────
type HdvideoBigSmallGame struct{ HdvideoBase }

func (g HdvideoBigSmallGame) ID() string   { return "hdvideo_bigsmall" }
func (g HdvideoBigSmallGame) Name() string { return "压大小" }

// Play 只读：余额 + 本期截止。
func (g HdvideoBigSmallGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	body, err := hdvPage(ctx, hc, baseURL, "big-small", cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: true, Balance: hdvBalance(body)}
	if m := compileRegex(`本期截止\s*([\d\- :]+)`).FindStringSubmatch(body); m != nil {
		res.Message = fmt.Sprintf("已同步压大小，本期截止 %s", strings.TrimSpace(m[1]))
	} else {
		res.Message = "已同步压大小"
	}
	return res, nil
}

// Bet 押注：Selection=big|small|triple|straight|number；Extra.number=XYZ（押数字时 111-666）；Amount=票数。
func (g HdvideoBigSmallGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	choice := strings.ToLower(strings.TrimSpace(req.Selection))
	switch choice {
	case "big", "small", "triple", "straight":
	case "number":
	default:
		return &PlayResult{Success: false, Message: "请选 big/small/triple/straight/number"}, nil
	}
	if req.Amount <= 0 {
		return &PlayResult{Success: false, Message: "请填写票数"}, nil
	}
	form := url.Values{}
	form.Set("choice", choice)
	if choice == "number" {
		num := strings.TrimSpace(req.Extra["number"])
		if !compileRegex(`^[1-6]{3}$`).MatchString(num) {
			return &PlayResult{Success: false, Message: "押数字需 Extra.number 为 111-666 三位（每位 1-6）"}, nil
		}
		form.Set("bet_number", num)
	}
	form.Set("amount", strconv.Itoa(int(req.Amount)))
	m, err := hdvPostForm(ctx, hc, baseURL, "big-small", cr.Cookie, form.Encode())
	if err != nil {
		return nil, err
	}
	// 非 AJAX 提交可能返回整页 HTML——解析失败视为表单提交成功后重定向，重拉余额确认
	if m == nil {
		body, _ := hdvPage(ctx, hc, baseURL, "big-small", cr.Cookie)
		return &PlayResult{Success: true, Message: "押注已提交", Balance: hdvBalance(body)}, nil
	}
	ok := boolField(m, "ok") || boolField(m, "success")
	return &PlayResult{Success: ok,
		Message: firstNonEmpty(strField(m, "message"), strField(m, "error"), strField(m, "msg")),
		Raw:     m}, nil
}

// Stats 余额。
func (g HdvideoBigSmallGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := hdvPage(ctx, hc, baseURL, "big-small", cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: hdvBalance(body)}, nil
}

// ── 老虎机（action=spin&bet=N） ─────────────────────────────────────
type HdvideoSlotsGame struct{ HdvideoBase }

func (g HdvideoSlotsGame) ID() string   { return "hdvideo_slots" }
func (g HdvideoSlotsGame) Name() string { return "老虎机" }

// Spin 转一次（MultiPlayable 语义：PlayMulti=连转 count 次，单次bet=N 走 Bet）。
func (g HdvideoSlotsGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	body, err := hdvPage(ctx, hc, baseURL, "slots", cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	return &PlayResult{Success: true, Balance: hdvBalance(body), Message: "已同步老虎机，转动走下注面板"}, nil
}

// Bet 转一次：Amount=单注票数（页面快捷 100/500/1000）。
func (g HdvideoSlotsGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	bet := int(req.Amount)
	if bet <= 0 {
		bet = 100
	}
	m, err := hdvPostForm(ctx, hc, baseURL, "slots", cr.Cookie, "action=spin&bet="+strconv.Itoa(bet))
	if err != nil {
		return nil, err
	}
	ok := boolField(m, "ok")
	msg := ""
	if ok {
		if reels, ok2 := m["reels"].([]any); ok2 && len(reels) == 3 {
			msg = fmt.Sprintf("转轴 %v%v%v", reels[0], reels[1], reels[2])
		}
		if w := numField(m, "win"); w > 0 {
			msg += fmt.Sprintf("，赢得 %v 票", w)
		} else if msg == "" {
			msg = "未中奖"
		}
	} else {
		msg = firstNonEmpty(strField(m, "error"), "转动失败")
	}
	return &PlayResult{Success: ok, Message: msg, Raw: m}, nil
}

// Stats 余额。
func (g HdvideoSlotsGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := hdvPage(ctx, hc, baseURL, "slots", cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: hdvBalance(body)}, nil
}

// ── 刮刮乐（action=buy，固定价） ────────────────────────────────────
type HdvideoScratchGame struct{ HdvideoBase }

func (g HdvideoScratchGame) ID() string   { return "hdvideo_scratch" }
func (g HdvideoScratchGame) Name() string { return "刮刮乐" }

// PlayMulti 买一张刮刮乐（免费动作外唯一消费，固定票价站点侧结算）。
func (g HdvideoScratchGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	var msgs []string
	balance := 0.0
	for i := 0; i < count; i++ {
		m, err := hdvPostForm(ctx, hc, baseURL, "scratch", cr.Cookie, "action=buy")
		if err != nil {
			return &PlayResult{Success: false, Message: err.Error()}, nil
		}
		if !boolField(m, "ok") {
			return &PlayResult{Success: false,
				Message: firstNonEmpty(strField(m, "error"), "购买失败"), Raw: m}, nil
		}
		msgs = append(msgs, fmt.Sprintf("%s（%s）", strField(m, "name"), strField(m, "reward")))
		if w := numField(m, "balance"); w > 0 {
			balance = w
		}
	}
	return &PlayResult{Success: true, Balance: balance,
		Message: "刮出：" + strings.Join(msgs, "；")}, nil
}

// Play 只读余额。
func (g HdvideoScratchGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	body, err := hdvPage(ctx, hc, baseURL, "scratch", cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	return &PlayResult{Success: true, Balance: hdvBalance(body), Message: "已同步刮刮乐，购买走连刮"}, nil
}

// Stats 余额。
func (g HdvideoScratchGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := hdvPage(ctx, hc, baseURL, "scratch", cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: hdvBalance(body)}, nil
}
