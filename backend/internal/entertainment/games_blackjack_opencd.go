// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── OpenCD（open.cd）21 點 ────────────────────────────────
// 契约（2026-09 实测 blackjack.php 页面表单）：
//
//	开局：POST game=start&bonus=<赌注>（自建对局，等对手来赌）
//	应战：POST game=start&gameid=<对方局 id>（賭這一局）
//	进行中动作：game=hit/stand/double（与 Nxp 家族相同，无 start=yes）
//
// 页面另有「當前可對抗賭局」列表（username+赌注+gameid 表单行）。
// 货币为魔力值，赢家扣 10% 场费。开局即扣赌注——new 不自动开局，下注开局走 Blackjack action=start。
type OpenCDBlackjackGame struct{}

func (OpenCDBlackjackGame) ID() string   { return "opencd_blackjack" }
func (OpenCDBlackjackGame) Name() string { return "21 點遊戲" }

// opencdBjGet 取整页。
func opencdBjGet(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (string, error) {
	return nxpBjGet(ctx, hc, baseURL, cookie)
}

// opencdBjPost 提交（game + 可选 bonus/gameid）。
func opencdBjPost(ctx context.Context, hc *httpclient.Client, baseURL, cookie, action string, extra url.Values) (string, error) {
	u := joinPath(baseURL, "blackjack.php")
	form := url.Values{}
	form.Set("game", action)
	for k, vs := range extra {
		for _, v := range vs {
			form.Set(k, v)
		}
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form.Encode()), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Referer":      u,
	}, cookie, true)
	if err != nil {
		return "", err
	}
	logResp("opencd_blackjack "+action, u, resp)
	return string(resp.Body), nil
}

var (
	opencdBjChallengingRe = regexp.MustCompile(`name=['"]?gameid['"]?[^>]*value=['"]?(\d+)`)
	opencdBjIdleFormRe    = regexp.MustCompile(`name=['"]?bonus['"]?`)
)

// opencdBjParse 复用 Nxp 牌面/点数/余额解析；状态判据按 OpenCD 形态：
// gameid 表单=可对抗他局（idle 语义），bonus 表单=开局入口（idle），
// hit/stand/double 无 start=yes =进行中。
func opencdBjParse(body string) *BlackjackResult {
	res := nxpBjParse(body)
	// OpenCD 无 start=yes 标记：把 nxp 视作 idle 的「开局表单」也排除掉，
	// 真正的进行中判据 = 动作表单（hit/stand/double 且无 gameid/bonus 同现）。
	inGame := false
	for _, fm := range nxpBjFormRe.FindAllString(body, -1) {
		mm := nxpBjInputRe.FindStringSubmatch(fm)
		if mm == nil {
			continue
		}
		switch mm[1] {
		case "hit", "stand", "double":
			if !opencdBjChallengingRe.MatchString(fm) && !opencdBjIdleFormRe.MatchString(fm) {
				inGame = true
			}
		}
	}
	if inGame {
		res.Stage = "playing"
		res.Result = "playing"
	} else if len(res.PlayerCards) > 0 {
		res.Stage = "finished"
	} else {
		res.Stage = "idle"
		// 可对抗局提示
		if ids := opencdBjChallengingRe.FindAllStringSubmatch(body, -1); len(ids) > 0 {
			res.Message = "当前无进行中对局；有 " + strconv.Itoa(len(ids)) + " 个可对抗赌局（应战走 action=challenge&gameid=…）"
			return res
		}
		res.Message = "当前无对局：开局走 action=start&bonus=赌注（网页确认），或应战 action=challenge"
	}
	return res
}

// Blackjack 实现 Blackjackable：
//
//	new/hit/stand/double 同常规；start=自开新局（网页确认后显式调用）；
//	challenge=应战他人局（gameid 经 action 形如 "challenge:651388" 传入）。
func (g OpenCDBlackjackGame) Blackjack(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string) (*BlackjackResult, error) {
	if action == "" {
		action = "new"
	}
	var extra url.Values
	switch {
	case action == "new":
		body, err := opencdBjGet(ctx, hc, baseURL, cr.Cookie)
		if err != nil {
			return nil, err
		}
		return opencdBjParse(body), nil
	case action == "start":
		// 开局需赌注：默认 500（页面默认值），此处不自动调用——由前端显式触发
		extra = url.Values{"bonus": {"500"}}
	case strings.HasPrefix(action, "challenge:"):
		extra = url.Values{"gameid": {strings.TrimPrefix(action, "challenge:")}, "start": {"yes"}}
		action = "start"
	case action == "hit" || action == "stand" || action == "double":
		extra = url.Values{}
	default:
		return nil, fmt.Errorf("不支持的动作 %q", action)
	}
	body, err := opencdBjPost(ctx, hc, baseURL, cr.Cookie, action, extra)
	if err != nil {
		return nil, err
	}
	return opencdBjParse(body), nil
}

// Play（LotteryGame 兜底）= 读当前牌局。
func (g OpenCDBlackjackGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	r, err := g.Blackjack(ctx, hc, baseURL, cr, "new")
	if err != nil {
		return nil, err
	}
	return &PlayResult{Success: true, Balance: r.Balance, Prize: r.Result, Message: r.Message}, nil
}

// Stats 余额（blackjack.php 页内魔力值）。
func (g OpenCDBlackjackGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := opencdBjGet(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{}
	if m := nxpBjBonusRe.FindStringSubmatch(body); m != nil {
		st.Balance, _ = strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	}
	return st, nil
}
