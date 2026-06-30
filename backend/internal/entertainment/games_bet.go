package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 下注/选号类娱乐（Bettable）：猜大小/骰宝/双色球/翻面/Betting/红包。纯 Cookie。
// 各 Game 实现 LotteryGame.Play（默认下注兜底）+ Bettable.Bet + Statable。

// ── 好学 猜大小（hxpt.org） ────────────────────────────────
// games/bigsmall/ajax.php?action=：game_status（取 round_token+game.id+balance）
// → POST action=bet {game_id,bet_type,amount,round_token,csrf_token}。响应 {code:0,data:{...}} / {code:1,msg}。
// csrf 抠 bigsmall.php 的 data-csrf-token；开奖为到点自动 settle（last_game_result 在 game_status，刷新查看）。
type HxptBigSmallGame struct{}

func (HxptBigSmallGame) ID() string   { return "hxpt_bigsmall" }
func (HxptBigSmallGame) Name() string { return "猜大小" }

// hxptBigSmallAjax 调 ajax 端点（GET 只读 / POST 写），返回 (data, code, msg)。
func hxptBigSmallAjax(ctx context.Context, hc *httpclient.Client, baseURL, action, cookie, csrf, postBody string) (map[string]any, int, string) {
	u := joinPath(baseURL, "games/bigsmall/ajax.php") + "?action=" + action
	method := http.MethodGet
	var body []byte
	headers := map[string]string{"X-Requested-With": "XMLHttpRequest", "Referer": joinPath(baseURL, "bigsmall.php")}
	if postBody != "" {
		method = http.MethodPost
		body = []byte(postBody)
		headers["Content-Type"] = "application/x-www-form-urlencoded"
		if csrf != "" {
			headers["X-CSRF-Token"] = csrf
		}
	}
	resp, err := hc.Do(ctx, method, u, body, headers, cookie, true)
	if err != nil {
		return nil, -1, err.Error()
	}
	logResp("hxpt_bigsmall "+action, u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return nil, -1, "parse fail"
	}
	code := int(longptNum(m["code"]))
	data, _ := m["data"].(map[string]any)
	return data, code, strField(m, "msg")
}

func (HxptBigSmallGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	page := joinPath(baseURL, "bigsmall.php")
	pr, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	csrf := firstGroup(pr.Body, `data-csrf-token="([^"]+)"`)

	data, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "game_status", cr.Cookie, "", "")
	if data == nil {
		return &PlayResult{Success: false, Message: "获取局信息失败，可能未登录"}, nil
	}
	game, _ := data["game"].(map[string]any)
	gameID := int(longptNum(game["id"]))
	roundToken := strField(data, "round_token")
	balance := numField(data, "balance")
	status := strField(game, "status")
	if status != "" && status != "betting" {
		return &PlayResult{Success: false, Balance: balance, Message: "当前不在下注阶段（" + status + "）"}, nil
	}
	if roundToken == "" || gameID == 0 {
		return &PlayResult{Success: false, Balance: balance, Message: "缺少 round_token / game_id"}, nil
	}
	betType := req.Selection
	if betType != "big" && betType != "small" {
		betType = "small"
	}
	amount := req.Amount
	if amount <= 0 {
		amount = numField(data, "min_bet")
	}
	if amount <= 0 {
		amount = 100
	}
	body := "game_id=" + strconv.Itoa(gameID) + "&bet_type=" + betType + "&amount=" +
		strconv.FormatFloat(amount, 'f', -1, 64) + "&round_token=" + roundToken + "&csrf_token=" + csrf
	bdata, code, msg := hxptBigSmallAjax(ctx, hc, baseURL, "bet", cr.Cookie, csrf, body)
	if code == 0 {
		return &PlayResult{
			Success: true,
			Balance: numField(bdata, "balance"),
			Prize:   "押 " + betType + " " + strconv.FormatFloat(amount, 'f', -1, 64),
			Message: "下注成功，等待开奖（刷新查看结果）",
		}, nil
	}
	return &PlayResult{Success: false, Balance: balance, Message: firstNonEmpty(msg, "下注失败")}, nil
}

func (HxptBigSmallGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return HxptBigSmallGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{Selection: "small"})
}

func (HxptBigSmallGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	data, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "game_status", cr.Cookie, "", "")
	if data == nil {
		return &StatsResult{}, nil
	}
	return &StatsResult{Balance: numField(data, "balance"), CostPerPlay: numField(data, "min_bet")}, nil
}

// ── CBG 骰宝竞技场（cangbao.ge） ──────────────────────────
// POST api/api_sicbo.php {amount,type} → {status,new_magic_point,message}（复用 cbgApiPost）。
type CbgSicboGame struct{}

func (CbgSicboGame) ID() string   { return "cbg_sicbo" }
func (CbgSicboGame) Name() string { return "骰宝竞技场" }

func (CbgSicboGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	sel := req.Selection
	if sel == "" {
		sel = "big"
	}
	amt := req.Amount
	if amt <= 0 {
		amt = 100
	}
	m, err := cbgApiPost(ctx, hc, baseURL, "api/api_sicbo.php", "sicboarena.php", cr.Cookie, map[string]any{"amount": amt, "type": sel})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	return &PlayResult{
		Success: strField(m, "status") == "success",
		Balance: numField(m, "new_magic_point"),
		Prize:   "押 " + sel + " " + strconv.FormatFloat(amt, 'f', -1, 64),
		Message: strField(m, "message"),
		Raw:     m,
	}, nil
}

