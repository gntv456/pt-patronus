// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── PANDA 翻面夺宝（pandapt.net） ─────────────────────────
// POST /ajax_flip.php form action=…&params[…]=…：
//
//	gameState → {ret,data:{roundNumber,balance,costPerFlip,layout:[{index,tierId,flipped}]}}
//	getFlipTiers → {ret,data:[{id,name,reward,reward_type,count,color}]}（6 档合计 100 张）
//	recordFlip + params[round_number]+params[tile_index] → {ret,data:{status,balance,flipResult:{tier,tierId,reward,isSpecial,reward_type}}}
//
// task_id 只用于旁支任务系统（startTask/flipGetReward），翻格不需要。
type PandaFlipGame struct{}

func (PandaFlipGame) ID() string   { return "panda_flip" }
func (PandaFlipGame) Name() string { return "翻面夺宝" }

// pandaFlipAjax 调 ajax_flip.php（统一 form：action + params[…]），返回解析后的整 map。
func pandaFlipAjax(ctx context.Context, hc *httpclient.Client, baseURL, action, cookie string, params map[string]string) (map[string]any, error) {
	u := joinPath(baseURL, "ajax_flip.php")
	form := "action=" + action
	for k, v := range params {
		form += "&params[" + k + "]=" + v
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "flip.php"),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("panda_flip "+action, u, resp)
	return parseRaw(resp.Body)
}

// pandaFlipData 取 m["data"]（getFlipTiers 的 data 是数组，此时返回 nil）。
func pandaFlipData(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return nil
}

func (PandaFlipGame) FlipState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FlipState, error) {
	// gameState：round/balance/cost/layout
	gm, err := pandaFlipAjax(ctx, hc, baseURL, "gameState", cr.Cookie, nil)
	state := &FlipState{TotalTiles: 100, Tiers: []FlipTier{}, Tiles: []FlipTile{}, History: []GameRecord{}}
	if err == nil {
		if d := pandaFlipData(gm); d != nil {
			state.RoundNumber = int(longptNum(d["roundNumber"]))
			state.Balance = numField(d, "balance")
			state.CostPerFlip = numField(d, "costPerFlip")
			// 站点 isFlipped(idx) = allFlips.some(f => f.index===idx)：本局所有已翻格 [{index,tierId,tier}]。
			// 优先 allFlips；无则兜底 layout（按位置索引的 tierId 串：layout[i]=第 i+1 格 tierId，空=未翻）。
			if flips, ok := d["allFlips"].([]any); ok {
				for _, it := range flips {
					f, ok := it.(map[string]any)
					if !ok {
						continue
					}
					idx := int(longptNum(f["index"]))
					if idx == 0 {
						continue
					}
					state.Tiles = append(state.Tiles, FlipTile{
						Index:   idx,
						Flipped: true,
						TierID:  firstNonEmpty(strField(f, "tierId"), tierIDOf(f)),
						ByUser:  firstNonEmpty(strField(f, "user"), strField(f, "playerName"), strField(f, "player")),
					})
					state.FlippedCount++
				}
			} else if layout, ok := d["layout"].([]any); ok {
				for i, it := range layout {
					var tid string
					if s, ok := it.(string); ok {
						tid = s
					} else if m, ok := it.(map[string]any); ok {
						tid = firstNonEmpty(strField(m, "tierId"), tierIDOf(m))
					}
					if tid == "" {
						continue
					}
					state.Tiles = append(state.Tiles, FlipTile{Index: i + 1, Flipped: true, TierID: tid})
					state.FlippedCount++
				}
			}
		}
	}
	// getFlipTiers：6 档奖项
	tm, err := pandaFlipAjax(ctx, hc, baseURL, "getFlipTiers", cr.Cookie, nil)
	if err == nil {
		if arr, ok := tm["data"].([]any); ok {
			tiers := make([]FlipTier, 0, len(arr))
			for _, it := range arr {
				t, ok := it.(map[string]any)
				if !ok {
					continue
				}
				tiers = append(tiers, FlipTier{
					ID:         tierIDOf(t),
					Name:       firstNonEmpty(strField(t, "name"), pandaTierName(t)),
					Reward:     numField(t, "reward"),
					RewardType: strField(t, "reward_type"),
					Count:      int(longptNum(t["count"])),
					Color:      strField(t, "color"),
				})
			}
			if len(tiers) > 0 {
				state.Tiers = tiers
			}
		}
	}
	if state.CostPerFlip == 0 {
		state.CostPerFlip = 2000 // 页面默认 2000 魔力/次
	}
	// 本局翻牌记录：站点仅经 websocket 推送（无 HTTP 端点，初始页「暂无记录」）；
	// 但 gameState.layout 含每个已翻格 {index,tierId,user} —— 已翻格即本局记录，据此忠实重建。
	state.History = pandaFlipHistory(state.Tiles, state.Tiers)
	return state, nil
}

