// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 魔力值刮刮乐 ────────────────────────────────────────────

// NovaHDScratchGame 魔力值刮刮乐：buy(POST→HTML 抠 scratch_id) → scratch(POST→JSON) → settle。
// 源码配置 cost=5000, daily_limit=5, 6 档奖品。
type NovaHDScratchGame struct{}

func (NovaHDScratchGame) ID() string   { return "novahd_scratch" }
func (NovaHDScratchGame) Name() string { return "魔力值刮刮乐" }

func (NovaHDScratchGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	endpoint := joinPath(baseURL, "magic_scratch.php")
	referer := endpoint

	// 1. 购买资格：POST 返 HTML，内嵌 currentScratchId=N。
	buyResp, err := hc.PostFormAjax(ctx, endpoint+"?action=buy", cr.Cookie, "", referer, true)
	if err != nil {
		return nil, err
	}
	logResp("novahd_scratch buy", endpoint, buyResp)
	scratchID := firstGroup(buyResp.Body, `currentScratchId\s*=\s*(\d+)`)
	if scratchID == "" {
		scratchID = firstGroup(buyResp.Body, `scratch_id[=:" ]+(\d+)`)
	}
	if scratchID == "" {
		// 买卡失败（余额不足/达上限）：抠提示文本。
		msg := strings.TrimSpace(stripTags(firstGroup(buyResp.Body, `(?s)<div class="message"[^>]*>(.*?)</div>`)))
		if msg == "" {
			msg = "购买刮奖资格失败（余额不足或今日已达上限）"
		}
		return &PlayResult{Success: false, Message: msg}, nil
	}

	// 2. 刮开：POST scratch_id 返 JSON {success, prize, prize_level, scratch_id}。
	scratchResp, err := hc.PostFormAjax(ctx, endpoint+"?action=scratch", cr.Cookie, "scratch_id="+scratchID, referer, true)
	if err != nil {
		return nil, err
	}
	logResp("novahd_scratch scratch", endpoint, scratchResp)
	m, err := parseRaw(scratchResp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "解析刮奖结果失败"}, nil
	}
	res := &PlayResult{
		Success: boolField(m, "success"),
		Amount:  numField(m, "prize"),
		Prize:   strField(m, "prize_level"),
		Raw:     m,
	}
	if res.Success && res.Amount > 0 {
		res.Message = fmt.Sprintf("%s：获得 %g 魔力值", res.Prize, res.Amount)
	} else if res.Success {
		res.Prize, res.Message = "未中奖", "很遗憾，未中奖"
	}

	// 3. 结算（best-effort）：GET settle，把中奖魔力入账。
	if res.Success && res.Amount > 0 {
		_, _ = hc.GetWithHeaders(ctx, endpoint+"?action=settle&scratch_id="+scratchID, cr.Cookie, map[string]string{"Referer": referer}, true)
	}
	return res, nil
}

// Stats 余额 + 今日剩余/上限（抠 magic_scratch.php 页面）。
func (NovaHDScratchGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "magic_scratch.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	balance := novaHDNum(firstGroup(resp.Body, `余额[^0-9-]*<[^>]*>\s*([\d,.]+)`))
	if balance == 0 {
		balance = novaHDNum(firstGroup(resp.Body, `余额[^0-9-]*([\d,.]+)`))
	}
	limit, remaining := 5, 5
	if g := findGroups(resp.Body, `(\d+)\s*/\s*(\d+)`); len(g) >= 3 { // 今日剩余 remaining/limit
		remaining, _ = strconv.Atoi(strings.TrimSpace(g[1]))
		limit, _ = strconv.Atoi(strings.TrimSpace(g[2]))
	}
	return &StatsResult{
		Balance:     balance,
		TodaySpins:  limit - remaining,
		MaxDaily:    limit,
		CostPerPlay: 5000,
	}, nil
}

// Prizes 6 档中奖规则（源码 magic_scratch.php 11-18 行常量）。
func (NovaHDScratchGame) Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error) {
	return []PrizeDef{
		{ID: 0, Name: "未中奖（无奖励）", Type: "99"},
		{ID: 5, Name: "五等奖 +500 魔力值", Type: "01"},
		{ID: 4, Name: "四等奖 +1000 魔力值", Type: "01"},
		{ID: 3, Name: "三等奖 +3000 魔力值", Type: "01"},
		{ID: 2, Name: "二等奖 +5000 魔力值", Type: "01"},
		{ID: 1, Name: "一等奖 +100000 魔力值", Type: "01"},
	}, nil
}

// History 刮奖记录（页面渲染结构待真实校准，暂返回空）。
func (NovaHDScratchGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	return []GameRecord{}, nil
}

