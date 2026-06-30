package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 财神（cspt）富豪刮刮乐。契约来自迁移前 Kotlin 版（adapter/lottery/CsptScratchGame.kt）
// + cspt-web Laravel 源码（ScratchCardController）。
//
// 财神 API 走 **Bearer token**：POST /api/login（站点用户名+密码 JSON）换 token，
// 再带 Authorization: Bearer 调各 form-urlencoded 接口（start-game/user-info/base-info/buy/log）。
// 光带站点 cookie → 401 未授权；token 失效（ret==401/未授权）→ 重登一次再试。
// 游戏实例为单例，无法跨站缓存 token，故每次操作都重登。
type CsptScratchGame struct{}

func (CsptScratchGame) ID() string   { return "cspt_scratch" }
func (CsptScratchGame) Name() string { return "财神刮刮乐" }

// csptBuyOptions 购卡档位（单卡 2000 金元宝；50 张送 2、100 张送 5）。
var csptBuyOptions = []BuyOption{
	{Type: 1, BuyCount: 1, GiftCount: 0, UnitPrice: 2000},
	{Type: 2, BuyCount: 10, GiftCount: 0, UnitPrice: 2000},
	{Type: 3, BuyCount: 50, GiftCount: 2, UnitPrice: 2000},
	{Type: 4, BuyCount: 100, GiftCount: 5, UnitPrice: 2000},
}

func (g CsptScratchGame) BuyOptions() []BuyOption { return csptBuyOptions }

// Play 刮一张（start-game）。
func (CsptScratchGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := csptRequest(ctx, hc, strings.TrimRight(baseURL, "/"), "api/plugin-scratch-card-start-game", "", cr)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	return csptPlayResult(m), nil
}

// Stats 剩余刮卡 / 金元宝余额（user-info）。
func (CsptScratchGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	m, err := csptRequest(ctx, hc, strings.TrimRight(baseURL, "/"), "api/plugin-scratch-card-user-info", "", cr)
	if err != nil {
		return nil, err
	}
	data, _ := m["data"].(map[string]any)
	return &StatsResult{
		Balance:     numField(data, "seedbonus"),
		CardNumber:  int(numField(data, "card_number")),
		CostPerPlay: 2000,
	}, nil
}

// Buy 购卡（buy）。
func (CsptScratchGame) Buy(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, optionType int) (*PlayResult, error) {
	opt := csptBuyOptions[0]
	for _, o := range csptBuyOptions {
		if o.Type == optionType {
			opt = o
			break
		}
	}
	form := url.Values{}
	form.Set("type", fmt.Sprint(optionType))
	form.Set("id", fmt.Sprint(optionType))
	form.Set("buy_type", fmt.Sprint(optionType))
	form.Set("option_type", fmt.Sprint(optionType))
	form.Set("count", fmt.Sprint(opt.BuyCount))
	form.Set("number", fmt.Sprint(opt.BuyCount))
	form.Set("number_buy", fmt.Sprint(opt.BuyCount))
	form.Set("number_give", fmt.Sprint(opt.GiftCount))
	m, err := csptRequest(ctx, hc, strings.TrimRight(baseURL, "/"), "api/plugin-scratch-card-buy", form.Encode(), cr)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := csptPlayResult(m)
	if res.Message == "" {
		res.Message = fmt.Sprintf("购买 %d 张刮刮卡", opt.BuyCount)
	}
	return res, nil
}

// Prizes 奖品展示（base-info.rewardDisplay.rewardList）。
func (CsptScratchGame) Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error) {
	m, err := csptRequest(ctx, hc, strings.TrimRight(baseURL, "/"), "api/plugin-scratch-card-base-info", "", cr)
	if err != nil {
		return nil, err
	}
	data, _ := m["data"].(map[string]any)
	display, _ := data["rewardDisplay"].(map[string]any)
	list, _ := display["rewardList"].([]any)
	out := make([]PrizeDef, 0, len(list))
	for i, it := range list {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		title := strField(obj, "title")
		if title == "" {
			continue
		}
		out = append(out, PrizeDef{ID: i + 1, Name: title, Icon: strField(obj, "url")})
	}
	return out, nil
}

// History 抽奖记录（log）。
func (CsptScratchGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	m, err := csptRequest(ctx, hc, strings.TrimRight(baseURL, "/"), "api/plugin-scratch-card-log", "limit=30", cr)
	if err != nil {
		return nil, err
	}
	data, _ := m["data"].(map[string]any)
	list, _ := data["data"].([]any)
	out := make([]GameRecord, 0, len(list))
	for _, it := range list {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, GameRecord{
			PrizeName: strField(obj, "reward_title"),
			Amount:    numField(obj, "reward_num"),
			CreatedAt: strField(obj, "created_at"),
		})
	}
	return out, nil
}