// pandaFlipHistory 由已翻格重建本局记录：每条 "{user} 翻了 #{index} · {tier}" + reward。
// 倒序（最近翻的在前，按 index 降序近似时间倒序）。
func pandaFlipHistory(tiles []FlipTile, tiers []FlipTier) []GameRecord {
	tierName := func(id string) (string, float64) {
		for _, t := range tiers {
			if t.ID == id {
				return t.Name, t.Reward
			}
		}
		return "", 0
	}
	out := make([]GameRecord, 0, len(tiles))
	for _, tl := range tiles {
		if !tl.Flipped {
			continue
		}
		name, reward := tierName(tl.TierID)
		who := tl.ByUser
		label := "#" + numToStr(tl.Index)
		if who != "" {
			label = who + " 翻了 #" + numToStr(tl.Index)
		}
		if name != "" {
			label += " · " + name
		}
		out = append(out, GameRecord{PrizeName: label, PrizeType: tl.TierID, Amount: reward})
	}
	// 倒序：index 大的（通常后翻）在前
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// tierIDOf 兼容 id/tid/tierId 多键（layout 项与 tier 配置键名可能不同）。
func tierIDOf(m map[string]any) string {
	if v := firstNonEmpty(strField(m, "id"), strField(m, "tid"), strField(m, "tierId")); v != "" {
		return v
	}
	if v, ok := m["id"]; ok {
		return numToStr(v)
	}
	return ""
}

// pandaTierName getFlipTiers 项可能无 name，按 reward_type + reward 兜底描述。
func pandaTierName(m map[string]any) string {
	r := numField(m, "reward")
	if r <= 0 {
		return ""
	}
	return formatFloat(r) + "魔力"
}

func (PandaFlipGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	tile := strings.TrimSpace(req.Selection)
	if tile == "" || tile == "0" {
		tile = "1"
	}
	// round_number 优先用前端传入（来自 FlipState），缺则现取 gameState。
	round := strings.TrimSpace(req.Extra["round_number"])
	if round == "" {
		if gm, err := pandaFlipAjax(ctx, hc, baseURL, "gameState", cr.Cookie, nil); err == nil {
			if d := pandaFlipData(gm); d != nil {
				round = numToStr(d["roundNumber"])
			}
		}
	}
	if round == "" {
		round = "1"
	}
	m, err := pandaFlipAjax(ctx, hc, baseURL, "recordFlip", cr.Cookie, map[string]string{
		"round_number": round,
		"tile_index":   tile,
	})
	if err != nil {
		return nil, err
	}
	res := &PlayResult{Success: false, Raw: m}
	if d := pandaFlipData(m); d != nil {
		// 站点成功：ret==0 && data.status==1。status≠1 多为「该格已被别人翻开」。
		ok := int(longptNum(m["ret"])) == 0 && (int(longptNum(d["status"])) == 1 || d["status"] == nil)
		res.Success = ok
		res.Balance = numField(d, "balance")
		if fr, ok := d["flipResult"].(map[string]any); ok {
			// flipResult.tier 可能是对象 {name,reward_type,reward} 也可能是字符串
			tierObj, _ := fr["tier"].(map[string]any)
			name := strField(fr, "tier")
			if name == "" && tierObj != nil {
				name = strField(tierObj, "name")
			}
			if name == "" {
				name = strField(fr, "tierId")
			}
			reward := numField(fr, "reward")
			if reward == 0 && tierObj != nil {
				reward = numField(tierObj, "reward")
			}
			rtype := strField(fr, "reward_type")
			if rtype == "" && tierObj != nil {
				rtype = strField(tierObj, "reward_type")
			}
			res.Prize = pandaFlipPrizeText(name, reward, rtype)
			if rtype == "none" || (name == "" && reward == 0) {
				res.Message = "未中奖，继续加油"
			}
		}
	}
	if res.Success && res.Prize == "" {
		res.Prize = "中奖"
	}
	if !res.Success && res.Message == "" {
		// 该格已被翻开等：提示后前端会 load() 刷新出最新格子状态
		res.Message = firstNonEmpty(strField(m, "msg"), "该格可能已被翻开，请刷新后再试")
	}
	return res, nil
}

// pandaFlipPrizeText 按奖励类型组装奖项描述（对齐站点 JS：coins +魔力 / medal 勋章 / vip 限时VIP / none 未中奖）。
func pandaFlipPrizeText(name string, reward float64, rtype string) string {
	switch rtype {
	case "none":
		if name != "" {
			return name
		}
		return ""
	case "medal":
		return firstNonEmpty(name, "中奖") + " 获得勋章"
	case "vip":
		return firstNonEmpty(name, "中奖") + " 获得限时VIP"
	default:
		if name == "" && reward <= 0 {
			return ""
		}
		if name == "" {
			return "+" + formatFloat(reward) + "魔力"
		}
		if reward > 0 {
			return name + " +" + formatFloat(reward) + "魔力"
		}
		return name
	}
}

func (PandaFlipGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return PandaFlipGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{Selection: "1"})
}

// Stats 余额兜底（hero 用；FlipState 提供更全数据）。
func (PandaFlipGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	gm, err := pandaFlipAjax(ctx, hc, baseURL, "gameState", cr.Cookie, nil)
	if err != nil {
		return &StatsResult{}, nil
	}
	if d := pandaFlipData(gm); d != nil {
		return &StatsResult{Balance: numField(d, "balance"), CostPerPlay: numField(d, "costPerFlip")}, nil
	}
	return &StatsResult{}, nil
}
