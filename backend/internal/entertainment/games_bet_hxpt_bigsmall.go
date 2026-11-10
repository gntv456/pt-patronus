// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 好学 猜大小（hxpt.org） ────────────────────────────────
// games/bigsmall/ajax.php?action=：game_status / bet / add_bet / trends / rankings /
// game_history / my_items / my_tasks / chat_get / chat_send / claim_task / claim_reward /
// claim_refund / redpacket_send / redpacket_claim / redpacket_info / get_captcha。
// 信封 {code,data,msg}（code 0=ok, 1=csrf失效, 2=冷却, 3=需验证码）；POST 带 csrf_token（抠 bigsmall.php 的 data-csrf-token）。
// 开奖为到点自动 settle（last_game_result 在 game_status，刷新查看）。
type HxptBigSmallGame struct{}

func (HxptBigSmallGame) ID() string   { return "hxpt_bigsmall" }
func (HxptBigSmallGame) Name() string { return "猜大小" }

// hxptBigSmallAjax 调 ajax 端点（GET 只读 / POST 写），返回 (完整信封 m, code, msg)。
// 调用方按需用 hxptBSData(m)（data 为对象）/ hxptBSArr(m)（data 为数组）取 data。
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
	return m, int(longptNum(m["code"])), strField(m, "msg")
}

// hxptBSData 取信封 data（对象形态；数组形态返回 nil）。
func hxptBSData(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return nil
}

// hxptBSArr 取信封 data（数组形态）。
func hxptBSArr(m map[string]any) []any {
	if a, ok := m["data"].([]any); ok {
		return a
	}
	return nil
}

// hxptBSCsrf 抠 bigsmall.php 的 data-csrf-token（写操作前置）。
func hxptBSCsrf(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) string {
	page := joinPath(baseURL, "bigsmall.php")
	pr, err := hc.GetWithHeaders(ctx, page, cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return ""
	}
	return firstGroup(pr.Body, `data-csrf-token="([^"]+)"`)
}

func (HxptBigSmallGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	csrf := hxptBSCsrf(ctx, hc, baseURL, cr.Cookie)

	m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "game_status", cr.Cookie, "", "")
	data := hxptBSData(m)
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
	amount := req.Amount
	if amount <= 0 {
		amount = numField(data, "min_bet")
	}
	if amount <= 0 {
		amount = 100
	}
	// 上限钳位：优先用站点 max_bet，再兜底硬上限（防超大值透传）。
	if mb := numField(data, "max_bet"); mb > 0 && amount > mb {
		amount = mb
	}
	if amount > maxBetCeiling {
		amount = maxBetCeiling
	}
	amtStr := strconv.FormatFloat(amount, 'f', -1, 64)

	addBet := req.Extra["add_bet"] == "1"
	var action, body string
	if addBet {
		// 追加：action=add_bet，仅 game_id/amount/round_token（无 bet_type/prop_id）。
		action = "add_bet"
		body = "game_id=" + strconv.Itoa(gameID) + "&amount=" + amtStr + "&round_token=" + roundToken + "&csrf_token=" + csrf
	} else {
		betType := req.Selection
		if betType != "big" && betType != "small" {
			betType = "small"
		}
		propID := strings.TrimSpace(req.Extra["prop_id"])
		if propID == "" {
			propID = "0"
		}
		action = "bet"
		body = "game_id=" + strconv.Itoa(gameID) + "&bet_type=" + betType + "&amount=" + amtStr +
			"&prop_id=" + propID + "&round_token=" + roundToken + "&csrf_token=" + csrf
		if h := req.Extra["captcha_hash"]; h != "" {
			body += "&captcha_hash=" + h + "&captcha_code=" + req.Extra["captcha_code"]
		}
	}
	bm, code, msg := hxptBigSmallAjax(ctx, hc, baseURL, action, cr.Cookie, csrf, body)
	if code == 0 {
		var prize string
		if !addBet {
			betType := req.Selection
			if betType != "big" && betType != "small" {
				betType = "small"
			}
			prize = "押 " + betType + " " + amtStr
		} else {
			prize = "追加 " + amtStr
		}
		return &PlayResult{
			Success: true,
			Balance: numField(hxptBSData(bm), "balance"),
			Prize:   prize,
			Message: "下注成功，等待开奖（刷新查看结果）",
		}, nil
	}
	if code == 3 {
		// 需要验证码：前端弹框收 code，带 captcha_hash/code 重下。
		d := hxptBSData(bm)
		return &PlayResult{
			Success: false, Balance: balance,
			Message: firstNonEmpty(msg, "需要验证码"),
			Raw:     map[string]any{"captcha_required": true, "image": strField(d, "image"), "imagehash": strField(d, "imagehash")},
		}, nil
	}
	return &PlayResult{Success: false, Balance: balance, Message: firstNonEmpty(msg, "下注失败")}, nil
}

