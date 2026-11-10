// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 蟹黄堡 快三（crabpt.vip，fast-three.php）────────────────────────
// 契约（2026-09 实测 /nodeapi 前缀 + build/js/LcSNzvVk.js）：
//
//	GET  /nodeapi/fast-three        → {code:0,data:{history,next,amount:{totalBetAmount,totalWinningAmount}}}
//	GET  /nodeapi/fast-three/config → 赔率与单价等
//	POST /nodeapi/fast-three        json {chosen_character:"皮卡丘|杰尼龟|妙蛙种子"}（单次投注）
//	POST /nodeapi/fast-three/batch  json {num:N[, chosen_character]}（批量投注）
//
// 货币为蟹币；betType 1=固定投注 2=批量固定 3=批量随机。投注属花钱操作，仅 Bet 显式触发。
type CrabFastThreeGame struct{}

func (CrabFastThreeGame) ID() string   { return "crab_fastthree" }
func (CrabFastThreeGame) Name() string { return "快三" }

// crabFastThreeGet 拉取快三状态（下期/历史/累计投注与中奖）。
func crabFastThreeGet(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, "nodeapi/fast-three")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{
		"Accept":  "application/json, text/javascript, */*; q=0.01",
		"Referer": joinPath(baseURL, "fast-three.php"),
	}, true)
	if err != nil {
		return nil, err
	}
	logResp("crab_fastthree state", u, resp)
	return parseRaw(resp.Body)
}

// Play 只读同步：下期开奖时间/累计投注中奖（不自动下注）。
func (CrabFastThreeGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := crabFastThreeGet(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: int(longptNum(m["code"])) == 0, Raw: m}
	if d, ok := m["data"].(map[string]any); ok {
		next := ""
		if lst, ok := d["next"].([]any); ok && len(lst) > 0 {
			next = fmt.Sprintf("，下期 %v", lst[0])
		}
		if amt, ok := d["amount"].(map[string]any); ok {
			res.Message = fmt.Sprintf("已同步快三：累计投注 %v / 累计中奖 %v%s",
				strOrDash(amt["totalBetAmount"]), strOrDash(amt["totalWinningAmount"]), next)
			return res, nil
		}
	}
	res.Message = firstNonEmpty(strField(m, "message"), "已同步快三状态")
	return res, nil
}

// strOrDash nil 显示 -，数字原样（避免大数被格式化成科学计数法）。
func strOrDash(v any) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		if strings.TrimSpace(x) == "" {
			return "-"
		}
		return x
	default:
		s := fmt.Sprintf("%v", x)
		if strings.TrimSpace(s) == "" {
			return "-"
		}
		return s
	}
}

// crabFastThreeCharacters 可选角色（页面三个固定选项）。
var crabFastThreeCharacters = map[string]string{
	"皮卡丘": "魔蛆", "杰尼龟": "痞老板", "妙蛙种子": "章鱼哥",
}

// Bet 投注：Selection=角色名（皮卡丘/杰尼龟/妙蛙种子），Amount=1 走单注端点，>1 走批量 num=N。
func (CrabFastThreeGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	character := strings.TrimSpace(req.Selection)
	name, ok := crabFastThreeCharacters[character]
	if !ok {
		return &PlayResult{Success: false, Message: "请选择角色：皮卡丘 / 杰尼龟 / 妙蛙种子"}, nil
	}
	// 批量端点必须带 num，缺省请求会被站点拒绝/错投，直接在前置校验拦下。
	if req.Amount <= 0 {
		return &PlayResult{Success: false, Message: "投注局数须 > 0（1=单注，>1 为批量）"}, nil
	}
	u := joinPath(baseURL, "nodeapi/fast-three")
	payload := map[string]any{"chosen_character": character}
	if req.Amount > 1 {
		u = joinPath(baseURL, "nodeapi/fast-three/batch")
		payload["num"] = int(req.Amount)
	}
	body, _ := json.Marshal(payload)
	resp, err := hc.Do(ctx, http.MethodPost, u, body, map[string]string{
		"Content-Type":     "application/json; charset=UTF-8",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "fast-three.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("crab_fastthree bet", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "响应解析失败"}, nil
	}
	okRes := int(longptNum(m["code"])) == 0
	msg := firstNonEmpty(strField(m, "message"), strField(m, "msg"))
	if !okRes && msg == "" {
		msg = "投注失败"
	}
	return &PlayResult{Success: okRes, Message: msg, Prize: name, Raw: m}, nil
}
