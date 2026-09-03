package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 思齐钓鱼（si-qi.xyz，siqi_fishing.php）────────────────────────
// 契约（2026-09 实测页面内嵌 postAction）：POST siqi_fishing.php，form action=<名>+附加参数，
// 响应 {success, message, outcome, data:{user, state, inventory, ...}}（data 与页面 FISHING_DATA 同构）。
// 主要 action：perform_fishing(+slider_position 0-1)、select_rod、select_lure、purchase_lure、
// submit_guess、start/answer/claim_knowledge_quiz、gift_fish、deliver_task。
// 钓鱼免费（每日 30 次基础次数），不花魔力；收竿位置影响命中——适配器取 0.5（页面滑块往返中心）。
type SiqiFishingGame struct{}

func (SiqiFishingGame) ID() string   { return "siqi_fishing" }
func (SiqiFishingGame) Name() string { return "思齐钓鱼" }

// siqiFishPage GET 页面，提取内嵌 FISHING_DATA JSON。
func siqiFishPage(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, "siqi_fishing.php")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{
		"Referer": u,
	}, true)
	if err != nil {
		return nil, err
	}
	logResp("siqi_fishing page", u, resp)
	m := siqiFishingDataRe.FindSubmatch(resp.Body)
	if m == nil {
		return nil, fmt.Errorf("页面无 FISHING_DATA（可能未登录或改版）")
	}
	var data map[string]any
	if err := json.Unmarshal(m[1], &data); err != nil {
		return nil, fmt.Errorf("parse FISHING_DATA: %w", err)
	}
	return data, nil
}

var siqiFishingDataRe = regexp.MustCompile(`(?s)const\s+FISHING_DATA\s*=\s*(\{.*?\})\s*;?\s*</script>`)

// siqiFishPost POST action=…（form 编码），返回整包响应。
func siqiFishPost(ctx context.Context, hc *httpclient.Client, baseURL, cookie, action string, params url.Values) (map[string]any, error) {
	u := joinPath(baseURL, "siqi_fishing.php")
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
	logResp("siqi_fishing "+action, u, resp)
	return parseRaw(resp.Body)
}

// Play 只读：钓次进度 + 已解锁鱼数 + 今日钓获。不自动抛竿——抛竿走 PlayMulti/动作层显式触发。
func (g SiqiFishingGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	data, err := siqiFishPage(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: true, Raw: data}
	used, limit, caught := 0, 0, 0
	if st, ok := data["state"].(map[string]any); ok {
		used = int(longptNum(st["daily_attempts_used"]))
		limit = int(longptNum(st["daily_attempts_limit"]))
		caught = int(longptNum(st["daily_catch_count"]))
	}
	known := 0
	if codex, ok := data["codex"].([]any); ok {
		for _, c := range codex {
			if cm, ok := c.(map[string]any); ok && boolField(cm, "discovered") {
				known++
			}
		}
	}
	res.Message = fmt.Sprintf("今日钓次 %d/%d，已钓获 %d 尾，图鉴已解锁 %d 种", used, limit, caught, known)
	return res, nil
}

// PlayMulti 一次抛竿收竿（perform_fishing）。count 固定按 1 次（免费钓次，逐次结算）。
func (g SiqiFishingGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	total := &PlayResult{Success: true}
	var msgs []string
	for i := 0; i < count; i++ {
		m, err := siqiFishPost(ctx, hc, baseURL, cr.Cookie, "perform_fishing", url.Values{
			"slider_position": {"0.5000"},
		})
		if err != nil {
			return total, err
		}
		ok := boolField(m, "success")
		msg := firstNonEmpty(strField(m, "message"), strField(m, "outcome"))
		if !ok {
			// 次数用尽/未登录等，直接终止循环并回报
			return &PlayResult{Success: false, Message: firstNonEmpty(msg, "抛竿失败"),
				Raw: m}, nil
		}
		msgs = append(msgs, msg)
	}
	total.Message = strings.Join(msgs, "；")
	return total, nil
}