func (HxptBigSmallGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return HxptBigSmallGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{Selection: "small"})
}

func (HxptBigSmallGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "game_status", cr.Cookie, "", "")
	data := hxptBSData(m)
	if data == nil {
		return &StatsResult{}, nil
	}
	return &StatsResult{Balance: numField(data, "balance"), CostPerPlay: numField(data, "min_bet")}, nil
}

// BigSmallState 聚合当前局/走势/排行/记录/任务/道具/聊天（红包好学嵌在聊天里，此处不单独取）。
func (HxptBigSmallGame) BigSmallState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*BigSmallState, error) {
	st := &BigSmallState{Currency: "火花", Payout: "1.7", HasHouse: true,
		RecentResults: []string{}, Rankings: []BigSmallRank{}, MyRecords: []BigSmallRecord{}, AllRecords: []BigSmallRecord{},
		Tasks: []BigSmallTask{}, Props: []BigSmallProp{}, Chat: []BigSmallChat{}, Redpackets: []BigSmallRedpacket{}}

	if m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "game_status", cr.Cookie, "", ""); m != nil {
		if d := hxptBSData(m); d != nil {
			st.Balance = numField(d, "balance")
			st.MinBet = numField(d, "min_bet")
			st.MaxBet = numField(d, "max_bet")
			if g, ok := d["game"].(map[string]any); ok {
				st.Round = BigSmallRound{
					ID:            int(longptNum(g["id"])),
					GameNo:        firstNonEmpty(strField(g, "game_no"), numToStr(g["game_no"])),
					StartTime:     strField(g, "start_time"),
					EndTime:       strField(g, "end_time"),
					Status:        strField(g, "status"),
					Countdown:     int(longptNum(g["countdown"])),
					SealCountdown: int(longptNum(g["seal_countdown"])),
					PlayerCount:   int(longptNum(g["player_count"])),
					TotalBet:      numField(g, "total_bet"),
				}
			}
			if mb, ok := d["my_bet"].(map[string]any); ok {
				st.MyBet = &BigSmallMyBet{BetType: strField(mb, "bet_type"), Amount: numField(mb, "amount")}
			}
			if lr, ok := d["last_game_result"].(map[string]any); ok {
				st.LastResult = &BigSmallMyBet{
					GameNo:    firstNonEmpty(strField(lr, "game_no"), numToStr(lr["game_no"])),
					BetType:   strField(lr, "bet_type"),
					Amount:    numField(lr, "amount"),
					Result:    strField(lr, "result"),
					NetResult: numField(lr, "result_amount"),
				}
			}
			st.BigBets = hxptBettors(d["big_bets"])
			st.SmallBets = hxptBettors(d["small_bets"])
		}
	}
	if m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "trends", cr.Cookie, "", ""); m != nil {
		if d := hxptBSData(m); d != nil {
			st.Trend40 = hxptTrend(d["stats40"])
			st.Trend10 = hxptTrend(d["stats10"])
			st.TrendText = strField(d, "trend_text")
			if rr, ok := d["recent_results"].([]any); ok {
				out := make([]string, 0, len(rr))
				for _, v := range rr {
					out = append(out, numToStr(v))
				}
				st.RecentResults = out
			}
		}
	}
	if m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "rankings", cr.Cookie, "", ""); m != nil {
		if d := hxptBSData(m); d != nil {
			st.Rankings = hxptRankings(d)
		}
	}
	if m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "game_history&type=my&limit=10", cr.Cookie, "", ""); m != nil {
		st.MyRecords = hxptMyRecords(hxptBSArr(m))
	}
	if m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "game_history&type=all&limit=20", cr.Cookie, "", ""); m != nil {
		st.AllRecords = hxptAllRecords(hxptBSArr(m))
	}
	if m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "my_tasks", cr.Cookie, "", ""); m != nil {
		st.Tasks = hxptTasks(hxptBSArr(m))
	}
	if m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "my_items", cr.Cookie, "", ""); m != nil {
		if d := hxptBSData(m); d != nil {
			st.Props = hxptProps(d["items"])
		}
	}
	if m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "chat_get&last_id=0", cr.Cookie, "", ""); m != nil {
		st.Chat = hxptChat(hxptBSArr(m))
	}
	return st, nil
}

