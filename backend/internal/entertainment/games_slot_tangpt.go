package entertainment

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 不可躺 老虎机（www.tangpt.top，omnibot_slot.php）─────────────────
// 契约（2026-09 实测页面 JS + stats 真实响应）：
//
//	GET  /web/omnibot/slot-machine/stats → {config{base_cost,daily_free_spins},user_bonus,
//	      jackpot_pool,daily_play_used/remaining,daily_free_spins_remaining,summary,history,
//	      prize_rows,announcements,spin_token}
//	POST /web/omnibot/slot-machine/draw form {multiplier, spin_token} → 落针结果（page_state 等）
//
// 单次基础消耗 5000 魔力（每日 2 次免费）；draw 会扣费——只走 Bet 显式。
type TangptSlotGame struct{}

func (TangptSlotGame) ID() string   { return "tangpt_slot" }
func (TangptSlotGame) Name() string { return "老虎机" }

// tangptSlotStats GET stats（只读全景）。
func tangptSlotStats(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, "web/omnibot/slot-machine/stats")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{
		"Accept":           "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "omnibot_slot.php"),
	}, true)
	if err != nil {
		return nil, err
	}
	logResp("tangpt_slot stats", u, resp)
	return parseRaw(resp.Body)
}

// Play 只读同步：余额/奖池/今日剩余次数/最近公告。
func (g TangptSlotGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := tangptSlotStats(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: true, Balance: numField(m, "user_bonus"), Raw: m}
	cost := numField(m, "base_cost")
	if cfg, ok := m["config"].(map[string]any); ok {
		cost = numField(cfg, "base_cost")
	}
	pool := numField(m, "jackpot_pool")
	remain := longptNum(m["daily_play_remaining"])
	free := longptNum(m["daily_free_spins_remaining"])
	res.Message = fmt.Sprintf("老虎机同步：单抽 %v 魔力，奖池 %v，今日剩余 %v 次（含免费 %v）",
		cost, pool, remain, free)
	return res, nil
}

// Bet 转一次：Selection 为倍率档（"1" 常规；页面另有倍率选项则透传），消耗 = base_cost × multiplier。
// spin_token 从 stats 取（一次性/短时效防重放）。
func (g TangptSlotGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	multiplier := strings.TrimSpace(req.Selection)
	if multiplier == "" || multiplier == "spin" {
		multiplier = "1"
	}
	st, err := tangptSlotStats(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	token := strField(st, "spin_token")
	if token == "" {
		return &PlayResult{Success: false, Message: "未取得 spin_token（可能未登录或游戏停用）"}, nil
	}
	remain := longptNum(st["daily_play_remaining"])
	if remain <= 0 {
		return &PlayResult{Success: false, Message: "今日次数已用完"}, nil
	}
	u := joinPath(baseURL, "web/omnibot/slot-machine/draw")
	form := "multiplier=" + urlQueryEscape(multiplier) + "&spin_token=" + urlQueryEscape(token)
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "omnibot_slot.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("tangpt_slot draw", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "响应解析失败"}, nil
	}
	ok := boolField(m, "ok") || boolField(m, "success")
	msg := firstNonEmpty(strField(m, "message"), strField(m, "msg"))
	if ok && msg == "" {
		// 结果行文案（reels 三轴 + 输赢）
		if ps, ok2 := m["page_state"].(map[string]any); ok2 {
			msg = fmt.Sprintf("转轴结果：%s", compactReels(ps))
		} else {
			msg = "转动完成"
		}
	}
	if !ok && msg == "" {
		msg = "转动失败"
	}
	return &PlayResult{Success: ok, Message: msg, Balance: numField(m, "user_bonus"), Raw: m}, nil
}

// Stats 余额 + 今日剩余。
func (g TangptSlotGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	m, err := tangptSlotStats(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{Balance: numField(m, "user_bonus")}
	st.MaxDaily = int(longptNum(m["daily_play_limit"]))
	st.TodaySpins = int(longptNum(m["daily_play_used"]))
	return st, nil
}

// compactReels 从 page_state 提取三轴图标摘要。
func compactReels(ps map[string]any) string {
	reels, ok := ps["reels"].([]any)
	if !ok {
		return "…"
	}
	var parts []string
	for _, r := range reels {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		parts = append(parts, firstNonEmpty(strField(rm, "icon"), strField(rm, "key"), "?"))
	}
	return strings.Join(parts, " ")
}

// urlQueryEscape 轻量转义（避免直接依赖 net/url 单处引用）。
func urlQueryEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "&", "%26"), "+", "%2B")
}
