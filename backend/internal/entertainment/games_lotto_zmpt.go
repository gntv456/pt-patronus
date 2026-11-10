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

// ── 织梦 双色球（zmpt.cc，union-lotto.php）─────────────────────────
// 与麒麟同源（omnibot 家族）：前端 fetch 封装统一加 /nodeapi 前缀（实测 BY0sxZy4.js）。
//
//	GET  /nodeapi/union-lotto  → {code:0,data:{time,list,totalAmount,UnionLottoInceptionAmount}}
//	POST /nodeapi/union-lotto  json {nums:[6红+1蓝]} → {code,message}
//
// 红球 1-33 六个 + 蓝球 1-16 一个；开奖 time 形如 "2026-09-03(周四)21:15:00"。
type ZmptLottoGame struct{}

func (ZmptLottoGame) ID() string   { return "zmpt_lotto" }
func (ZmptLottoGame) Name() string { return "双色球" }

// zmptLottoGet GET 状态。
func zmptLottoGet(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, "nodeapi/union-lotto")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{
		"Accept":  "application/json, text/javascript, */*; q=0.01",
		"Referer": joinPath(baseURL, "union-lotto.php"),
	}, true)
	if err != nil {
		return nil, err
	}
	logResp("zmpt_lotto state", u, resp)
	return parseRaw(resp.Body)
}

// Play 只读：本期投注 + 奖池。
func (g ZmptLottoGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := zmptLottoGet(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: int(longptNum(m["code"])) == 0, Raw: m}
	n, pool, drawAt := 0, "", ""
	if d, ok := m["data"].(map[string]any); ok {
		if lst, ok := d["list"].([]any); ok {
			n = len(lst)
		}
		pool = strOrDash(d["totalAmount"])
		drawAt = strField(d, "time")
	}
	res.Message = fmt.Sprintf("已同步双色球：本期我的投注 %d 注，奖池 %s，开奖 %s", n, pool, drawAt)
	return res, nil
}

// zmptNumsRe 选号文本 "1,2,3,4,5,6|7"。
var zmptNumsRe = regexp.MustCompile(`^(\d{1,2})\D+(\d{1,2})\D+(\d{1,2})\D+(\d{1,2})\D+(\d{1,2})\D+(\d{1,2})\D+(\d{1,2})$`)

// Bet 投一注：Selection="1,2,3,4,5,6|7"。
func (g ZmptLottoGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	nums := zmptNumsRe.FindStringSubmatch(strings.TrimSpace(req.Selection))
	if nums == nil {
		return &PlayResult{Success: false, Message: "选号格式：6 个红球(1-33)+1 个蓝球(1-16)，如 1,2,3,4,5,6|7"}, nil
	}
	arr := make([]int, 0, 7)
	seen := map[int]bool{}
	for _, ns := range nums[1:] {
		n, _ := strconv.Atoi(ns)
		if seen[n] {
			return &PlayResult{Success: false, Message: "号码有重复"}, nil
		}
		seen[n] = true
		arr = append(arr, n)
	}
	for i := 0; i < 6; i++ {
		if arr[i] < 1 || arr[i] > 33 {
			return &PlayResult{Success: false, Message: "红球须在 1-33"}, nil
		}
	}
	if arr[6] < 1 || arr[6] > 16 {
		return &PlayResult{Success: false, Message: "蓝球须在 1-16"}, nil
	}
	payload, _ := json.Marshal(map[string]any{"nums": arr})
	u := joinPath(baseURL, "nodeapi/union-lotto")
	resp, err := hc.Do(ctx, http.MethodPost, u, payload, map[string]string{
		"Content-Type":     "application/json; charset=UTF-8",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "union-lotto.php"),
	}, cr.Cookie, true)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	logResp("zmpt_lotto bet", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "响应解析失败"}, nil
	}
	ok := int(longptNum(m["code"])) == 0
	return &PlayResult{Success: ok,
		Message: firstNonEmpty(strField(m, "message"), strField(m, "msg")),
		Prize:   fmt.Sprintf("投注 %v", arr), Raw: m}, nil
}

// Stats 尽力取余额。
func (g ZmptLottoGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	m, err := zmptLottoGet(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{}
	if d, ok := m["data"].(map[string]any); ok {
		st.Balance = firstNonZero(numField(d, "bonus"), numField(d, "user_bonus"))
	}
	return st, nil
}
