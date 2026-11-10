// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package groupchat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// longPTHandler LongPT：喊话走 JSON API(POST /pt-api/v1/nexus/shoutbox/shout body {text})，
// 响应 {code,msg}；额外实现 LotteryPlayer(每日抽奖 GET /pt-api/v1/lucky/.../join)。
// 不嵌入 nexusPHPBase(协议完全不同)。
type longPTHandler struct{}

func (longPTHandler) Match(s SiteRef) bool {
	return strings.Contains(strings.ToLower(s.Name), "longpt")
}

func (longPTHandler) Send(ctx context.Context, c Ctx, message string) (*Feedback, error) {
	endpoint := joinBase(c.Site.BaseURL, "/pt-api/v1/nexus/shoutbox/shout")
	body, _ := json.Marshal(map[string]string{"text": message})
	resp, err := c.HC.Do(ctx, "POST", endpoint, body, map[string]string{
		"Content-Type": "application/json",
		"Referer":      refererOf(c.Site.BaseURL),
	}, c.Cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("longpt shoutbox HTTP %d", resp.StatusCode)
	}
	fb := parseLongPTResponse(resp.Body)
	fb.Sent = true
	return fb, nil
}

// parseLongPTResponse 解析 LongPT JSON 响应 {code,msg}：code==0 成功；msg 剥 [emXXX] 表情码后分类奖励。
var emCodeRe = regexp.MustCompile(`\[em\d+\]`)

func parseLongPTResponse(body []byte) *Feedback {
	fb := &Feedback{}
	var res struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return fb
	}
	if res.Code != 0 {
		return fb
	}
	msg := emCodeRe.ReplaceAllString(res.Msg, "")
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return fb
	}
	fb.Raw = msg
	fb.Rewards = classifyRewards(msg)
	return fb
}

// PlayLottery 实现 LotteryPlayer：GET /pt-api/v1/lucky/pluginsPrizeReceiptRecord/join。
func (longPTHandler) PlayLottery(ctx context.Context, c Ctx) (string, error) {
	endpoint := joinBase(c.Site.BaseURL, "/pt-api/v1/lucky/pluginsPrizeReceiptRecord/join")
	resp, err := c.HC.GetWithHeaders(ctx, endpoint, c.Cookie, map[string]string{"Referer": refererOf(c.Site.BaseURL)}, true)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("lottery HTTP %d", resp.StatusCode)
	}
	var res struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(resp.Body), &res); err != nil {
		return string(resp.Body), nil
	}
	if res.Code == 0 {
		return "已参与", nil
	}
	return strings.TrimSpace(res.Msg), nil
}
