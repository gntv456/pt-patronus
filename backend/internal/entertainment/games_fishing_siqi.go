package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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
