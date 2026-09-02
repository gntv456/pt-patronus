package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 天枢（dubhe.site）星尘农场：重建太阳系 ─────────────────────────
// 契约（2026-09 实测页面 JS）：POST ajax.php
//
//	body: action=<StardustAction>&params=<JSON 参数>
//	响应: {ret:0, msg:"OK", data:{...}}（NexusPHP AJAX 外壳）
//
// 动作：getStardustFarm{user_id}（全景：lands/crops/余额）、plantStardustCrop{land_id,crop_id}、
// harvestStardustCrop{land_id}、harvestAllStardustCrops{}、waterStardustLand{target_user_id,land_id}、
// stealStardustFragment（偷碎片）、craftStardustPlanet{crop_id}（合成星球）、purchaseStardustLand{}。
// 本适配器实现 Farmable 主链路（全景/种植/收获/一键收获）；偷/浇水/合成走 Gobang 式后续扩展。
type DubheStardustGame struct{}

func (DubheStardustGame) ID() string   { return "dubhe_stardust" }
func (DubheStardustGame) Name() string { return "星尘农场" }

// dubheAPI 调 ajax.php。
func dubheAPI(ctx context.Context, hc *httpclient.Client, baseURL, cookie, action string, params map[string]any) (map[string]any, error) {
	u := joinPath(baseURL, "ajax.php")
	if params == nil {
		params = map[string]any{}
	}
	pj, _ := json.Marshal(params)
	form := url.Values{}
	form.Set("action", action)
	form.Set("params", string(pj))
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form.Encode()), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "stardust_farm.php"),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("dubhe_stardust "+action, u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("响应解析失败: %w", err)
	}
	if ret := int(longptNum(m["ret"])); ret != 0 {
		return nil, fmt.Errorf("%s", firstNonEmpty(strField(m, "msg"), "操作失败"))
	}
	return m, nil
}

// dubheData 取 data 段。
func dubheData(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return nil
}

// Farm 全景：getStardustFarm{user_id:0} → 地块/作物映射 FarmState。
// 站点 data 形态（实测渲染 JS）：{lands:[{id,crop_id,status,harvest_at,...}], crops:[{id,name,...}], bonus/stardust 余额}。
func (DubheStardustGame) Farm(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FarmState, error) {
	m, err := dubheAPI(ctx, hc, baseURL, cr.Cookie, "getStardustFarm", map[string]any{"user_id": 0})
	if err != nil {
		return nil, err
	}
	d := dubheData(m)
	st := &FarmState{Crops: []FarmItem{}, Animals: []FarmItem{}}
	if d == nil {
		return st, nil
	}
	st.Balance = firstNonZero(numField(d, "bonus"), numField(d, "stardust"), numField(d, "user_bonus"))
	// 作物图鉴 → Crops（section=crop，价格/时长尽力取；缺省 0 由前端显示「—」）
	if arr, ok := d["crops"].([]any); ok {
		for _, it := range arr {
			c, ok := it.(map[string]any)
			if !ok {
				continue
			}
			st.Crops = append(st.Crops, FarmItem{
				ID:        int(longptNum(c["id"])),
				Name:      firstNonEmpty(strField(c, "name"), strField(c, "name_en")),
				Section:   "crop",
				GrowHours: int(numField(c, "grow_hours")),
				Price:     firstNonZero(numField(c, "price"), numField(c, "cost")),
				Status:    "idle",
			})
		}
	}
	// 地块状态 → 覆盖对应作物的 status（land.crop_id 归属；无 crop 的地块不影响图鉴）
	if lands, ok := d["lands"].([]any); ok {
		ready := map[int]bool{}
		growing := map[int]bool{}
		for _, it := range lands {
			l, ok := it.(map[string]any)
			if !ok {
				continue
			}
			cid := int(longptNum(l["crop_id"]))
			if cid == 0 {
				continue
			}
			status := strField(l, "status")
			if status == "" {
				status = "growing"
			}
			if status == "ready" || status == "mature" || status == "harvestable" {
				ready[cid] = true
			} else {
				growing[cid] = true
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
	return st, nil
}

// Plant 种植 plantStardustCrop{land_id,crop_id}；站点 land_id 选择由前端传入（Extra 缺省 1）。
func (DubheStardustGame) Plant(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, section string, itemID int) (*PlayResult, error) {
	landID := 1
	m, err := dubheAPI(ctx, hc, baseURL, cr.Cookie, "plantStardustCrop", map[string]any{"land_id": landID, "crop_id": itemID})
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	d := dubheData(m)
	res := &PlayResult{Success: true, Message: firstNonEmpty(strField(m, "msg"), "种植成功")}
	if d != nil {
		res.Balance = firstNonZero(numField(d, "bonus"), numField(d, "user_bonus"))
	}
	return res, nil
}

// Harvest 收获单块 harvestStardustCrop{land_id}。
func (DubheStardustGame) Harvest(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, section string, itemID int) (*PlayResult, error) {
	m, err := dubheAPI(ctx, hc, baseURL, cr.Cookie, "harvestStardustCrop", map[string]any{"land_id": itemID})
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	d := dubheData(m)
	res := &PlayResult{Success: true, Message: firstNonEmpty(strField(m, "msg"), "收获完成"), Amount: numField(d, "reward")}
	if d != nil {
		res.Balance = firstNonZero(numField(d, "bonus"), numField(d, "user_bonus"))
	}
	return res, nil
}

// HarvestAll 一键收获 harvestAllStardustCrops{}。
func (DubheStardustGame) HarvestAll(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := dubheAPI(ctx, hc, baseURL, cr.Cookie, "harvestAllStardustCrops", map[string]any{})
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	d := dubheData(m)
	res := &PlayResult{Success: true, Message: firstNonEmpty(strField(m, "msg"), "一键收获完成")}
	if d != nil {
		res.Balance = firstNonZero(numField(d, "bonus"), numField(d, "user_bonus"))
		res.Amount = numField(d, "total_reward")
	}
	return res, nil
}

// Sell 星尘农场无出售（产物直接入账）；提供合成星球入口 craftStardustPlanet{crop_id}（itemID=crop_id）。
func (DubheStardustGame) Sell(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, key string) (*PlayResult, error) {
	m, err := dubheAPI(ctx, hc, baseURL, cr.Cookie, "craftStardustPlanet", map[string]any{"crop_id": longptNum(key)})
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	return &PlayResult{Success: true, Message: firstNonEmpty(strField(m, "msg"), "合成完成")}, nil
}

// Play（LotteryGame 兜底）= 一键收获。
func (DubheStardustGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return DubheStardustGame{}.HarvestAll(ctx, hc, baseURL, cr)
}

// Stats 余额。
func (DubheStardustGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	st, err := DubheStardustGame{}.Farm(ctx, hc, baseURL, cr)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: st.Balance}, nil
}

// firstNonZero 返回首个非 0 值（多字段名兼容用）。
func firstNonZero(vs ...float64) float64 {
	for _, v := range vs {
		if v != 0 {
			return v
		}
	}
	return 0
}
