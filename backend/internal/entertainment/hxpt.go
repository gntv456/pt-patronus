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

// 好学（hxpt.org）娱乐游戏，契约来自站点源码 jgg.php / magic_scratch.php。

// JggGame 九宫格（magic grid）：POST jgg.php action=spin。
// 响应 {success, new_balance, message, prize_name?, prize_value?}。
type JggGame struct{}

func (JggGame) ID() string   { return "jgg" }
func (JggGame) Name() string { return "九宫格" }

func (JggGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	url := joinPath(baseURL, "jgg.php")
	resp, err := hc.PostFormAjax(ctx, url, cr.Cookie, "action=spin", baseURL, true)
	if err != nil {
		return nil, err
	}
	logResp("jgg", url, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return nil, err
	}
	res := &PlayResult{
		Success: boolField(m, "success"),
		Message: strField(m, "message"),
		Balance: numField(m, "new_balance"),
		Raw:     m,
	}
	// 奖品在嵌套 prize 对象里：{id,name,icon,color,type,value}；兼容扁平 prize_name/prize_value。
	if p, ok := m["prize"].(map[string]any); ok {
		res.PrizeID = int(numField(p, "id"))
		res.Prize = strField(p, "name")
		res.Amount = numField(p, "value")
	}
	if res.Prize == "" {
		res.Prize = strField(m, "prize_name")
	}
	if res.Amount == 0 {
		res.Amount = numField(m, "prize_value")
	}
	return res, nil
}

// Stats 九宫格统计（get_stats）：今日次数 / 上限 / 单次消耗。
func (JggGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	resp, err := hc.PostFormAjax(ctx, joinPath(baseURL, "jgg.php"), cr.Cookie, "action=get_stats", baseURL, true)
	if err != nil {
		return nil, err
	}
	m, err := parseRaw(resp.Body)
	if err != nil {
		return nil, err
	}
	return &StatsResult{
		TodaySpins:  int(numField(m, "today_spins")),
		MaxDaily:    int(numField(m, "max_daily_spins")),
		CostPerPlay: numField(m, "cost_per_spin"),
	}, nil
}

// PlayMulti 九宫格连抽：POST jgg/magic_grid_bulk.php bulk_count=N（站点仅接受 10/20/50/100）。
// 响应 {success, results:[{prize,message}], prize_stats:{name:count}, total_cost, new_balance}。
// 每次结果聚合进 PlayResult.Prizes（前端按 prize_name 分组统计，等价于站点 prize_stats）。
func (JggGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count != 10 && count != 20 && count != 50 && count != 100 {
		return &PlayResult{Success: false, Message: "连抽次数仅支持 10/20/50/100"}, nil
	}
	url := joinPath(baseURL, "jgg/magic_grid_bulk.php")
	referer := joinPath(baseURL, "jgg.php")
	resp, err := hc.PostFormAjax(ctx, url, cr.Cookie, fmt.Sprintf("bulk_count=%d", count), referer, true)
	if err != nil {
		return nil, err
	}
	logResp("jgg bulk", url, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return nil, err
	}
	res := &PlayResult{
		Success: boolField(m, "success"),
		Message: strField(m, "message"),
		Balance: numField(m, "new_balance"),
		Raw:     m,
	}
	if !res.Success {
		return res, nil
	}
	if res.Message == "" {
		res.Message = fmt.Sprintf("%d 连抽完成", count)
	}
	if results, ok := m["results"].([]any); ok {
		res.Prizes = make([]PrizeOutcome, 0, len(results))
		for _, r := range results {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			res.Prizes = append(res.Prizes, jggOutcome(rm["prize"]))
		}
	}
	return res, nil
}

// History 九宫格抽奖记录：POST jgg.php action=get_history → {history:[{prize_name,prize_type,prize_value,cost,created_at}]}。
// prize_value 为 PHP serialize 标量（如 i:10737418240; / d:1.0;），按类型还原为可读量。
func (JggGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	url := joinPath(baseURL, "jgg.php")
	resp, err := hc.PostFormAjax(ctx, url, cr.Cookie, "action=get_history", baseURL, true)
	if err != nil {
		return nil, err
	}
	logResp("jgg history", url, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return nil, err
	}
	items, _ := m["history"].([]any)
	out := make([]GameRecord, 0, len(items))
	for _, it := range items {
		im, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ptype := strField(im, "prize_type")
		amt, _ := jggPrizeValue(phpSerializedNum(strField(im, "prize_value")), ptype)
		out = append(out, GameRecord{
			PrizeName: strField(im, "prize_name"),
			PrizeType: ptype,
			Amount:    amt,
			Cost:      numField(im, "cost"),
			CreatedAt: strField(im, "created_at"),
		})
	}
	return out, nil
}

// jggOutcome 把站点奖品对象 {id,name,icon,color,type,value} 转成 PrizeOutcome（连抽聚合用）。
func jggOutcome(raw any) PrizeOutcome {
	pm, _ := raw.(map[string]any)
	if pm == nil {
		return PrizeOutcome{}
	}
	ptype := strField(pm, "type")
	amt, unit := jggPrizeValue(numField(pm, "value"), ptype)
	return PrizeOutcome{
		PrizeID:   int(numField(pm, "id")),
		PrizeName: strField(pm, "name"),
		PrizeType: ptype,
		Amount:    amt,
		UnitName:  unit,
		IsWin:     ptype != "nothing",
	}
}