// FishingState 全景快照（FISHING_DATA 直读，前端富组件渲染）。只读。
func (g SiqiFishingGame) FishingState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FishingState, error) {
	data, err := siqiFishPage(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &FishingState{Codex: []FishingFish{}}
	if u, ok := data["user"].(map[string]any); ok {
		st.Balance = longptNum(u["seedbonus"])
	}
	if s, ok := data["state"].(map[string]any); ok {
		st.AttemptsUsed = int(longptNum(s["daily_attempts_used"]))
		st.AttemptsLimit = int(longptNum(s["daily_attempts_limit"]))
		st.CaughtToday = int(longptNum(s["daily_catch_count"]))
		st.SelectedRod = strField(s, "selected_rod")
		st.SelectedLure = strField(s, "selected_lure")
	}
	// 装备明细：config.rods/lures（名称/价格/效果/描述）× inventory（解锁态/持有量）
	var invRods map[string]any
	var invLures map[string]any
	if inv, ok := data["inventory"].(map[string]any); ok {
		invRods, _ = inv["rods"].(map[string]any)
		invLures, _ = inv["lures"].(map[string]any)
	}
	if cfg, ok := data["config"].(map[string]any); ok {
		if rods, ok := cfg["rods"].(map[string]any); ok {
			// 按 key 排序保证前端顺序稳定
			keys := make([]string, 0, len(rods))
			for k := range rods {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				rm, _ := rods[k].(map[string]any)
				if rm == nil {
					continue
				}
				rd := FishingRod{
					Key:         k,
					Name:        strField(rm, "name"),
					Icon:        strField(rm, "icon"),
					Description: strField(rm, "description"),
					Selected:    st.SelectedRod == k,
				}
				if im, ok := invRods[k].(map[string]any); ok {
					rd.Unlocked = boolField(im, "unlocked")
				}
				// 解锁条件提示
				if ul, ok := rm["unlock"].(map[string]any); ok {
					switch strField(ul, "type") {
					case "default":
						rd.UnlockHint = "默认"
					case "task":
						rd.UnlockHint = fmt.Sprintf("任务 %v 解锁", ul["task_id"])
					default:
						rd.UnlockHint = strField(ul, "type")
					}
				}
				st.Rods = append(st.Rods, rd)
			}
		}
		if lures, ok := cfg["lures"].(map[string]any); ok {
			keys := make([]string, 0, len(lures))
			for k := range lures {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				lm, _ := lures[k].(map[string]any)
				if lm == nil {
					continue
				}
				lr := FishingLure{
					Key:         k,
					Name:        strField(lm, "name"),
					Icon:        strField(lm, "icon"),
					Price:       longptNum(lm["price"]),
					Description: strField(lm, "description"),
					Selected:    st.SelectedLure == k,
				}
				if v, ok := invLures[k]; ok {
					lr.Owned = int(longptNum(v))
				}
				st.Lures = append(st.Lures, lr)
			}
		}
	}
	// 图鉴
	if codex, ok := data["codex"].([]any); ok {
		for _, c := range codex {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			f := FishingFish{
				Name:        strField(cm, "name"),
				Rarity:      strField(cm, "rarity"),
				RarityLabel: strField(cm, "rarity_label"),
				TimeSlot:    int(longptNum(cm["time_slot"])),
				Discovered:  boolField(cm, "discovered"),
				Quantity:    int(longptNum(cm["quantity"])),
			}
			f.Code = int(longptNum(cm["code"]))
			st.Codex = append(st.Codex, f)
			if f.Discovered {
				st.DiscoveredCount++
			}
			st.TotalSpecies++
		}
	}
	st.Message = fmt.Sprintf("今日钓次 %d/%d，已钓获 %d 尾，图鉴 %d/%d",
		st.AttemptsUsed, st.AttemptsLimit, st.CaughtToday, st.DiscoveredCount, st.TotalSpecies)
	return st, nil
}

// Stats 余额（页面 user.seedbonus）。
func (g SiqiFishingGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	data, err := siqiFishPage(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{}
	if u, ok := data["user"].(map[string]any); ok {
		st.Balance = longptNum(u["seedbonus"])
	}
	return st, nil
}

// Bet 装备操作（批16）：
//
//	Selection=buy_lure   Extra: lure_key, quantity(默认1) —— 买鱼饵（花魔力）
//	Selection=select_lure Extra: lure_key               —— 挂饵
//	Selection=select_rod  Extra: rod_key                —— 换竿（需已解锁）
//	Selection=guess       Extra: value                  —— 每日猜数（额外钓次）
func (g SiqiFishingGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	sel := strings.TrimSpace(req.Selection)
	var action string
	params := url.Values{}
	switch sel {
	case "buy_lure":
		key := strings.TrimSpace(req.Extra["lure_key"])
		if key == "" {
			return &PlayResult{Success: false, Message: "需要 Extra.lure_key"}, nil
		}
		qty := strings.TrimSpace(req.Extra["quantity"])
		if qty == "" {
			qty = "1"
		}
		action = "purchase_lure"
		params.Set("lure_key", key)
		params.Set("quantity", qty)
	case "select_lure":
		key := strings.TrimSpace(req.Extra["lure_key"])
		if key == "" {
			return &PlayResult{Success: false, Message: "需要 Extra.lure_key"}, nil
		}
		action = "select_lure"
		params.Set("lure_key", key)
	case "select_rod":
		key := strings.TrimSpace(req.Extra["rod_key"])
		if key == "" {
			return &PlayResult{Success: false, Message: "需要 Extra.rod_key"}, nil
		}
		action = "select_rod"
		params.Set("rod_key", key)
	case "guess":
		v := strings.TrimSpace(req.Extra["value"])
		if v == "" {
			return &PlayResult{Success: false, Message: "需要 Extra.value"}, nil
		}
		action = "submit_guess"
		params.Set("guess_value", v)
	default:
		return &PlayResult{Success: false, Message: "支持动作：buy_lure / select_lure / select_rod / guess"}, nil
	}
	m, err := siqiFishPost(ctx, hc, baseURL, cr.Cookie, action, params)
	if err != nil {
		return nil, err
	}
	ok := boolField(m, "success")
	msg := firstNonEmpty(strField(m, "message"), strField(m, "outcome"))
	if !ok && msg == "" {
		msg = "操作失败"
	}
	if ok && msg == "" {
		msg = "操作成功"
	}
	return &PlayResult{Success: ok, Message: msg, Raw: m}, nil
}
