// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// Audiences（audiences.me）二十一点：POST blackjack.php form game=hit|stand|double|new&ajax=1 + X-Requested-With。
// 响应混合：{mode:'content', html, bonus}（html 片段含牌面）或 {ok, heading, message}。牌面抠取待真实页面校准。
type AudiencesBlackjackGame struct{}

func (AudiencesBlackjackGame) ID() string   { return "audiences_blackjack" }
func (AudiencesBlackjackGame) Name() string { return "二十一点" }

func (AudiencesBlackjackGame) Blackjack(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string) (*BlackjackResult, error) {
	if action == "" {
		action = "new"
	}
	u := joinPath(baseURL, "blackjack.php")
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte("game="+action+"&ajax=1"), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          u,
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("blackjack "+action, u, resp)
	m, err := parseRaw(resp.Body)
	res := &BlackjackResult{}
	if err != nil {
		return res, nil
	}
	res.Balance = numField(m, "bonus")
	res.Message = firstNonEmpty(strField(m, "heading"), strField(m, "message"))
	if html, ok := m["html"].(string); ok && html != "" {
		res.Stage = "playing"
		lower := strings.ToLower(html)
		switch {
		case strings.Contains(lower, "bust") || strings.Contains(html, "爆"):
			res.Result = "lose"
		case strings.Contains(lower, "blackjack") || strings.Contains(html, " blackjack"):
			res.Result = "blackjack"
		case strings.Contains(html, "赢") || strings.Contains(lower, "win"):
			res.Result = "win"
		case strings.Contains(html, "平") || strings.Contains(lower, "push"):
			res.Result = "push"
		}
		if res.Result != "" {
			res.Stage = "finished"
		}
		res.PlayerCards = parseBlackjackCards(html)
	}
	if res.Message == "" {
		res.Message = action + " 完成"
	}
	return res, nil
}

// parseBlackjackCards 宽松抠牌（♠♥♦♣ + 点数）；精确结构待真实页面校准。
func parseBlackjackCards(html string) []string {
	re := compileRegex(`[♠♥♦♣🂡-🂾][\dAJQK]+|[\dAJQK]+[♠♥♦♣]`)
	return re.FindAllString(html, -1)
}

// Play（LotteryGame）= 开始新局（new），转 PlayResult。
func (AudiencesBlackjackGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	r, err := AudiencesBlackjackGame{}.Blackjack(ctx, hc, baseURL, cr, "new")
	if err != nil {
		return nil, err
	}
	return &PlayResult{Success: true, Balance: r.Balance, Prize: r.Result, Message: r.Message}, nil
}

// Stats 余额（页面 [data-bonus-value]）。
func (AudiencesBlackjackGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "blackjack.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: novaHDNum(firstGroup(resp.Body, `data-bonus-value="([\d,.]+)"`))}, nil
}
