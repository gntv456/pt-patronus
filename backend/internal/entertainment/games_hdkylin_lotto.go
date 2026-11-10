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

// ── HDKylin（麒麟）双色球 union-lotto.php ──────────────────────
// 契约（2026-09 实测页面 JS /game/ssq/build/js）：POST /union-lotto.php
//
//	json {"nums":[n1..n6,b]} → {code:0,message}（红球 1-33 六个 + 蓝球 1-16 一个）
//	GET  /union-lotto.php → {code:0,data:{list:[...投注记录],...}}
//
// 每注扣魔力（奖池 UnionLottoInceptionAmount 起始 100 万）；开奖由站点定期进行。
type HdkylinLottoGame struct{}

func (HdkylinLottoGame) ID() string   { return "hdkylin_lotto" }
func (HdkylinLottoGame) Name() string { return "双色球" }

// hdkylinLottoGet GET /nodeapi/union-lotto.php（JSON 状态：投注记录/奖池；页面 fetch 封装统一加 /nodeapi 前缀）。
func hdkylinLottoGet(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, "nodeapi/union-lotto.php")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{
		"Accept":  "application/json, text/javascript, */*; q=0.01",
		"Referer": joinPath(baseURL, "union-lotto.php"),
	}, true)
	if err != nil {
		return nil, err
	}
	logResp("hdkylin_lotto state", u, resp)
	return parseRaw(resp.Body)
}

// Play（LotteryGame 兜底）= 只读状态（本期投注注数 + 奖池）。不自动投注——购注走 Bet 显式下单。
func (HdkylinLottoGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := hdkylinLottoGet(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: int(longptNum(m["code"])) == 0, Raw: m}
	n, pool := 0, ""
	if d, ok := m["data"].(map[string]any); ok {
		if lst, ok := d["list"].([]any); ok {
			n = len(lst)
		}
		pool = strOrDash(d["totalAmount"])
	}
	res.Message = fmt.Sprintf("已同步双色球：本期我的投注 %d 注，奖池 %s", n, pool)
	return res, nil
}

// hdkylinNumsRe 校验/解析选号文本："1,2,3,4,5,6|7" 或 JSON 数组文本。
var hdkylinNumsRe = regexp.MustCompile(`^(\d{1,2})\D+(\d{1,2})\D+(\d{1,2})\D+(\d{1,2})\D+(\d{1,2})\D+(\d{1,2})\D+(\d{1,2})$`)

// Bet 投一注：Selection="1,2,3,4,5,6|7"（六红一蓝）→ POST json {"nums":[...]}。
func (HdkylinLottoGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	sel := strings.TrimSpace(req.Selection)
	nums := hdkylinNumsRe.FindStringSubmatch(sel)
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
	u := joinPath(baseURL, "nodeapi/union-lotto.php")
	resp, err := hc.Do(ctx, http.MethodPost, u, payload, map[string]string{
		"Content-Type":     "application/json; charset=UTF-8",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "union-lotto.php"),
	}, cr.Cookie, true)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	logResp("hdkylin_lotto bet", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "响应解析失败"}, nil
	}
	ok := int(longptNum(m["code"])) == 0
	return &PlayResult{Success: ok,
		Message: firstNonEmpty(strField(m, "message"), strField(m, "msg")),
		Prize:   fmt.Sprintf("投注 %v", arr), Raw: m}, nil
}

// Stats 余额尽力取（nodeapi 返回 data 里通常无 bonus 字段，留空由站点余额兜底）。
func (HdkylinLottoGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	m, err := hdkylinLottoGet(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{}
	if d, ok := m["data"].(map[string]any); ok {
		st.Balance = firstNonZero(numField(d, "bonus"), numField(d, "user_bonus"))
	}
	return st, nil
}