// ── 幸运抽奖 ────────────────────────────────────────────────

// NovaHDLuckyGame 幸运抽奖：POST fortune-wheel-spin.php count=N。
// 响应 {success, results:[{status,type,value,unit,name?}]}，奖品 type：bonus/attendance_card/upload/nothing。
type NovaHDLuckyGame struct{}

func (NovaHDLuckyGame) ID() string   { return "novahd_lucky" }
func (NovaHDLuckyGame) Name() string { return "幸运抽奖" }

func (NovaHDLuckyGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return NovaHDLuckyGame{}.spin(ctx, hc, baseURL, cr, 1)
}

// PlayMulti 连抽（count=10/20/50）。
func (NovaHDLuckyGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	if count > maxMultiCount {
		count = maxMultiCount
	}
	return NovaHDLuckyGame{}.spin(ctx, hc, baseURL, cr, count)
}

func (NovaHDLuckyGame) spin(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	u := joinPath(baseURL, "fortune-wheel-spin.php")
	referer := joinPath(baseURL, "fortune-wheel.php")
	resp, err := hc.PostFormAjax(ctx, u, cr.Cookie, fmt.Sprintf("count=%d", count), referer, true)
	if err != nil {
		return nil, err
	}
	logResp("novahd_lucky spin", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	if !boolField(m, "success") {
		return &PlayResult{Success: false, Message: strField(m, "message")}, nil
	}
	results, _ := m["results"].([]any)
	out := &PlayResult{Success: true, Prizes: make([]PrizeOutcome, 0, len(results))}
	var sum float64
	for _, r := range results {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		o := novaHDLuckyOutcome(rm)
		out.Prizes = append(out.Prizes, o)
		if o.IsWin && o.PrizeType == "bonus" { // 顶层 Amount 仅累加魔力值收益（补签卡/上传 value 是其他单位）
			sum += o.Amount
		}
	}
	out.Amount = sum
	for _, o := range out.Prizes {
		if o.IsWin {
			out.PrizeID, out.Prize = o.PrizeID, o.PrizeName
			break
		}
	}
	if out.Prize == "" {
		if len(out.Prizes) > 0 {
			out.Prize, out.Message = "未中奖", "很遗憾，本次未中奖"
		} else {
			out.Message = "抽奖完成"
		}
	}
	return out, nil
}

// novaHDLuckyOutcome 把单个奖品结果转 PrizeOutcome。
func novaHDLuckyOutcome(rm map[string]any) PrizeOutcome {
	typ := strField(rm, "type")
	name := firstNonEmpty(strField(rm, "name"), strField(rm, "prize_name"))
	if name == "" {
		switch typ {
		case "bonus":
			name = "魔力值"
		case "attendance_card":
			name = "补签卡"
		case "upload":
			name = "上传量"
		case "nothing":
			name = "谢谢参与"
		default:
			name = "奖品"
		}
	}
	// 站点可能省略 status 只回 type+value：仅以 typ 判定中奖（nothing/空=未中）。
	isWin := typ != "nothing" && typ != ""
	return PrizeOutcome{
		PrizeID:   int(numField(rm, "id")),
		PrizeName: name,
		PrizeType: typ,
		Amount:    numField(rm, "value"),
		UnitName:  strField(rm, "unit"),
		IsWin:     isWin,
	}
}

// Stats 抽奖信息（余额/每次消耗/今日/免费，抠 fortune-wheel.php 页面，文本以真实页面为准）。
func (NovaHDLuckyGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "fortune-wheel.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	body := resp.Body
	res := &StatsResult{}
	// 余额（魔力值，页面可能不直接展示，抠首个大数兜底）。
	res.Balance = novaHDNum(firstGroup(body, `魔力[^0-9<]{0,6}([\d,.]+)`))
	// 每次消耗。
	if v := firstGroup(body, `消耗[^0-9]{0,6}([\d,.]+)`); v != "" {
		res.CostPerPlay = novaHDNum(v)
	}
	// 今日已抽 N/M。
	if g := findGroups(body, `(\d+)\s*/\s*(\d+)`); len(g) >= 3 {
		res.TodaySpins, _ = strconv.Atoi(strings.TrimSpace(g[1]))
		res.MaxDaily, _ = strconv.Atoi(strings.TrimSpace(g[2]))
	}
	return res, nil
}

// Prizes 奖品列表（由 getPrizeGrades 渲染，结构待真实校准，暂返回空）。
func (NovaHDLuckyGame) Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error) {
	return []PrizeDef{}, nil
}

// History 抽奖记录（getUserDrawRecords 渲染，待真实校准，暂返回空）。
func (NovaHDLuckyGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	return []GameRecord{}, nil
}
