// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 思齐（si-qi.xyz）种菜赚魔力 ────────────────────────────
// 契约（2026-09 实测页面 JS）：
//
//	GET  plant_game.php?action=fetch                     → 农场全景 {success,inventory,lands,seeds,user_bonus,...}
//	POST plant_game.php action=plant&land_id&plot_index&seed_id
//	POST plant_game.php action=plant_fill_empty&seed_id  （一键种空地）
//	POST plant_game.php action=harvest&land_id&plot_index（单块收获）
//	POST plant_game.php action=harvest_all&imagehash&imagestring（需验证码——不自动）
//	POST plant_game.php action=sell_inventory&seed_id&quantity
//
// 本适配器实现 Farmable 主链路：全景/种植（一键种空地）/单块收获/出售；
// harvest_all 需图形验证码，走签到同款 OCR（NexusPHP 适配器已具备；此处仅作占位不自动触发）。
type SiqiPlantGame struct{}

func (SiqiPlantGame) ID() string   { return "siqi_plant" }
func (SiqiPlantGame) Name() string { return "种菜赚魔力" }

// siqiGet GET plant_game.php（query 形如 "fetch"）。
func siqiGet(ctx context.Context, hc *httpclient.Client, baseURL, query, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, "plant_game.php") + "?action=" + query
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "plant_game.php"),
	}, true)
	if err != nil {
		return nil, err
	}
	logResp("siqi_plant "+query, u, resp)
	return parseRaw(resp.Body)
}

// siqiPost POST action=…。
func siqiPost(ctx context.Context, hc *httpclient.Client, baseURL, cookie, action string, params url.Values) (map[string]any, error) {
	u := joinPath(baseURL, "plant_game.php")
	form := url.Values{}
	form.Set("action", action)
	for k, vs := range params {
		for _, v := range vs {
			form.Set(k, v)
		}
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form.Encode()), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          u,
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("siqi_plant "+action, u, resp)
	return parseRaw(resp.Body)
}

// Farm 全景：action=fetch。inventory（仓库种子）与 seeds（商店）合并为 Crops 图鉴；
// lands 的种植状态按 plot 汇总（idle/growing/ready）。
func (SiqiPlantGame) Farm(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FarmState, error) {
	m, err := siqiGet(ctx, hc, baseURL, "fetch", cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &FarmState{Crops: []FarmItem{}, Animals: []FarmItem{}}
	if !boolField(m, "success") {
		return st, fmt.Errorf("%s", firstNonEmpty(strField(m, "msg"), "农场数据加载失败"))
	}
	st.Balance = numField(m, "user_bonus")
	// 累计收获（种子解锁门槛：unlock_harvest）
	totalHarvest := longptNum(m["total_harvest"])
	if us, ok := m["user_stats"].(map[string]any); ok && totalHarvest == 0 {
		totalHarvest = numField(us, "total_harvest")
	}
	// seeds 商店（cost/grow_time 秒/base_reward/unlock_harvest）
	if arr, ok := m["seeds"].([]any); ok {
		for _, it := range arr {
			s, ok := it.(map[string]any)
			if !ok {
				continue
			}
			fi := FarmItem{
				ID:        int(longptNum(s["id"])),
				Name:      firstNonEmpty(strField(s, "name"), strField(s, "seed_name")),
				Icon:      strField(s, "icon"), // 站点直接给 emoji
				Section:   "crop",
				GrowHours: int(longptNum(s["grow_time"]) / 3600),
				Price:     numField(s, "cost"),
				Reward:    numField(s, "base_reward"),
				Status:    "idle",
			}
			if fi.Icon == "" {
				fi.Icon = strField(s, "emoji")
			}
			if fi.GrowHours == 0 && longptNum(s["grow_time"]) > 0 {
				fi.GrowHours = 1 // 不足 1 小时的按 1 计
			}
			if gate := longptNum(s["unlock_harvest"]); gate > 0 && totalHarvest < gate {
				fi.Locked = true
				fi.UnlockText = fmt.Sprintf("累计收获 %s 解锁（当前 %s）",
					strOrDash(gate), strOrDash(totalHarvest))
			}
			st.Crops = append(st.Crops, fi)
		}
	}
	// lands 地块状态 → 按种子 id 汇总 ready/growing
	if lands, ok := m["lands"].([]any); ok {
		ready := map[int]bool{}
		growing := map[int]bool{}
		for _, it := range lands {
			l, ok := it.(map[string]any)
			if !ok {
				continue
			}
			// 地块结构（渲染 JS）：plots 数组内 {seed_id,status,...}；兼容平铺。
			plots := []map[string]any{}
			if ps, ok := l["plots"].([]any); ok {
				for _, p := range ps {
					if pm, ok := p.(map[string]any); ok {
						plots = append(plots, pm)
					}
				}
			} else {
				plots = append(plots, l)
			}
			for _, p := range plots {
				sid := int(longptNum(p["seed_id"]))
				if sid == 0 {
					continue
				}
				switch strField(p, "status") {
				case "mature", "ready", "harvestable":
					ready[sid] = true
				default:
					growing[sid] = true
				}
			}
		}
		for i := range st.Crops {
			if ready[st.Crops[i].ID] {
				st.Crops[i].Status = "ready"
			} else if growing[st.Crops[i].ID] {
				st.Crops[i].Status = "growing"
			}
		}
	}
	// inventory 仓库（收获物可出售）
	if inv, ok := m["inventory"].([]any); ok {
		for _, it := range inv {
			v, ok := it.(map[string]any)
			if !ok {
				continue
			}
			st.Warehouse = append(st.Warehouse, StorageItem{
				Key:   fmt.Sprint(longptNum(v["seed_id"])),
				Name:  firstNonEmpty(strField(v, "name"), strField(v, "seed_name")),
				Count: int(longptNum(v["count"])),
			})
		}
	}
	return st, nil
}

// Plant 一键种空地（plant_fill_empty&seed_id；站点支持，比逐块 land/plot 稳）。
func (SiqiPlantGame) Plant(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, section string, itemID int) (*PlayResult, error) {
	m, err := siqiPost(ctx, hc, baseURL, cr.Cookie, "plant_fill_empty", url.Values{
		"seed_id": {fmt.Sprint(itemID)},
	})
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: boolField(m, "success"), Message: firstNonEmpty(strField(m, "msg"), strField(m, "message")), Raw: m}
	res.Balance = numField(m, "user_bonus")
	if res.Success && res.Message == "" {
		res.Message = "一键种植完成"
	}
	return res, nil
}

