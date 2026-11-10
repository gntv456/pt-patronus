// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── PANDA 猜大小（pandapt.net） ───────────────────────────
// /ajax.php POST action=+params[k]=，信封 {ret,data,msg}（ret 0=ok）。无 csrf。
// reloadData 一次拿大头（my_bonus/timeLeft/isBettingOpen/currentGame/last_result 走势/my_list/other_list），
// 其余 task_list/getMyItems/getActiveRedPackets/getLastMsg 各一次轻取。
// 注：熊猫排行（本月赌神/今日赌王…）SSR 烘焙在 bigorsmall.php 页面 Vue data 里，reloadData 可能不含 →
// 此处尽力从 reloadData 取 god/poor/todayRanking/loseRanking 键，缺失则留空（待真实校准）。
type PandaBigSmallGame struct{}

func (PandaBigSmallGame) ID() string   { return "panda_bigsmall" }
func (PandaBigSmallGame) Name() string { return "猜大小" }

// pandaBigSmallAjax 调 /ajax.php（统一 form：action + params[…]），返回完整信封 map。
func pandaBigSmallAjax(ctx context.Context, hc *httpclient.Client, baseURL, action, cookie string, params map[string]string) (map[string]any, error) {
	u := joinPath(baseURL, "ajax.php")
	form := "action=" + action
	for k, v := range params {
		form += "&params[" + k + "]=" + url.QueryEscape(v)
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "bigorsmall.php"),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("panda_bigsmall "+action, u, resp)
	return parseRaw(resp.Body)
}

func pandaBSData(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return nil
}
func pandaBSArr(m map[string]any) []any {
	if a, ok := m["data"].([]any); ok {
		return a
	}
	return nil
}

// pandaStr 取多候选键的首个非空字符串（熊猫 JSON 键名不确定，多键兜底）。
func pandaStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := strField(m, k); v != "" {
			return v
		}
	}
	return ""
}

// numFieldMulti 取多候选键的首个非零数值。
func numFieldMulti(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v := numField(m, k); v != 0 {
			return v
		}
	}
	return 0
}

// pandaResultStr 结果归一为 big/small（1或"大"→big，2或"小"→small）。
func pandaResultStr(m map[string]any) string {
	r := pandaStr(m, "result", "type")
	if r == "" {
		n := numToStr(m["result"])
		if n == "1" {
			return "big"
		}
		if n == "2" {
			return "small"
		}
		return ""
	}
	switch {
	case r == "1" || r == "big" || strings.Contains(r, "大"):
		return "big"
	case r == "2" || r == "small" || strings.Contains(r, "小"):
		return "small"
	}
	return r
}

// pandaJoinBetType 解析 currentGame.join（"大 : 1000"/"小 : 500"/"未参与"）→ bet_type。
func pandaJoinBetType(join string) string {
	s := strings.TrimSpace(join)
	if strings.HasPrefix(s, "大") {
		return "big"
	}
	if strings.HasPrefix(s, "小") {
		return "small"
	}
	return ""
}

// pandaJoinAmount 从 join 串尾抠金额（"大 : 1,000" → 1000）。
func pandaJoinAmount(join string) float64 {
	s := strings.ReplaceAll(join, ",", "")
	f, _ := strconv.ParseFloat(firstGroup([]byte(s), `([\d.]+)\s*$`), 64)
	return f
}

// pandaTrend 由 last_result（二维数组，1=大）算 40场/最近10场 统计 + 走势点阵。
func pandaTrend(v any) (BigSmallTrend, BigSmallTrend, []string) {
	flat := make([]any, 0)
	if arr, ok := v.([]any); ok {
		for _, row := range arr {
			if r, ok := row.([]any); ok {
				flat = append(flat, r...)
			} else {
				flat = append(flat, row)
			}
		}
	}
	if len(flat) == 0 {
		return BigSmallTrend{}, BigSmallTrend{}, []string{}
	}
	bigAll := 0
	for _, x := range flat {
		if numToStr(x) == "1" {
			bigAll++
		}
	}
	total := len(flat)
	t40 := BigSmallTrend{BigCount: bigAll, SmallCount: total - bigAll}
	if total > 0 {
		t40.BigPct = bigAll * 100 / total
		t40.SmallPct = 100 - t40.BigPct
	}
	n := 10
	if n > total {
		n = total
	}
	big10 := 0
	for _, x := range flat[:n] {
		if numToStr(x) == "1" {
			big10++
		}
	}
	t10 := BigSmallTrend{BigCount: big10, SmallCount: n - big10}
	if n > 0 {
		t10.BigPct = big10 * 100 / n
		t10.SmallPct = 100 - t10.BigPct
	}
	rr := make([]string, 0, total)
	for _, x := range flat {
		if numToStr(x) == "1" {
			rr = append(rr, "big")
		} else {
			rr = append(rr, "small")
		}
	}
	return t40, t10, rr
}

