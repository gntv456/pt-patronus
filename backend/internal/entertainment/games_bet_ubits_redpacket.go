// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── UBits 红包（ubits.club） ──────────────────────────────
// POST redpacket.php form action=send|claim（整页 HTML，无 JSON）。send: amount=总额+extra(packet_type/distribution_type/packet_count)；claim: extra.id。
type UbitsRedpacketGame struct{}

// ubitsRedpacketFailRe 站点失败页常见文案。整页 HTML 无结构化结果，
// 命中关键词才算失败，未命中才视为成功（不能无条件报成功）。
var ubitsRedpacketFailRe = regexp.MustCompile(`失败|不足|已领取过|已抢完|已领完|手慢|已过期|不存在`)

func (UbitsRedpacketGame) ID() string   { return "ubits_redpacket" }
func (UbitsRedpacketGame) Name() string { return "红包" }

func (UbitsRedpacketGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	action := req.Selection
	if action == "" {
		action = "claim"
	}
	u := joinPath(baseURL, "redpacket.php")
	var body string
	if action == "send" {
		if req.Amount <= 0 {
			return &PlayResult{Success: false, Message: "红包总额须 > 0"}, nil
		}
		amt := req.Amount
		if amt > maxBetCeiling {
			amt = maxBetCeiling
		}
		body = "action=send&amount=" + strconv.FormatFloat(amt, 'f', -1, 64) +
			"&packet_type=" + req.Extra["packet_type"] + "&distribution_type=" + req.Extra["distribution_type"] +
			"&packet_count=" + req.Extra["packet_count"]
	} else {
		body = "action=claim&id=" + req.Extra["id"]
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(body), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Referer":      u,
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("ubits_redpacket "+action, u, resp)
	msg := strings.TrimSpace(stripTags(firstGroup(resp.Body, `(?s)(成功|失败|领取|已|红包)[^<]{0,40}`)))
	return &PlayResult{
		Success: !ubitsRedpacketFailRe.Match(resp.Body),
		Balance: novaHDNum(firstGroup(resp.Body, `U币[^0-9-]*([\d,.]+)`)),
		Prize:   action,
		Message: firstNonEmpty(msg, action+" 操作已提交"),
	}, nil
}

func (UbitsRedpacketGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return UbitsRedpacketGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{Selection: "claim"})
}