// BigSmallChat 拉新聊天（since 之后的；站点 chat_get&last_id=N 返回 id>N 的消息）。
func (HxptBigSmallGame) BigSmallChat(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, since int) ([]BigSmallChat, error) {
	m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "chat_get&last_id="+strconv.Itoa(since), cr.Cookie, "", "")
	return hxptChat(hxptBSArr(m)), nil
}

// BigSmallSendChat 发聊天（chat_send）。
func (HxptBigSmallGame) BigSmallSendChat(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, message string) (*PlayResult, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return &PlayResult{Success: false, Message: "内容不能为空"}, nil
	}
	csrf := hxptBSCsrf(ctx, hc, baseURL, cr.Cookie)
	body := "message=" + url.QueryEscape(message) + "&csrf_token=" + csrf
	_, code, msg := hxptBigSmallAjax(ctx, hc, baseURL, "chat_send", cr.Cookie, csrf, body)
	ok := code == 0
	resMsg := firstNonEmpty(msg, "发送失败")
	if ok {
		resMsg = "发送成功"
	}
	return &PlayResult{Success: ok, Message: resMsg}, nil
}

// BigSmallTaskAction 任务操作：claim（领取/开始）/ reward（领奖）/ refund（亏返卡，taskID 忽略）。
func (HxptBigSmallGame) BigSmallTaskAction(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string, taskID int) (*PlayResult, error) {
	csrf := hxptBSCsrf(ctx, hc, baseURL, cr.Cookie)
	var act, body string
	switch action {
	case "reward":
		act = "claim_reward"
		body = "task_id=" + strconv.Itoa(taskID) + "&csrf_token=" + csrf
	case "refund":
		act = "claim_refund"
		body = "csrf_token=" + csrf
	default:
		act = "claim_task"
		body = "task_id=" + strconv.Itoa(taskID) + "&csrf_token=" + csrf
	}
	_, code, msg := hxptBigSmallAjax(ctx, hc, baseURL, act, cr.Cookie, csrf, body)
	ok := code == 0
	resMsg := firstNonEmpty(msg, "操作失败")
	if ok {
		resMsg = "操作成功"
	}
	return &PlayResult{Success: ok, Message: resMsg}, nil
}

// BigSmallUseItem 好学道具随注选择（prop_id）；仅亏返卡类可独立使用 → claim_refund。
func (HxptBigSmallGame) BigSmallUseItem(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, tag string) (*PlayResult, error) {
	if tag == "refund" || strings.Contains(tag, "refund") || strings.Contains(tag, "loss_refund") {
		return HxptBigSmallGame{}.BigSmallTaskAction(ctx, hc, baseURL, cr, "refund", 0)
	}
	return &PlayResult{Success: false, Message: "好学道具随注选择（下注时选道具），无需独立使用"}, nil
}

// BigSmallRedpacket 红包：send（total_amount/packet_count/greeting）/ claim（packet_id）/ info（packet_id）。
func (HxptBigSmallGame) BigSmallRedpacket(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string, extra map[string]string) (*PlayResult, error) {
	if action == "info" {
		m, _, _ := hxptBigSmallAjax(ctx, hc, baseURL, "redpacket_info&packet_id="+extra["id"], cr.Cookie, "", "")
		d := hxptBSData(m)
		if d == nil {
			return &PlayResult{Success: false, Message: "红包不存在或已过期"}, nil
		}
		return &PlayResult{Success: true, Raw: d, Prize: strField(d, "greeting")}, nil
	}
	csrf := hxptBSCsrf(ctx, hc, baseURL, cr.Cookie)
	var act, body string
	switch action {
	case "claim":
		act = "redpacket_claim"
		body = "packet_id=" + extra["id"] + "&csrf_token=" + csrf
	default: // send
		act = "redpacket_send"
		body = "total_amount=" + extra["amount"] + "&packet_count=" + firstNonEmpty(extra["count"], "1") +
			"&greeting=" + url.QueryEscape(extra["greeting"]) + "&csrf_token=" + csrf
	}
	_, code, msg := hxptBigSmallAjax(ctx, hc, baseURL, act, cr.Cookie, csrf, body)
	ok := code == 0
	resMsg := firstNonEmpty(msg, "操作失败")
	if ok {
		resMsg = "操作成功"
	}
	return &PlayResult{Success: ok, Message: resMsg}, nil
}