// jggPrizeValue 按奖品类型把原始 value 换算成人类可读量（对齐站点 jgg.php formatPrizeValue）。
// upload/download 为字节→GB；vip/noad 为秒→天；invite 为个数；其余原值。
func jggPrizeValue(value float64, ptype string) (amount float64, unit string) {
	switch ptype {
	case "upload", "download":
		return value / 1024 / 1024 / 1024, "GB"
	case "vip", "noad":
		return value / 86400, "天"
	case "invite", "tmp_invite":
		return value, "个"
	case "medal":
		return value, ""
	}
	return value, ""
}

// phpSerializedNum 解析 PHP serialize 的标量值（i:N; / d:N.N;），否则按普通数字解析。
// 历史记录里 prize_value 形如 "i:10737418240;"（10GB 上传量字节值）。
func phpSerializedNum(s string) float64 {
	if g := findGroups([]byte(s), `^[id]:(-?[0-9.]+);`); len(g) >= 2 {
		var f float64
		_, _ = fmt.Sscanf(g[1], "%f", &f)
		return f
	}
	var f float64
	_, _ = fmt.Sscanf(s, "%f", &f)
	return f
}

// MagicScratchGame 魔力刮刮乐：两步 —— buy 买卡（返回 HTML，内嵌 currentScratchId）→ scratch 刮开（JSON 得 prize）→ 中奖 settle。
type MagicScratchGame struct{}

func (MagicScratchGame) ID() string   { return "magic_scratch" }
func (MagicScratchGame) Name() string { return "魔力刮刮乐" }

func (MagicScratchGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	endpoint := joinPath(baseURL, "magic_scratch.php")
	referer := endpoint // 具体游戏页 Referer（对齐 Kotlin 版，非站点根）
	cookie := cr.Cookie

	// 1. 买卡：站点返回 HTML（内嵌 currentScratchId=N），用正则抠 id，不能当 JSON 解析。
	buyURL := endpoint + "?action=buy"
	buyResp, err := hc.PostFormAjax(ctx, buyURL, cookie, "", referer, true)
	if err != nil {
		return nil, err
	}
	logResp("magic_scratch buy", buyURL, buyResp)
	scratchID := firstGroup(buyResp.Body, `currentScratchId\s*=\s*(\d+)`)
	if scratchID == "" {
		scratchID = firstGroup(buyResp.Body, `scratch_id[= ](\d+)`)
	}
	if scratchID == "" {
		// 买卡失败（余额不足等）：从 HTML 抠提示文本。
		msg := strings.TrimSpace(stripTags(firstGroup(buyResp.Body, `(?s)<div[^>]*font-weight:\s*bold[^>]*>(.*?)</div>`)))
		return &PlayResult{Success: false, Message: msg}, nil
	}

	// 2. 刮开：返回 JSON {success, msg, prize, prize_level, scratch_id}。
	scratchURL := endpoint + "?action=scratch"
	scratchResp, err := hc.PostFormAjax(ctx, scratchURL, cookie, "scratch_id="+scratchID, referer, true)
	if err != nil {
		return nil, err
	}
	logResp("magic_scratch scratch", scratchURL, scratchResp)
	m, err := parseRaw(scratchResp.Body)
	if err != nil {
		return nil, err
	}
	res := &PlayResult{
		Success: boolField(m, "success"),
		Message: strField(m, "msg"),
		Amount:  numField(m, "prize"),
		Prize:   strField(m, "prize_level"),
		Raw:     m,
	}

	// 3. 中奖结算（best-effort）。
	if res.Success && res.Amount > 0 {
		settleURL := endpoint + "?action=settle&scratch_id=" + scratchID
		if _, err := hc.GetWithHeaders(ctx, settleURL, cookie, map[string]string{"Referer": referer}, true); err == nil && res.Message == "" {
			res.Message = fmt.Sprintf("获得 %d 火花，已自动结算", int64(res.Amount))
		}
	}
	return res, nil
}

// Stats 魔力刮刮乐统计：余额 + 今日剩余/上限（GET magic_scratch.php HTML 抠，对齐 Kotlin loadStats）。
func (MagicScratchGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	pageURL := joinPath(baseURL, "magic_scratch.php")
	resp, err := hc.GetWithHeaders(ctx, pageURL, cr.Cookie, map[string]string{"Referer": pageURL}, true)
	if err != nil {
		return nil, err
	}
	balStr := strings.ReplaceAll(firstGroup(resp.Body, `(?s)当前的.*?余额:.*?>([\d,.]+)</span>`), ",", "")
	balance, _ := strconv.ParseFloat(balStr, 64)
	limit, remaining := 20, 0
	if g := findGroups(resp.Body, `今日剩余刮奖次数:[^\d]*(\d+)/(\d+)`); len(g) >= 3 {
		remaining, _ = strconv.Atoi(g[1])
		limit, _ = strconv.Atoi(g[2])
	}
	return &StatsResult{
		Balance:     balance,
		TodaySpins:  limit - remaining,
		MaxDaily:    limit,
		CostPerPlay: 5000,
	}, nil
}