// csptPlayResult 把信封 map 抽成 PlayResult（data 里取 reward_title/reward_num/bonus）。
func csptPlayResult(m map[string]any) *PlayResult {
	msg := strField(m, "msg")
	// 财神响应的 msg 常是 Laravel 翻译键（如 "scratchcard.startgame"），后端未本地化、
	// 原样回键名 → 非可读文案。丢弃以免前端刮卡区透传显示原始键（ScratchCardGame reveal-msg）。
	droppedKey := looksLikeI18NKey(msg)
	if droppedKey {
		msg = ""
	}
	ret := int64(numField(m, "ret"))
	if ret != 0 && ret != 200 {
		return &PlayResult{Success: false, Message: msg}
	}
	res := &PlayResult{Success: true, Message: msg, Raw: m}
	if data, ok := m["data"].(map[string]any); ok {
		title := strField(data, "reward_title")
		amount := numField(data, "reward_num")
		res.Prize = title
		res.Amount = amount
		res.Balance = numField(data, "bonus")
		// 仅当后端确实没回 msg 时补兜底文案；i18n 键被丢弃时不补——prize/amount 已完整展示结果，
		// 再补「获得 元宝」会与刮卡区奖项名重复。
		if res.Message == "" && !droppedKey {
			if amount > 0 {
				res.Message = "获得 " + title
			} else {
				res.Message = title
			}
		}
	}
	return res
}

// looksLikeI18NKey 判定 s 是否「翻译键」风格的原始字段值：仅含 [A-Za-z0-9_.] 且含点号。
// 用于识别财神未本地化回传的 Laravel 键（scratchcard.startgame / scratchcard.nocard …），
// 与真实可读文案（含中文/空格/标点）区分。
func looksLikeI18NKey(s string) bool {
	if !strings.Contains(s, ".") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '_' || r == '.':
		default:
			return false
		}
	}
	return true
}

// csptRequest 登录换 token 后 POST 财神 API（form-urlencoded）；401/未授权重登重试一次。
// 返回解析后的完整信封 {ret,msg,data}。
func csptRequest(ctx context.Context, hc *httpclient.Client, origin, path, formBody string, cr Creds) (map[string]any, error) {
	if cr.Username == "" || cr.Password == "" {
		return nil, fmt.Errorf("未配置财神站点用户名/密码（娱乐页需 API 登录，请在站点设置填写）")
	}
	token, err := csptLogin(ctx, hc, origin, cr)
	if err != nil {
		return nil, fmt.Errorf("财神登录失败：%s", err.Error())
	}
	urlStr := origin + "/" + path
	m, retry, err := csptDoPost(ctx, hc, origin, urlStr, formBody, cr.Cookie, token)
	if err != nil {
		return nil, err
	}
	if retry {
		token, err = csptLogin(ctx, hc, origin, cr)
		if err != nil {
			return nil, fmt.Errorf("财神登录失败：%s", err.Error())
		}
		m, _, err = csptDoPost(ctx, hc, origin, urlStr, formBody, cr.Cookie, token)
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}

// csptDoPost 带 Bearer POST 一次；返回 (信封, 是否需重登重试)。
func csptDoPost(ctx context.Context, hc *httpclient.Client, origin, urlStr, formBody, cookie, token string) (map[string]any, bool, error) {
	headers := csptAPIHeaders(origin, token)
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	var body []byte
	if formBody != "" {
		body = []byte(formBody)
	}
	resp, err := hc.Do(ctx, http.MethodPost, urlStr, body, headers, cookie, true)
	if err != nil {
		return nil, false, err
	}
	logResp("cspt "+pathTail(urlStr), urlStr, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		snippet := string(resp.Body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return map[string]any{"ret": float64(-1), "msg": snippet}, false, nil
	}
	msg := strField(m, "msg")
	ret := int64(numField(m, "ret"))
	low := strings.ToLower(msg)
	if ret == 401 || strings.Contains(low, "unauthorized") || strings.Contains(msg, "未授权") || strings.Contains(msg, "请先登录") {
		return m, true, nil
	}
	return m, false, nil
}

// csptLogin POST /api/login 换 Bearer token。
func csptLogin(ctx context.Context, hc *httpclient.Client, origin string, cr Creds) (string, error) {
	body, _ := json.Marshal(map[string]string{"username": cr.Username, "password": cr.Password})
	// 必须 Content-Type: application/json：财神 /api/login 走 Laravel 校验，仅当请求头声明 JSON 时
	// 才把 body 解析进 $request；否则 username 恒空，返 422「The username field is required.」。
	// （httpclient.Do 对未声明类型的 body 默认 text/plain，触发本 bug。）
	headers := csptAPIHeaders(origin, "")
	headers["Content-Type"] = "application/json"
	resp, err := hc.Do(ctx, http.MethodPost, origin+"/api/login", body, headers, cr.Cookie, true)
	if err != nil {
		return "", err
	}
	logResp("cspt login", origin+"/api/login", resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		snippet := string(resp.Body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return "", fmt.Errorf("%s", snippet)
	}
	if ret := int64(numField(m, "ret")); ret != 0 && ret != 200 {
		return "", fmt.Errorf("%s", strField(m, "msg"))
	}
	data, _ := m["data"].(map[string]any)
	tok := strField(data, "token")
	if tok == "" {
		return "", fmt.Errorf("登录成功但未返回 token")
	}
	if !strings.HasPrefix(strings.ToLower(tok), "bearer ") {
		tok = "Bearer " + tok
	}
	return tok, nil
}

// csptAPIHeaders 财神 API 公共头：Accept json + Origin + Referer（刮刮乐页）+ 可选 Bearer。
func csptAPIHeaders(origin, token string) map[string]string {
	h := map[string]string{
		"Accept":  "application/json, text/plain, */*",
		"Origin":  origin,
		"Referer": origin + "/new/#/lottery/scratch",
	}
	if token != "" {
		h["Authorization"] = token
	}
	return h
}