func (CbgSicboGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return CbgSicboGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{Selection: "big", Amount: 100})
}

func (CbgSicboGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "sicboarena.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: cbgBalance(resp.Body)}, nil
}

// ── 蟹黄堡 双色球（crabpt.vip） ───────────────────────────
// POST /union-lotto {nums:[6红+1蓝]} → {code,m,data}。extra.reds="1,2,3,4,5,6" / extra.blue="7"。
type CrabUnionLottoGame struct{}

func (CrabUnionLottoGame) ID() string   { return "crab_unionlotto" }
func (CrabUnionLottoGame) Name() string { return "双色球" }

func (CrabUnionLottoGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	nums := make([]string, 0, 7)
	for _, r := range strings.Split(req.Extra["reds"], ",") {
		if r = strings.TrimSpace(r); r != "" {
			nums = append(nums, r)
		}
	}
	if b := strings.TrimSpace(req.Extra["blue"]); b != "" {
		nums = append(nums, b)
	}
	if len(nums) < 7 {
		return &PlayResult{Success: false, Message: "请选 6 红 1 蓝（extra.reds=逗号分隔 / extra.blue）"}, nil
	}
	u := joinPath(baseURL, "union-lotto")
	payload, _ := json.Marshal(map[string]any{"nums": nums})
	resp, err := hc.Do(ctx, http.MethodPost, u, payload, map[string]string{
		"Content-Type":     "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "union-lotto.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("crab_unionlotto", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "解析投注结果失败"}, nil
	}
	return &PlayResult{
		Success: int(longptNum(m["code"])) == 0,
		Message: strField(m, "m"),
		Prize:   strings.Join(nums, ","),
		Raw:     m,
	}, nil
}

func (CrabUnionLottoGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return &PlayResult{Success: false, Message: "双色球需选号：请用 Bet（extra.reds=6红 / extra.blue=1蓝）"}, nil
}

// ── PANDA 翻面夺宝（pandapt.net） ─────────────────────────
// POST /ajax_flip.php form params[round_number,tile_index] → {ret,data:{status,balance,flipResult:{tier,reward}}}。
type PandaFlipGame struct{}

func (PandaFlipGame) ID() string   { return "panda_flip" }
func (PandaFlipGame) Name() string { return "翻面夺宝" }

func (PandaFlipGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	tile := req.Selection
	if tile == "" {
		tile = "0"
	}
	round := req.Extra["round_number"]
	if round == "" {
		round = "1"
	}
	u := joinPath(baseURL, "ajax_flip.php")
	body := "params[round_number]=" + round + "&params[tile_index]=" + tile
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(body), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "flip.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("panda_flip", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "解析翻面结果失败"}, nil
	}
	res := &PlayResult{Success: int(longptNum(m["ret"])) == 0, Raw: m}
	if data, ok := m["data"].(map[string]any); ok {
		res.Balance = numField(data, "balance")
		if fr, ok := data["flipResult"].(map[string]any); ok {
			res.Prize = strField(fr, "tier")
			if rw := strField(fr, "reward"); rw != "" {
				res.Prize += " " + rw
			}
		}
	}
	return res, nil
}

func (PandaFlipGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return PandaFlipGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{Selection: "0"})
}

// ── OurBits 赛事押注（ourbits.club） ──────────────────────
// bet.php 列表 → bet_odds.php?id=N 押注（整页 HTML）。押注表单字段待补抓 bet_odds.php，Bet 暂占位。
type OurbitsBetGame struct{}

func (OurbitsBetGame) ID() string   { return "ourbits_bet" }
func (OurbitsBetGame) Name() string { return "赛事押注" }

func (OurbitsBetGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	return &PlayResult{Success: false, Message: "押注表单待补抓 bet_odds.php（selection=赛事id=" + req.Selection + " 已知，押注字段待真实页面校准）"}, nil
}

func (OurbitsBetGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return OurbitsBetGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{})
}

// ── UBits 红包（ubits.club） ──────────────────────────────
// POST redpacket.php form action=send|claim（整页 HTML，无 JSON）。send: amount=总额+extra(packet_type/distribution_type/packet_count)；claim: extra.id。
type UbitsRedpacketGame struct{}

func (UbitsRedpacketGame) ID() string   { return "ubits_redpacket" }
func (UbitsRedpacketGame) Name() string { return "红包" }

func (UbitsRedpacketGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	action := req.Selection
	if action == "" {
		action = "claim"
	}
	u := joinPath(baseURL, "redpacket.php")
	var body string
	if action == "send" {
		body = "action=send&amount=" + strconv.FormatFloat(req.Amount, 'f', -1, 64) +
			"&packet_type=" + req.Extra["packet_type"] + "&distribution_type=" + req.Extra["distribution_type"] +
			"&packet_count=" + req.Extra["packet_count"]
	} else {
		body = "action=claim&id=" + req.Extra["id"]
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(body), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Referer":      u,
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("ubits_redpacket "+action, u, resp)
	msg := strings.TrimSpace(stripTags(firstGroup(resp.Body, `(?s)(成功|失败|领取|已|红包)[^<]{0,40}`)))
	return &PlayResult{
		Success: true,
		Balance: novaHDNum(firstGroup(resp.Body, `U币[^0-9-]*([\d,.]+)`)),
		Prize:   action,
		Message: firstNonEmpty(msg, action+" 操作已提交"),
	}, nil
}

func (UbitsRedpacketGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return UbitsRedpacketGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{Selection: "claim"})
}
