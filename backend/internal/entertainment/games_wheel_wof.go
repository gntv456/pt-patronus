// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── wof.php 幸运大转盘家族（HDKylin 麒麟 / UBits，wof=wheel of fortune） ────
// 契约（2026-09 实测页面 JS）：
//
//	HDKylin: GET  wof/ajax_chs.php?app=lottery_json → {rid,rname,edata,...}（奖品结果）
//	UBits:   POST wof.php?app=lottery_json  body _token=<csrf>&count=N → {rid,rname,edata}
//
// 两站同族（NexusPHP wof 插件），差异仅在端点与 CSRF：UBits 需从页面 meta[name=csrf-token]
// 取 token 附带；HDKylin 无需。count=单抽/连抽（10/20/50）。
// 余额在页面文本（麒麟「当前拥有」/ UB「U币」）。
type WofWheelGame struct{}

func (WofWheelGame) ID() string   { return "wof_wheel" }
func (WofWheelGame) Name() string { return "幸运大转盘" }

// wofPage GET wof.php 页面。
func wofPage(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (string, error) {
	u := joinPath(baseURL, "wof.php")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{"Referer": u}, true)
	if err != nil {
		return "", err
	}
	return string(resp.Body), nil
}

var wofCsrfRe = regexp.MustCompile(`csrf-token["'][^>]*content=["']([^"']+)["']`)

// WofPlay 通用抽奖：count<=1 单抽；UBits 需 csrf。
func wofPlay(ctx context.Context, hc *httpclient.Client, baseURL, cookie string, count int) (map[string]any, error) {
	if count < 1 {
		count = 1
	}
	pageBody, _ := wofPage(ctx, hc, baseURL, cookie)
	csrf := ""
	if m := wofCsrfRe.FindStringSubmatch(pageBody); m != nil {
		csrf = m[1]
	}
	// 先试 UBits 形态（POST wof.php?app=lottery_json + _token）；无 csrf 则 GET 麒麟形态。
	if csrf != "" {
		form := url.Values{}
		form.Set("_token", csrf)
		form.Set("count", fmt.Sprint(count))
		u := joinPath(baseURL, "wof.php?app=lottery_json")
		resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form.Encode()), map[string]string{
			"Content-Type":     "application/x-www-form-urlencoded",
			"X-Requested-With": "XMLHttpRequest",
			"Referer":          joinPath(baseURL, "wof.php"),
		}, cookie, true)
		if err != nil {
			return nil, err
		}
		logResp("wof_wheel lottery_json", u, resp)
		return parseRaw(resp.Body)
	}
	u := joinPath(baseURL, "wof/ajax_chs.php?app=lottery_json")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "wof.php"),
	}, true)
	if err != nil {
		return nil, err
	}
	logResp("wof_wheel chs", u, resp)
	return parseRaw(resp.Body)
}

// wofResult {rid,rname,edata} → PlayResult。
func wofResult(m map[string]any) *PlayResult {
	res := &PlayResult{Success: m != nil, Raw: m}
	if m == nil {
		res.Message = "站点响应异常"
		return res
	}
	res.Prize = firstNonEmpty(strField(m, "rname"), strField(m, "prize_text"))
	if res.Prize == "" && m["rid"] != nil {
		res.Prize = fmt.Sprintf("奖品#%v", m["rid"])
	}
	res.Message = firstNonEmpty(strField(m, "msg"), strField(m, "message"), "抽奖完成")
	return res
}

// Play 单抽。
func (WofWheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := wofPlay(ctx, hc, baseURL, cr.Cookie, 1)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	return wofResult(m), nil
}

// PlayMulti 连抽（10/20/50）。
func (WofWheelGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	if count > maxMultiCount {
		count = maxMultiCount
	}
	m, err := wofPlay(ctx, hc, baseURL, cr.Cookie, count)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	return wofResult(m), nil
}

var wofBalanceRe = regexp.MustCompile(`(?:当前拥有|余额|U币)[^0-9]{0,12}([\d,.]+)`)

// Stats 余额（wof.php 页面文本）。
func (WofWheelGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := wofPage(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{}
	if m := wofBalanceRe.FindStringSubmatch(body); m != nil {
		st.Balance = novaHDNum(m[1])
	}
	return st, nil
}
