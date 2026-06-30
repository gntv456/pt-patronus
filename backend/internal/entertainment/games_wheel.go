package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 多站幸运转盘/抽奖（纯 Cookie）：UB(wof.php) / LuckyDraw 插件(PANDA+蟹黄堡) / March(/lucky-wheel) / 不可躺(omnibot lottery)。

// ── UB 幸运大转盘（ubits.club） ────────────────────────────
// GET wof.php?app=lottery_json&count=N&csrf_token=T → {rid,rname}（rid=0 谢谢参与）。
// csrf 从 <meta name="csrf-token">；余额抠整页「U币」。
type UbWheelGame struct{}

func (UbWheelGame) ID() string   { return "ub_wheel" }
func (UbWheelGame) Name() string { return "幸运大转盘" }

func (UbWheelGame) draw(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	page := joinPath(baseURL, "wof.php")
	pr, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	logResp("ub_wheel page", page, pr)
	csrf := firstGroup(pr.Body, `(?s)name="csrf-token"\s+content="([^"]+)"`)
	balance := novaHDNum(firstGroup(pr.Body, `U币[^0-9-]*([\d,.]+)`))

	u := page + "?app=lottery_json"
	if count > 1 {
		u += "&count=" + strconv.Itoa(count)
	}
	if csrf != "" {
		u += "&csrf_token=" + csrf
	}
	resp, err := hc.Do(ctx, http.MethodGet, u, nil, map[string]string{"Referer": page, "X-Requested-With": "XMLHttpRequest"}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("ub_wheel draw", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "解析抽奖结果失败"}, nil
	}
	rid := int(longptNum(m["rid"]))
	rname := strField(m, "rname")
	res := &PlayResult{Success: true, PrizeID: rid, Prize: rname, Balance: balance, Raw: m}
	if rid == 0 || strings.Contains(rname, "谢谢") || strings.Contains(rname, "参与") {
		res.Prize, res.Message = "未中奖", "很遗憾，未中奖"
	} else {
		res.Message = "中奖：" + rname
	}
	return res, nil
}

func (UbWheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return UbWheelGame{}.draw(ctx, hc, baseURL, cr, 1)
}

func (UbWheelGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	return UbWheelGame{}.draw(ctx, hc, baseURL, cr, count)
}

func (UbWheelGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "wof.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: novaHDNum(firstGroup(resp.Body, `U币[^0-9-]*([\d,.]+)`))}, nil
}

// ── LuckyDraw 插件（PANDA pandapt.net + 蟹黄堡 crabpt.vip 共用） ────────────
// POST /plugin/lucky-draw {}（单）/ /plugin/lucky-draw-bulk {"option":N}（连）→ {ret,msg,data:{angle,prize_text}}。
type LuckyDrawWheelGame struct{}

func (LuckyDrawWheelGame) ID() string   { return "luckydraw_wheel" }
func (LuckyDrawWheelGame) Name() string { return "幸运大转盘" }

func luckyDrawPost(ctx context.Context, hc *httpclient.Client, baseURL, path, cookie string, body any) (map[string]any, error) {
	u := joinPath(baseURL, path)
	payload, _ := json.Marshal(body)
	resp, err := hc.Do(ctx, http.MethodPost, u, payload, map[string]string{
		"Content-Type":     "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "luckywheel.php"),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("luckydraw "+path, u, resp)
	return parseRaw(resp.Body)
}

func luckyDrawResult(m map[string]any) *PlayResult {
	ret := int(longptNum(m["ret"]))
	res := &PlayResult{Success: ret == 0, Message: strField(m, "msg"), Raw: m}
	if data, ok := m["data"].(map[string]any); ok {
		res.Prize = strField(data, "prize_text")
	}
	if res.Success && res.Prize == "" {
		res.Prize = "中奖"
	}
	if !res.Success && res.Message == "" {
		res.Message = "抽奖失败"
	}
	return res
}

func (LuckyDrawWheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := luckyDrawPost(ctx, hc, baseURL, "plugin/lucky-draw", cr.Cookie, map[string]any{})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	return luckyDrawResult(m), nil
}

func (LuckyDrawWheelGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	m, err := luckyDrawPost(ctx, hc, baseURL, "plugin/lucky-draw-bulk", cr.Cookie, map[string]any{"option": count})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	return luckyDrawResult(m), nil
}

// ── March 幸运大转盘（duckboobee.org） ────────────────────
// POST /lucky-wheel/spin → {success, results:[{...}], remaining_bonus, message}。
type MarchWheelGame struct{}

func (MarchWheelGame) ID() string   { return "march_wheel" }
func (MarchWheelGame) Name() string { return "幸运大转盘" }

func (MarchWheelGame) spin(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	u := joinPath(baseURL, "lucky-wheel/spin")
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte("{}"), map[string]string{
		"Content-Type":     "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "luckywheel.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("march spin", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	res := &PlayResult{Success: boolField(m, "success"), Balance: numField(m, "remaining_bonus"), Message: strField(m, "message"), Raw: m}
	if results, ok := m["results"].([]any); ok {
		res.Prizes = make([]PrizeOutcome, 0, len(results))
		for _, r := range results {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			name := firstNonEmpty(strField(rm, "name"), strField(rm, "prize_name"), strField(rm, "text"))
			res.Prizes = append(res.Prizes, PrizeOutcome{PrizeName: name, Amount: numField(rm, "amount"), IsWin: name != "" && !strings.Contains(name, "谢谢")})
		}
		if len(res.Prizes) > 0 {
			res.Prize = res.Prizes[0].PrizeName
		}
	}
	if res.Success && res.Prize == "" {
		res.Prize = "中奖"
	}
	return res, nil
}

func (MarchWheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return MarchWheelGame{}.spin(ctx, hc, baseURL, cr)
}

// ── 不可躺 抽奖（www.tangpt.top） ─────────────────────────
// POST /web/omnibot/lottery/draw form {count} → {results, draw_count, user_bonus_after}。
type TangptLotteryGame struct{}

func (TangptLotteryGame) ID() string   { return "tangpt_lottery" }
func (TangptLotteryGame) Name() string { return "幸运抽奖" }

func (TangptLotteryGame) draw(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	u := joinPath(baseURL, "web/omnibot/lottery/draw")
	if count < 1 {
		count = 1
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte("count="+strconv.Itoa(count)), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "omnibot_lottery.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("tangpt draw", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	res := &PlayResult{Success: true, Balance: numField(m, "user_bonus_after"), Message: strField(m, "message"), Raw: m}
	if results, ok := m["results"].([]any); ok {
		res.Prizes = make([]PrizeOutcome, 0, len(results))
		for _, r := range results {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			name := firstNonEmpty(strField(rm, "name"), strField(rm, "prize_name"))
			o := PrizeOutcome{PrizeName: name, Amount: numField(rm, "amount"), IsWin: name != "" && !strings.Contains(name, "谢谢")}
			res.Prizes = append(res.Prizes, o)
			if o.IsWin && res.Prize == "" {
				res.Prize = name
			}
		}
	}
	return res, nil
}

func (TangptLotteryGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return TangptLotteryGame{}.draw(ctx, hc, baseURL, cr, 1)
}

func (TangptLotteryGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	return TangptLotteryGame{}.draw(ctx, hc, baseURL, cr, count)
}