func pandaBettors(v any) []BigSmallBetSpot {
	arr, _ := v.([]any)
	out := make([]BigSmallBetSpot, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallBetSpot{
			Username: pandaStr(m, "username", "name"),
			Amount:   numFieldMulti(m, "amount", "bonus", "bet_amount"),
		})
	}
	return out
}

func pandaMyRecords(v any) []BigSmallRecord {
	arr, _ := v.([]any)
	out := make([]BigSmallRecord, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallRecord{
			GameNo:      pandaStr(m, "game_no", "gameNo", "id"),
			StartTime:   pandaStr(m, "start_time", "startTime"),
			EndTime:     pandaStr(m, "end_time", "endTime"),
			PlayerCount: int(longptNum(m["player_count"])),
			TotalBet:    numFieldMulti(m, "total_bet", "total_bet_amount"),
			Result:      pandaResultStr(m),
			MyBetType:   pandaJoinBetType(pandaStr(m, "my_bet_type", "my_bet", "join")),
			MyBet:       numFieldMulti(m, "my_bet", "my_amount", "bet"),
			MyResult:    numFieldMulti(m, "my_result", "result_amount", "net"),
		})
	}
	return out
}

func pandaAllRecords(v any) []BigSmallRecord {
	arr, _ := v.([]any)
	out := make([]BigSmallRecord, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		rec := BigSmallRecord{
			GameNo:    pandaStr(m, "game_no", "gameNo", "id"),
			StartTime: pandaStr(m, "start_time", "startTime"),
			EndTime:   pandaStr(m, "end_time", "endTime"),
			Result:    pandaResultStr(m),
		}
		if bets := pandaStr(m, "bets", "bet_info"); bets != "" {
			rec.Bets = bets
		}
		out = append(out, rec)
	}
	return out
}

// pandaRankings 从 reloadData 取 god/poor/todayRanking/loseRanking（SSR 数据若在 reload 响应里则填）。
func pandaRankings(d map[string]any) []BigSmallRank {
	tabs := []struct{ key, title, amtKey, cntKey string }{
		{"god", "本月赌神", "total_result", "total_count"},
		{"todayRanking", "今日赌王", "today_net_profit", "today_bets"},
		{"poor", "本月慈善", "total_result", "total_count"},
		{"loseRanking", "今日慈善", "today_net_profit", "today_bets"},
	}
	out := make([]BigSmallRank, 0, 4)
	for _, t := range tabs {
		var arr []any
		if a, ok := d[t.key].([]any); ok {
			arr = a
		} else if om, ok := d[t.key].(map[string]any); ok {
			arr = []any{om} // god/poor 单对象
		}
		if len(arr) == 0 {
			continue
		}
		r := BigSmallRank{Key: t.key, Title: t.title, Entries: make([]BigSmallRankEntry, 0, len(arr))}
		for _, it := range arr {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			r.Entries = append(r.Entries, BigSmallRankEntry{
				Username: pandaStr(m, "username", "name"),
				Amount:   numFieldMulti(m, t.amtKey, "amount"),
				Count:    int(longptNum(m[t.cntKey])),
			})
		}
		out = append(out, r)
	}
	return out
}

func pandaTasks(arr []any) []BigSmallTask {
	out := make([]BigSmallTask, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		status := "idle"
		switch int(longptNum(m["task_status"])) {
		case 1:
			status = "doing"
		case 3:
			status = "done"
		}
		if status == "idle" && boolField(m, "can_reward") {
			status = "rewardable"
		}
		out = append(out, BigSmallTask{
			ID:           int(longptNum(m["task_id"])),
			Title:        pandaStr(m, "title", "name"),
			Desc:         pandaStr(m, "desc", "description"),
			Reward:       pandaStr(m, "reward", "reward_name"),
			ProgressText: pandaStr(m, "progress_text", "progress"),
			Progress:     int(longptNum(m["progress"])),
			Status:       status,
		})
	}
	return out
}

func pandaProps(arr []any) []BigSmallProp {
	out := make([]BigSmallProp, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallProp{
			ID:       pandaStr(m, "id", "unique_tag"),
			Tag:      pandaStr(m, "unique_tag", "tag"),
			Name:     pandaStr(m, "title", "name"),
			Desc:     pandaStr(m, "desc", "description"),
			Image:    pandaStr(m, "image", "icon"),
			Quantity: int(longptNum(m["count"])),
			Usable:   true, // 熊猫道具独立 useItem 使用
		})
	}
	return out
}

func pandaRedpackets(arr []any) []BigSmallRedpacket {
	out := make([]BigSmallRedpacket, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallRedpacket{
			ID:    pandaStr(m, "baseId", "id"),
			Total: numField(m, "total"),
			Count: int(longptNum(m["count"])),
		})
	}
	return out
}

func pandaChat(arr []any) []BigSmallChat {
	out := make([]BigSmallChat, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallChat{
			ID:       int(longptNum(m["id"])),
			Username: pandaStr(m, "username", "name"),
			Message:  pandaStr(m, "content", "message"),
			Time:     pandaStr(m, "timestamp", "time"),
		})
	}
	return out
}