// Harvest 单块收获：itemID 编码 land*100+plot（前端约定；简化传 land_id）。
func (SiqiPlantGame) Harvest(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, section string, itemID int) (*PlayResult, error) {
	landID, plotIdx := itemID/100, itemID%100
	if landID == 0 {
		landID = itemID
		plotIdx = 0
	}
	m, err := siqiPost(ctx, hc, baseURL, cr.Cookie, "harvest", url.Values{
		"land_id":    {fmt.Sprint(landID)},
		"plot_index": {fmt.Sprint(plotIdx)},
	})
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: boolField(m, "success"), Message: firstNonEmpty(strField(m, "msg")), Raw: m}
	res.Amount = numField(m, "reward")
	res.Balance = numField(m, "user_bonus")
	if res.Success && res.Message == "" {
		res.Message = "收获完成"
	}
	return res, nil
}

// HarvestAll 一键收获需图形验证码（imagehash/imagestring）——不自动破解；
// 返回提示走站点页面操作（保守：验证码错误会空耗作物）。
func (SiqiPlantGame) HarvestAll(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return &PlayResult{Success: false, Message: "思齐一键收获需要图形验证码：请在站点页面手动完成（单块收获可用）"}, nil
}

// Sell 出售仓库 sell_inventory&seed_id&quantity（quantity 缺省全部）。
func (SiqiPlantGame) Sell(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, key string) (*PlayResult, error) {
	m, err := siqiPost(ctx, hc, baseURL, cr.Cookie, "sell_inventory", url.Values{
		"seed_id":  {key},
		"quantity": {"999999"},
	})
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: boolField(m, "success"), Message: firstNonEmpty(strField(m, "msg")), Raw: m}
	res.Balance = numField(m, "user_bonus")
	return res, nil
}

// Play（LotteryGame 兜底）= 全景只读。
func (SiqiPlantGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	st, err := SiqiPlantGame{}.Farm(ctx, hc, baseURL, cr)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	return &PlayResult{Success: true, Balance: st.Balance, Message: fmt.Sprintf("农场已同步（%d 种作物）", len(st.Crops))}, nil
}

// Stats 余额。
func (SiqiPlantGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	m, err := siqiGet(ctx, hc, baseURL, "fetch", cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: numField(m, "user_bonus")}, nil
}
