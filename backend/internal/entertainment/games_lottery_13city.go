package entertainment

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 13City（13city.org）彩票中心 ────────────────────────────
// GET lottery.php：每天 20:00 开奖的选号彩票（货币=啤酒瓶；5 注一组）。
// 购买（前端契约，2026-09 实测页面 JS）：
//
//	POST lottery.php  action=purchase5&numbers=<JSON数组>&bet_amount=<金额> → {success,message}
//
// 本适配器只做只读（开奖公告/余额）+ 单注购买走 Bet（花钱动作，chain spendGuard 防双击）；
// 开奖结果由站点每日自动结算，不做自动领奖。
type City13LotteryGame struct{}

func (City13LotteryGame) ID() string   { return "13city_lottery" }
func (City13LotteryGame) Name() string { return "彩票中心" }

var (
	c13BonusRe = regexp.MustCompile(`啤酒瓶[^0-9]{0,10}([\d,.]+)|([\d,.]+)\s*啤酒瓶`)
	// 开奖公告优先级：上期开奖/开奖号码（正文数据行）> 每天20:00开奖（title 说明，仅兜底）。
	c13DrawNumRe = regexp.MustCompile(`(?:上期开奖[^<]{0,50}|开奖号码[^<]{0,40})`)
	c13DrawTipRe = regexp.MustCompile(`每天\s*20:00\s*开奖`)
)

// city13Page 取彩票页。
func city13Page(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (string, error) {
	u := joinPath(baseURL, "lottery.php")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{"Referer": u}, true)
	if err != nil {
		return "", err
	}
	logResp("13city_lottery page", u, resp)
	return string(resp.Body), nil
}

// Stats 余额（啤酒瓶）。
func (City13LotteryGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := city13Page(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{}
	if m := c13BonusRe.FindStringSubmatch(body); m != nil {
		v := firstNonEmpty(m[1], m[2])
		st.Balance, _ = strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64)
	}
	return st, nil
}

// Play（LotteryGame 兜底）= 只读开奖信息（不自动购票——购票花啤酒瓶，走 Bet 显式下单）。
func (City13LotteryGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	body, err := city13Page(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: true, Message: "已同步彩票页"}
	if m := c13DrawNumRe.FindString(body); m != "" {
		res.Prize = strings.TrimSpace(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(m, ""))
	} else if m := c13DrawTipRe.FindString(body); m != "" {
		res.Prize = strings.TrimSpace(m)
	}
	if m := c13BonusRe.FindStringSubmatch(body); m != nil {
		res.Balance, _ = strconv.ParseFloat(strings.ReplaceAll(firstNonEmpty(m[1], m[2]), ",", ""), 64)
	}
	return res, nil
}

// Bet 购买 5 注彩票（numbers 为页面选号 JSON 数组文本，如 ["01","02","07","18","22"]；amount=单注金额）。
// 站点契约：POST action=purchase5&numbers=...&bet_amount=... → {success,message}。
func (City13LotteryGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	if req.Amount <= 0 {
		return &PlayResult{Success: false, Message: "请输入单注金额（啤酒瓶）"}, nil
	}
	numbers := firstNonEmpty(req.Extra["numbers"], `[]`)
	u := joinPath(baseURL, "lottery.php")
	form := url.Values{}
	form.Set("action", "purchase5")
	form.Set("numbers", numbers)
	form.Set("bet_amount", fmt.Sprintf("%g", req.Amount))
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form.Encode()), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          u,
	}, cr.Cookie, true)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	logResp("13city_lottery purchase5", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "响应解析失败"}, nil
	}
	res := &PlayResult{Success: boolField(m, "success"), Message: firstNonEmpty(strField(m, "message"), strField(m, "msg")), Raw: m}
	if res.Success {
		res.Prize = "已购 5 注"
	}
	return res, nil
}
