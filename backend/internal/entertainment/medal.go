package entertainment

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 好学卡牌/勋章合成（medal_synthesis.php）。
// 真实契约（迁移前 Kotlin 版 + 站点源码）：
//   - GET 页面取 csrf_token（CSRF_TOKEN = "..." 或 <input name=csrf_token>）+ synthesis_id 列表（页解析）；
//   - POST action=draw/draw_ten/draw_hundred/draw_custom + params={"synthesis_id":N[,count]} + csrf_token，
//     带 Referer + X-CSRF-TOKEN 头；合成 action=synthesize；
//   - 成功 draw: {ret:0, data:{draw_count, fragments:[{name,count}], luck_level, remaining_bonus}}；
//     合成: {ret:0, data:{medal:{name,image_large}}}；失败 {ret:1, msg}；
//     msg 含「安全验证/csrf/刷新页面」= token 失效，刷新后重试一次。
type MedalSynthesisGame struct{}

func (MedalSynthesisGame) ID() string   { return "medal_synthesis" }
func (MedalSynthesisGame) Name() string { return "卡牌合成" }

// Play 「玩一次」= 抽首个合成目标 1 片（站点 UI 本身要求先选目标，此处默认第一个）。
func (MedalSynthesisGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	pageURL := joinPath(baseURL, "medal_synthesis.php")
	csrf, synthID := loadMedalPage(ctx, hc, pageURL, pageURL, cr.Cookie)
	if synthID == "" {
		return &PlayResult{Success: false, Message: "未找到可用合成目标，请在站点页选择后再抽"}, nil
	}
	return medalDoDraw(ctx, hc, pageURL, cr.Cookie, synthID, 1, csrf), nil
}

// Draw 抽取指定合成目标 N 片（count: 1/10/100/其它）。
func (MedalSynthesisGame) Draw(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, synthesisID, count int) (*PlayResult, error) {
	pageURL := joinPath(baseURL, "medal_synthesis.php")
	csrf, _ := loadMedalPage(ctx, hc, pageURL, pageURL, cr.Cookie)
	return medalDoDraw(ctx, hc, pageURL, cr.Cookie, strconv.Itoa(synthesisID), count, csrf), nil
}

// Synthesize 合成指定目标（集齐碎片后）。
func (MedalSynthesisGame) Synthesize(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, synthesisID int) (*PlayResult, error) {
	pageURL := joinPath(baseURL, "medal_synthesis.php")
	params := fmt.Sprintf(`{"synthesis_id":%d}`, synthesisID)
	csrf, _ := loadMedalPage(ctx, hc, pageURL, pageURL, cr.Cookie)
	m, retry, _ := medalAction(ctx, hc, pageURL, cr.Cookie, "synthesize", params, csrf)
	if retry {
		csrf, _ = loadMedalPage(ctx, hc, pageURL, pageURL, cr.Cookie)
		m, _, _ = medalAction(ctx, hc, pageURL, cr.Cookie, "synthesize", params, csrf)
	}
	return medalSynthResult(m), nil
}

// Stats 当前火花余额（GET 页面抠 userBonus / 当前火花）。
func (MedalSynthesisGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	pageURL := joinPath(baseURL, "medal_synthesis.php")
	resp, err := hc.GetWithHeaders(ctx, pageURL, cr.Cookie, map[string]string{"Referer": pageURL}, true)
	if err != nil {
		return nil, err
	}
	bal := firstGroup(resp.Body, `userBonus['"]?>?[^=]*=\s*([\d.]+)`)
	if bal == "" {
		bal = strings.ReplaceAll(firstGroup(resp.Body, `当前火花[:：]\s*([\d,]+)`), ",", "")
	}
	balance, _ := strconv.ParseFloat(bal, 64)
	return &StatsResult{Balance: balance, CostPerPlay: 500}, nil
}

// History 抽取记录（action=get_history params={synthesis_id:0 全部, page:1}）。
func (MedalSynthesisGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	pageURL := joinPath(baseURL, "medal_synthesis.php")
	csrf, _ := loadMedalPage(ctx, hc, pageURL, pageURL, cr.Cookie)
	m, _, err := medalAction(ctx, hc, pageURL, cr.Cookie, "get_history", `{"synthesis_id":0,"page":1}`, csrf)
	if err != nil {
		return nil, err
	}
	data, _ := m["data"].(map[string]any)
	logs, _ := data["logs"].([]any)
	out := make([]GameRecord, 0, len(logs))
	for _, it := range logs {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, GameRecord{
			PrizeName: strField(obj, "fragment_name"),
			PrizeType: strField(obj, "synthesis_name"),
			Cost:      numField(obj, "cost"),
			CreatedAt: strField(obj, "created_at"),
		})
	}
	return out, nil
}

