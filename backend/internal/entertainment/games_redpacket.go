// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 好学（hxpt.org）红包雨：POST hby/redpacket_api.php action=record_trigger（建本轮，受日限5/时限1）
// → 循环 POST hby/redpacket.php（空 body，带 Referer/Origin）→ {success,reward_type,reward_value}。
// Play 封装「触发 + 循环点击至本轮上限」，累计 seedbonus 收益。
type HxptRedpacketGame struct{}

func (HxptRedpacketGame) ID() string   { return "hxpt_redpacket" }
func (HxptRedpacketGame) Name() string { return "红包雨" }

func (HxptRedpacketGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	apiURL := joinPath(baseURL, "hby/redpacket_api.php")
	// 1. record_trigger（建本轮 Redis 状态）
	resp, err := hc.Do(ctx, http.MethodPost, apiURL, []byte("action=record_trigger"), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "hby/redpacket.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("hxpt_redpacket trigger", apiURL, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "解析触发结果失败"}, nil
	}
	if !boolField(m, "success") {
		return &PlayResult{Success: false, Message: strField(m, "message")}, nil
	}
	clickLimit := int(longptNum(m["click_limit"]))
	if clickLimit <= 0 {
		clickLimit = 30
	}
	// 硬上限：防服务端返回/被篡改超大值导致循环近百万次 POST。
	if clickLimit > maxRedpacketClick {
		clickLimit = maxRedpacketClick
	}
	// 2. 循环点击领红包（每轮上限 clickLimit）
	rpURL := joinPath(baseURL, "hby/redpacket.php")
	headers := map[string]string{"Referer": joinPath(baseURL, "hby/redpacket.php")}
	var total float64
	wins := 0
	for i := 0; i < clickLimit; i++ {
		r, err := hc.Do(ctx, http.MethodPost, rpURL, nil, headers, cr.Cookie, true)
		if err != nil {
			break
		}
		rm, err := parseRaw(r.Body)
		if err != nil || !boolField(rm, "success") {
			break // 本轮结束（达上限/过期）
		}
		if strField(rm, "reward_type") == "seedbonus" {
			total += numField(rm, "reward_value")
		}
		wins++
	}
	return &PlayResult{
		Success: true,
		Amount:  total,
		Prize:   fmt.Sprintf("抢到 %d 个红包", wins),
		Message: fmt.Sprintf("本轮抢到 %d 个红包，火花 +%g", wins, total),
	}, nil
}

// Stats 触发配额（check_trigger）：今日已触发 / 日限。
func (HxptRedpacketGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	apiURL := joinPath(baseURL, "hby/redpacket_api.php")
	resp, err := hc.Do(ctx, http.MethodPost, apiURL, []byte("action=check_trigger"), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "hby/redpacket.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &StatsResult{}, nil
	}
	return &StatsResult{
		TodaySpins: int(longptNum(m["today_triggers"])),
		MaxDaily:   int(longptNum(m["max_daily"])),
	}, nil
}