// medalDoDraw 抽取（按 count 选 action），CSRF 失效重试一次。
func medalDoDraw(ctx context.Context, hc *httpclient.Client, pageURL, cookie, synthID string, count int, csrf string) *PlayResult {
	action := "draw_custom"
	switch count {
	case 1:
		action = "draw"
	case 10:
		action = "draw_ten"
	case 100:
		action = "draw_hundred"
	}
	params := fmt.Sprintf(`{"synthesis_id":%s}`, synthID)
	if action == "draw_custom" {
		params = fmt.Sprintf(`{"synthesis_id":%s,"count":%d}`, synthID, count)
	}
	m, retry, _ := medalAction(ctx, hc, pageURL, cookie, action, params, csrf)
	if retry {
		csrf, _ = loadMedalPage(ctx, hc, pageURL, pageURL, cookie)
		m, _, _ = medalAction(ctx, hc, pageURL, cookie, action, params, csrf)
	}
	return medalDrawResult(m, count)
}

// loadMedalPage GET 合成页，抠 csrf_token + 第一个 synthesis_id。取不到返回空。
func loadMedalPage(ctx context.Context, hc *httpclient.Client, pageURL, referer, cookie string) (csrf, synthID string) {
	resp, err := hc.GetWithHeaders(ctx, pageURL, cookie, map[string]string{"Referer": referer}, true)
	if err != nil {
		return "", ""
	}
	csrf = firstGroup(resp.Body, `CSRF_TOKEN\s*=\s*["']([^"']+)["']`)
	if csrf == "" {
		csrf = firstGroup(resp.Body, `(?i)name=["']csrf_token["']\s[^>]*value=["']([^"']+)["']`)
	}
	synthID = firstGroup(resp.Body, `data-synthesis-id="(\d+)"`)
	return csrf, synthID
}

// medalAction POST 一次操作（draw*/synthesize），返回 (信封, 是否CSRF失效需重试, err)。
func medalAction(ctx context.Context, hc *httpclient.Client, pageURL, cookie, action, paramsJSON, csrf string) (map[string]any, bool, error) {
	form := url.Values{}
	form.Set("action", action)
	form.Set("params", paramsJSON)
	extra := map[string]string{}
	if csrf != "" {
		form.Set("csrf_token", csrf)
		extra["X-CSRF-TOKEN"] = csrf
	}
	resp, err := hc.PostFormAjaxWith(ctx, pageURL, cookie, form.Encode(), pageURL, extra, true)
	if err != nil {
		return nil, false, err
	}
	logResp("medal "+action, pageURL, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		snippet := string(resp.Body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return map[string]any{"ret": float64(-1), "msg": snippet}, false, nil
	}
	if numField(m, "ret") != 0 {
		msg := strField(m, "msg")
		low := strings.ToLower(msg)
		csrfFail := strings.Contains(low, "csrf") || strings.Contains(msg, "安全验证") || strings.Contains(msg, "刷新页面")
		return m, csrfFail, nil
	}
	return m, false, nil
}

// medalDrawResult 把抽取信封抽成 PlayResult（碎片汇总 + 余额 + 抽取数）。
func medalDrawResult(m map[string]any, count int) *PlayResult {
	if m == nil {
		return &PlayResult{Success: false, Message: "无响应"}
	}
	if numField(m, "ret") != 0 {
		return &PlayResult{Success: false, Message: strField(m, "msg")}
	}
	res := &PlayResult{Success: true, Raw: m}
	if data, ok := m["data"].(map[string]any); ok {
		drawCount := int(numField(data, "draw_count"))
		luck := strField(data, "luck_level")
		res.Balance = numField(data, "remaining_bonus")
		if frags, ok := data["fragments"].([]any); ok {
			var parts []string
			for _, f := range frags {
				if fo, ok := f.(map[string]any); ok {
					name := strField(fo, "name")
					if name != "" {
						parts = append(parts, fmt.Sprintf("%s×%d", name, int(numField(fo, "count"))))
					}
				}
			}
			if len(parts) > 0 {
				res.Prize = strings.Join(parts, "、")
			}
		}
		if msg := strField(m, "msg"); msg != "" {
			res.Message = msg
		} else {
			cnt := drawCount
			if cnt == 0 {
				cnt = count
			}
			if cnt == 0 {
				cnt = 1
			}
			extra := ""
			if luck != "" {
				extra = "，" + luck
			}
			res.Message = fmt.Sprintf("%d 次抽取完成%s", cnt, extra)
		}
	}
	return res
}

// medalSynthResult 把合成信封抽成 PlayResult（勋章名）。
func medalSynthResult(m map[string]any) *PlayResult {
	if m == nil {
		return &PlayResult{Success: false, Message: "无响应"}
	}
	if numField(m, "ret") != 0 {
		return &PlayResult{Success: false, Message: strField(m, "msg")}
	}
	res := &PlayResult{Success: true, Message: strField(m, "msg"), Raw: m}
	if data, ok := m["data"].(map[string]any); ok {
		if medal, ok := data["medal"].(map[string]any); ok {
			res.Prize = strField(medal, "name")
		}
	}
	if res.Message == "" {
		res.Message = "合成成功"
	}
	return res
}
