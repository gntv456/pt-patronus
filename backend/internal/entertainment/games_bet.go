package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 下注/选号类娱乐（Bettable）：猜大小/骰宝/双色球/翻面/Betting/红包。纯 Cookie。
// 各 Game 实现 LotteryGame.Play（默认下注兜底）+ Bettable.Bet + Statable。

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
		prize := "押 " + amtStr
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

// ── 好学猜大小 解析助手 ──────────────────────────────────

func hxptTrend(v any) BigSmallTrend {
	m, _ := v.(map[string]any)
	if m == nil {
		return BigSmallTrend{}
	}
	return BigSmallTrend{
		BigCount:   int(longptNum(m["big"])),
		SmallCount: int(longptNum(m["small"])),
		HouseCount: int(longptNum(m["house"])),
		BigPct:     int(longptNum(m["big_pct"])),
		SmallPct:   int(longptNum(m["small_pct"])),
	}
}

func hxptBettors(v any) []BigSmallBetSpot {
	arr, _ := v.([]any)
	out := make([]BigSmallBetSpot, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallBetSpot{Username: strField(m, "username"), Amount: numField(m, "amount")})
	}
	return out
}

// hxptRankTabs 榜单 tab→数据键/金额键/次数键（对齐站点 JS bigsmall.js:690-701 动态取键）。
var hxptRankTabs = []struct{ key, dataKey, title, amtKey, cntKey string }{
	{"monthly_king", "monthly_king", "本月赌神", "total_won", "king_count"},
	{"daily_king", "daily_kings", "今日赌王", "total_won", "king_count"},
	{"monthly_charity", "monthly_charity", "本月慈善", "total_lost", "charity_count"},
	{"daily_charity", "daily_charity", "今日慈善", "total_lost", "charity_count"},
	{"monthly_charity_king", "monthly_charity_king", "慈善之神", "net_amount", "game_count"},
	{"daily_charity_king", "daily_charity_king", "慈善大王", "net_amount", "game_count"},
}

func hxptRankings(d map[string]any) []BigSmallRank {
	out := make([]BigSmallRank, 0, len(hxptRankTabs))
	for _, t := range hxptRankTabs {
		arr, _ := d[t.dataKey].([]any)
		if len(arr) == 0 {
			continue
		}
		r := BigSmallRank{Key: t.key, Title: t.title, Entries: make([]BigSmallRankEntry, 0, len(arr))}
		for _, it := range arr {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			r.Entries = append(r.Entries, BigSmallRankEntry{
				Username: strField(m, "username"),
				Amount:   numField(m, t.amtKey),
				Count:    int(longptNum(m[t.cntKey])),
			})
		}
		out = append(out, r)
	}
	return out
}

func hxptMyRecords(arr []any) []BigSmallRecord {
	out := make([]BigSmallRecord, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallRecord{
			GameNo:      firstNonEmpty(strField(m, "game_no"), numToStr(m["game_no"])),
			StartTime:   strField(m, "start_time"),
			EndTime:     strField(m, "end_time"),
			PlayerCount: int(longptNum(m["player_count"])),
			TotalBet:    numField(m, "total_bet"),
			Result:      strField(m, "result"),
			MyBetType:   strField(m, "my_bet_type"),
			MyBet:       numField(m, "my_bet"),
			MyResult:    numField(m, "my_result"),
		})
	}
	return out
}

func hxptAllRecords(arr []any) []BigSmallRecord {
	out := make([]BigSmallRecord, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		rec := BigSmallRecord{
			GameNo:    firstNonEmpty(strField(m, "game_no"), numToStr(m["game_no"])),
			StartTime: strField(m, "start_time"),
			EndTime:   strField(m, "end_time"),
			Result:    strField(m, "result"),
		}
		if bets, ok := m["bets"].([]any); ok {
			var sb strings.Builder
			for _, b := range bets {
				bm, _ := b.(map[string]any)
				if bm == nil {
					continue
				}
				if sb.Len() > 0 {
					sb.WriteString("；")
				}
				if u := strField(bm, "username"); u != "" {
					sb.WriteString(u)
					sb.WriteString(": ")
				}
				sb.WriteString(strField(bm, "bet_info"))
			}
			rec.Bets = sb.String()
		}
		out = append(out, rec)
	}
	return out
}

func hxptTasks(arr []any) []BigSmallTask {
	out := make([]BigSmallTask, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		status := "idle"
		switch {
		case boolField(m, "can_reward"):
			status = "rewardable"
		case boolField(m, "can_claim"):
			status = "doing"
		case boolField(m, "is_completed"):
			status = "done"
		}
		out = append(out, BigSmallTask{
			ID:           int(longptNum(m["id"])),
			Title:        firstNonEmpty(strField(m, "name"), strField(m, "title")),
			Desc:         strField(m, "description"),
			Reward:       firstNonEmpty(strField(m, "reward_description"), strField(m, "reward_name")),
			ProgressText: strField(m, "progress_text"),
			Progress:     int(longptNum(m["progress"])),
			Status:       status,
			MonthlyCount: int(longptNum(m["monthly_count"])),
			MonthlyLimit: int(longptNum(m["monthly_limit"])),
		})
	}
	return out
}

func hxptProps(v any) []BigSmallProp {
	arr, _ := v.([]any)
	out := make([]BigSmallProp, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallProp{
			ID:       numToStr(m["id"]),
			Tag:      strField(m, "code"),
			Name:     strField(m, "name"),
			Desc:     strField(m, "description"),
			Image:    strField(m, "image"),
			Quantity: int(longptNum(m["quantity"])),
			Usable:   boolField(m, "usable_in_bet"),
		})
	}
	return out
}

func hxptChat(arr []any) []BigSmallChat {
	out := make([]BigSmallChat, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallChat{
			ID:       int(longptNum(m["id"])),
			Username: strField(m, "username"),
			Message:  strField(m, "message"),
			Time:     strField(m, "time"),
		})
	}
	return out
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

// ── PANDA 猜大小（pandapt.net） ───────────────────────────
// /ajax.php POST action=+params[k]=，信封 {ret,data,msg}（ret 0=ok）。无 csrf。
// reloadData 一次拿大头（my_bonus/timeLeft/isBettingOpen/currentGame/last_result 走势/my_list/other_list），
// 其余 task_list/getMyItems/getActiveRedPackets/getLastMsg 各一次轻取。
// 注：熊猫排行（本月赌神/今日赌王…）SSR 烘焙在 bigorsmall.php 页面 Vue data 里，reloadData 可能不含 →
// 此处尽力从 reloadData 取 god/poor/todayRanking/loseRanking 键，缺失则留空（待真实校准）。
type PandaBigSmallGame struct{}

func (PandaBigSmallGame) ID() string   { return "panda_bigsmall" }
func (PandaBigSmallGame) Name() string { return "猜大小" }

// pandaBigSmallAjax 调 /ajax.php（统一 form：action + params[…]），返回完整信封 map。
func pandaBigSmallAjax(ctx context.Context, hc *httpclient.Client, baseURL, action, cookie string, params map[string]string) (map[string]any, error) {
	u := joinPath(baseURL, "ajax.php")
	form := "action=" + action
	for k, v := range params {
		form += "&params[" + k + "]=" + url.QueryEscape(v)
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "bigorsmall.php"),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("panda_bigsmall "+action, u, resp)
	return parseRaw(resp.Body)
}

func pandaBSData(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return nil
}
func pandaBSArr(m map[string]any) []any {
	if a, ok := m["data"].([]any); ok {
		return a
	}
	return nil
}

// pandaStr 取多候选键的首个非空字符串（熊猫 JSON 键名不确定，多键兜底）。
func pandaStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := strField(m, k); v != "" {
			return v
		}
	}
	return ""
}

// numFieldMulti 取多候选键的首个非零数值。
func numFieldMulti(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v := numField(m, k); v != 0 {
			return v
		}
	}
	return 0
}

// pandaResultStr 结果归一为 big/small（1或"大"→big，2或"小"→small）。
func pandaResultStr(m map[string]any) string {
	r := pandaStr(m, "result", "type")
	if r == "" {
		n := numToStr(m["result"])
		if n == "1" {
			return "big"
		}
		if n == "2" {
			return "small"
		}
		return ""
	}
	switch {
	case r == "1" || r == "big" || strings.Contains(r, "大"):
		return "big"
	case r == "2" || r == "small" || strings.Contains(r, "小"):
		return "small"
	}
	return r
}

// pandaJoinBetType 解析 currentGame.join（"大 : 1000"/"小 : 500"/"未参与"）→ bet_type。
func pandaJoinBetType(join string) string {
	s := strings.TrimSpace(join)
	if strings.HasPrefix(s, "大") {
		return "big"
	}
	if strings.HasPrefix(s, "小") {
		return "small"
	}
	return ""
}

// pandaJoinAmount 从 join 串尾抠金额（"大 : 1,000" → 1000）。
func pandaJoinAmount(join string) float64 {
	s := strings.ReplaceAll(join, ",", "")
	f, _ := strconv.ParseFloat(firstGroup([]byte(s), `([\d.]+)\s*$`), 64)
	return f
}

// pandaTrend 由 last_result（二维数组，1=大）算 40场/最近10场 统计 + 走势点阵。
func pandaTrend(v any) (BigSmallTrend, BigSmallTrend, []string) {
	flat := make([]any, 0)
	if arr, ok := v.([]any); ok {
		for _, row := range arr {
			if r, ok := row.([]any); ok {
				flat = append(flat, r...)
			} else {
				flat = append(flat, row)
			}
		}
	}
	if len(flat) == 0 {
		return BigSmallTrend{}, BigSmallTrend{}, []string{}
	}
	bigAll := 0
	for _, x := range flat {
		if numToStr(x) == "1" {
			bigAll++
		}
	}
	total := len(flat)
	t40 := BigSmallTrend{BigCount: bigAll, SmallCount: total - bigAll}
	if total > 0 {
		t40.BigPct = bigAll * 100 / total
		t40.SmallPct = 100 - t40.BigPct
	}
	n := 10
	if n > total {
		n = total
	}
	big10 := 0
	for _, x := range flat[:n] {
		if numToStr(x) == "1" {
			big10++
		}
	}
	t10 := BigSmallTrend{BigCount: big10, SmallCount: n - big10}
	if n > 0 {
		t10.BigPct = big10 * 100 / n
		t10.SmallPct = 100 - t10.BigPct
	}
	rr := make([]string, 0, total)
	for _, x := range flat {
		if numToStr(x) == "1" {
			rr = append(rr, "big")
		} else {
			rr = append(rr, "small")
		}
	}
	return t40, t10, rr
}

func pandaBettors(v any) []BigSmallBetSpot {
	arr, _ := v.([]any)
	out := make([]BigSmallBetSpot, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallBetSpot{
			Username: pandaStr(m, "username", "name"),
			Amount:   numFieldMulti(m, "amount", "bonus", "bet_amount"),
		})
	}
	return out
}

func pandaMyRecords(v any) []BigSmallRecord {
	arr, _ := v.([]any)
	out := make([]BigSmallRecord, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallRecord{
			GameNo:      pandaStr(m, "game_no", "gameNo", "id"),
			StartTime:   pandaStr(m, "start_time", "startTime"),
			EndTime:     pandaStr(m, "end_time", "endTime"),
			PlayerCount: int(longptNum(m["player_count"])),
			TotalBet:    numFieldMulti(m, "total_bet", "total_bet_amount"),
			Result:      pandaResultStr(m),
			MyBetType:   pandaJoinBetType(pandaStr(m, "my_bet_type", "my_bet", "join")),
			MyBet:       numFieldMulti(m, "my_bet", "my_amount", "bet"),
			MyResult:    numFieldMulti(m, "my_result", "result_amount", "net"),
		})
	}
	return out
}

func pandaAllRecords(v any) []BigSmallRecord {
	arr, _ := v.([]any)
	out := make([]BigSmallRecord, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		rec := BigSmallRecord{
			GameNo:    pandaStr(m, "game_no", "gameNo", "id"),
			StartTime: pandaStr(m, "start_time", "startTime"),
			EndTime:   pandaStr(m, "end_time", "endTime"),
			Result:    pandaResultStr(m),
		}
		if bets := pandaStr(m, "bets", "bet_info"); bets != "" {
			rec.Bets = bets
		}
		out = append(out, rec)
	}
	return out
}

// pandaRankings 从 reloadData 取 god/poor/todayRanking/loseRanking（SSR 数据若在 reload 响应里则填）。
func pandaRankings(d map[string]any) []BigSmallRank {
	tabs := []struct{ key, title, amtKey, cntKey string }{
		{"god", "本月赌神", "total_result", "total_count"},
		{"todayRanking", "今日赌王", "today_net_profit", "today_bets"},
		{"poor", "本月慈善", "total_result", "total_count"},
		{"loseRanking", "今日慈善", "today_net_profit", "today_bets"},
	}
	out := make([]BigSmallRank, 0, 4)
	for _, t := range tabs {
		var arr []any
		if a, ok := d[t.key].([]any); ok {
			arr = a
		} else if om, ok := d[t.key].(map[string]any); ok {
			arr = []any{om} // god/poor 单对象
		}
		if len(arr) == 0 {
			continue
		}
		r := BigSmallRank{Key: t.key, Title: t.title, Entries: make([]BigSmallRankEntry, 0, len(arr))}
		for _, it := range arr {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			r.Entries = append(r.Entries, BigSmallRankEntry{
				Username: pandaStr(m, "username", "name"),
				Amount:   numFieldMulti(m, t.amtKey, "amount"),
				Count:    int(longptNum(m[t.cntKey])),
			})
		}
		out = append(out, r)
	}
	return out
}

func pandaTasks(arr []any) []BigSmallTask {
	out := make([]BigSmallTask, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		status := "idle"
		switch int(longptNum(m["task_status"])) {
		case 1:
			status = "doing"
		case 3:
			status = "done"
		}
		if status == "idle" && boolField(m, "can_reward") {
			status = "rewardable"
		}
		out = append(out, BigSmallTask{
			ID:           int(longptNum(m["task_id"])),
			Title:        pandaStr(m, "title", "name"),
			Desc:         pandaStr(m, "desc", "description"),
			Reward:       pandaStr(m, "reward", "reward_name"),
			ProgressText: pandaStr(m, "progress_text", "progress"),
			Progress:     int(longptNum(m["progress"])),
			Status:       status,
		})
	}
	return out
}

func pandaProps(arr []any) []BigSmallProp {
	out := make([]BigSmallProp, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallProp{
			ID:       pandaStr(m, "id", "unique_tag"),
			Tag:      pandaStr(m, "unique_tag", "tag"),
			Name:     pandaStr(m, "title", "name"),
			Desc:     pandaStr(m, "desc", "description"),
			Image:    pandaStr(m, "image", "icon"),
			Quantity: int(longptNum(m["count"])),
			Usable:   true, // 熊猫道具独立 useItem 使用
		})
	}
	return out
}

func pandaRedpackets(arr []any) []BigSmallRedpacket {
	out := make([]BigSmallRedpacket, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallRedpacket{
			ID:    pandaStr(m, "baseId", "id"),
			Total: numField(m, "total"),
			Count: int(longptNum(m["count"])),
		})
	}
	return out
}

func pandaChat(arr []any) []BigSmallChat {
	out := make([]BigSmallChat, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, BigSmallChat{
			ID:       int(longptNum(m["id"])),
			Username: pandaStr(m, "username", "name"),
			Message:  pandaStr(m, "content", "message"),
			Time:     pandaStr(m, "timestamp", "time"),
		})
	}
	return out
}

func (PandaBigSmallGame) BigSmallState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*BigSmallState, error) {
	st := &BigSmallState{Currency: "魔力值", Payout: "1.95", HasHouse: false, MinBet: 100,
		RecentResults: []string{}, Rankings: []BigSmallRank{}, MyRecords: []BigSmallRecord{}, AllRecords: []BigSmallRecord{},
		Tasks: []BigSmallTask{}, Props: []BigSmallProp{}, Chat: []BigSmallChat{}, Redpackets: []BigSmallRedpacket{}}

	if m, err := pandaBigSmallAjax(ctx, hc, baseURL, "reloadData", cr.Cookie, nil); err == nil {
		if d := pandaBSData(m); d != nil {
			st.Balance = numField(d, "my_bonus")
			st.MaxBet = numField(d, "max_bonus")
			timeLeft := int(longptNum(d["timeLeft"]))
			open := boolField(d, "isBettingOpen")
			switch {
			case open:
				st.Round.Status = "betting"
			case timeLeft > 0:
				st.Round.Status = "sealed"
			default:
				st.Round.Status = "finished"
			}
			st.Round.Countdown = timeLeft
			seal := timeLeft - 3 // 对齐站点 JS：timeLeft≤3 封盘
			if seal < 0 {
				seal = 0
			}
			st.Round.SealCountdown = seal
			if cg, ok := d["currentGame"].(map[string]any); ok {
				st.Round.ID = int(longptNum(cg["id"]))
				st.Round.GameNo = numToStr(cg["id"])
				if join := pandaStr(cg, "join", "my_join"); join != "" && join != "未参与" {
					if bt := pandaJoinBetType(join); bt != "" {
						amt := numFieldMulti(cg, "amount", "my_amount")
						if amt == 0 {
							amt = pandaJoinAmount(join)
						}
						st.MyBet = &BigSmallMyBet{BetType: bt, Amount: amt}
					}
				}
			}
			st.Trend40, st.Trend10, st.RecentResults = pandaTrend(d["last_result"])
			st.MyRecords = pandaMyRecords(d["my_list"])
			st.AllRecords = pandaAllRecords(d["other_list"])
			st.Rankings = pandaRankings(d)
		}
	}
	if m, err := pandaBigSmallAjax(ctx, hc, baseURL, "get_current_users", cr.Cookie, nil); err == nil {
		if d := pandaBSData(m); d != nil {
			st.BigBets = pandaBettors(d["big"])
			st.SmallBets = pandaBettors(d["small"])
		}
	}
	if m, err := pandaBigSmallAjax(ctx, hc, baseURL, "task_list", cr.Cookie, nil); err == nil {
		st.Tasks = pandaTasks(pandaBSArr(m))
	}
	if m, err := pandaBigSmallAjax(ctx, hc, baseURL, "getMyItems", cr.Cookie, nil); err == nil {
		st.Props = pandaProps(pandaBSArr(m))
	}
	if m, err := pandaBigSmallAjax(ctx, hc, baseURL, "getActiveRedPackets", cr.Cookie, nil); err == nil {
		st.Redpackets = pandaRedpackets(pandaBSArr(m))
	}
	if m, err := pandaBigSmallAjax(ctx, hc, baseURL, "getLastMsg", cr.Cookie, nil); err == nil {
		st.Chat = pandaChat(pandaBSArr(m))
	}
	return st, nil
}

func (PandaBigSmallGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	typeNum := "2" // 小
	label := "小"
	if req.Selection == "big" {
		typeNum = "1"
		label = "大"
	}
	amount := req.Amount
	if amount <= 0 {
		amount = 100
	}
	gameID := strings.TrimSpace(req.Extra["game_id"])
	if gameID == "" {
		if m, err := pandaBigSmallAjax(ctx, hc, baseURL, "reloadData", cr.Cookie, nil); err == nil {
			if d := pandaBSData(m); d != nil {
				if cg, ok := d["currentGame"].(map[string]any); ok {
					gameID = numToStr(cg["id"])
				}
			}
		}
	}
	if gameID == "" || gameID == "0" {
		return &PlayResult{Success: false, Message: "未取到当前局 id，可能已封盘"}, nil
	}
	m, err := pandaBigSmallAjax(ctx, hc, baseURL, "saveBet", cr.Cookie, map[string]string{
		"amount": strconv.FormatFloat(amount, 'f', -1, 64),
		"type":   typeNum,
		"id":     gameID,
	})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	ok := int(longptNum(m["ret"])) == 0
	msg := strField(m, "msg")
	balance := 0.0
	if d := pandaBSData(m); d != nil {
		balance = numField(d, "my_bonus")
	}
	if ok && msg == "" {
		msg = "下注成功，等待开奖"
	}
	return &PlayResult{
		Success: ok,
		Balance: balance,
		Prize:   "押 " + label + " " + strconv.FormatFloat(amount, 'f', -1, 64),
		Message: firstNonEmpty(msg, "下注失败"),
		Raw:     m,
	}, nil
}

func (PandaBigSmallGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return PandaBigSmallGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{Selection: "big", Amount: 100})
}

func (PandaBigSmallGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	m, err := pandaBigSmallAjax(ctx, hc, baseURL, "reloadData", cr.Cookie, nil)
	if err != nil {
		return &StatsResult{}, nil
	}
	if d := pandaBSData(m); d != nil {
		return &StatsResult{Balance: numField(d, "my_bonus"), CostPerPlay: 100}, nil
	}
	return &StatsResult{}, nil
}

// BigSmallChat 拉最近聊天（熊猫 getLastMsg 无游标；since 忽略，前端按 id 去重）。
func (PandaBigSmallGame) BigSmallChat(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, since int) ([]BigSmallChat, error) {
	m, err := pandaBigSmallAjax(ctx, hc, baseURL, "getLastMsg", cr.Cookie, nil)
	if err != nil {
		return []BigSmallChat{}, nil
	}
	return pandaChat(pandaBSArr(m)), nil
}

// BigSmallSendChat 发聊天（sendMsg：userId 数值不确定，best-effort 用 username；待真实校准）。
func (PandaBigSmallGame) BigSmallSendChat(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, message string) (*PlayResult, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return &PlayResult{Success: false, Message: "内容不能为空"}, nil
	}
	m, err := pandaBigSmallAjax(ctx, hc, baseURL, "sendMsg", cr.Cookie, map[string]string{
		"userId":   "0",
		"username": cr.Username,
		"content":  message,
		"timestamp": strconv.FormatInt(time.Now().UnixMilli(), 10),
	})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "发送失败"}, nil
	}
	ok := int(longptNum(m["ret"])) == 0
	return &PlayResult{Success: ok, Message: firstNonEmpty(strField(m, "msg"), "发送成功")}, nil
}

// BigSmallTaskAction getTask（claim）/ getReward（reward）；refund 熊猫无对应。
func (PandaBigSmallGame) BigSmallTaskAction(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string, taskID int) (*PlayResult, error) {
	act := "getTask"
	if action == "reward" {
		act = "getReward"
	}
	m, err := pandaBigSmallAjax(ctx, hc, baseURL, act, cr.Cookie, map[string]string{"task_id": strconv.Itoa(taskID)})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "操作失败"}, nil
	}
	ok := int(longptNum(m["ret"])) == 0
	msg := strField(m, "msg")
	prize := ""
	if d := pandaBSData(m); d != nil {
		if ok && d["status"] != nil && int(longptNum(d["status"])) != 1 {
			ok = false
		}
		if msg == "" {
			msg = strField(d, "msg")
		}
		if b := numField(d, "back"); b > 0 {
			prize = "返还 " + formatFloat(b) + " 魔力"
		}
	}
	return &PlayResult{Success: ok, Prize: prize, Message: firstNonEmpty(msg, "操作成功")}, nil
}

// BigSmallUseItem useItem(unique_tag)。
func (PandaBigSmallGame) BigSmallUseItem(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, tag string) (*PlayResult, error) {
	m, err := pandaBigSmallAjax(ctx, hc, baseURL, "useItem", cr.Cookie, map[string]string{"unique_tag": tag})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "使用失败"}, nil
	}
	ok := int(longptNum(m["ret"])) == 0
	msg := strField(m, "msg")
	if d := pandaBSData(m); d != nil {
		if ok && d["status"] != nil && int(longptNum(d["status"])) == 0 {
			ok = false
		}
		if msg == "" {
			msg = strField(d, "msg")
		}
	}
	return &PlayResult{Success: ok, Message: firstNonEmpty(msg, "已使用")}, nil
}

// BigSmallRedpacket send（count/total）/ claim（baseId）。
func (PandaBigSmallGame) BigSmallRedpacket(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string, extra map[string]string) (*PlayResult, error) {
	var act string
	params := map[string]string{}
	switch action {
	case "claim":
		act = "claimRedPacket"
		params["baseId"] = extra["id"]
	default:
		act = "sendRedPacket"
		params["count"] = firstNonEmpty(extra["count"], "1")
		params["total"] = extra["amount"]
	}
	m, err := pandaBigSmallAjax(ctx, hc, baseURL, act, cr.Cookie, params)
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "操作失败"}, nil
	}
	ok := int(longptNum(m["ret"])) == 0
	msg := strField(m, "msg")
	balance := 0.0
	if d := pandaBSData(m); d != nil {
		balance = numField(d, "new_bonus")
		if a := numField(d, "amount"); a > 0 {
			msg = firstNonEmpty(msg, "抢到 "+formatFloat(a)+" 魔力")
		}
	}
	return &PlayResult{Success: ok, Balance: balance, Message: firstNonEmpty(msg, "操作成功")}, nil
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
// POST /ajax_flip.php form action=…&params[…]=…：
//   gameState → {ret,data:{roundNumber,balance,costPerFlip,layout:[{index,tierId,flipped}]}}
//   getFlipTiers → {ret,data:[{id,name,reward,reward_type,count,color}]}（6 档合计 100 张）
//   recordFlip + params[round_number]+params[tile_index] → {ret,data:{status,balance,flipResult:{tier,tierId,reward,isSpecial,reward_type}}}
// task_id 只用于旁支任务系统（startTask/flipGetReward），翻格不需要。
type PandaFlipGame struct{}

func (PandaFlipGame) ID() string   { return "panda_flip" }
func (PandaFlipGame) Name() string { return "翻面夺宝" }

// pandaFlipAjax 调 ajax_flip.php（统一 form：action + params[…]），返回解析后的整 map。
func pandaFlipAjax(ctx context.Context, hc *httpclient.Client, baseURL, action, cookie string, params map[string]string) (map[string]any, error) {
	u := joinPath(baseURL, "ajax_flip.php")
	form := "action=" + action
	for k, v := range params {
		form += "&params[" + k + "]=" + v
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "flip.php"),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("panda_flip "+action, u, resp)
	return parseRaw(resp.Body)
}

// pandaFlipData 取 m["data"]（getFlipTiers 的 data 是数组，此时返回 nil）。
func pandaFlipData(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return nil
}

func (PandaFlipGame) FlipState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FlipState, error) {
	// gameState：round/balance/cost/layout
	gm, err := pandaFlipAjax(ctx, hc, baseURL, "gameState", cr.Cookie, nil)
	state := &FlipState{TotalTiles: 100, Tiers: []FlipTier{}, Tiles: []FlipTile{}, History: []GameRecord{}}
	if err == nil {
		if d := pandaFlipData(gm); d != nil {
			state.RoundNumber = int(longptNum(d["roundNumber"]))
			state.Balance = numField(d, "balance")
			state.CostPerFlip = numField(d, "costPerFlip")
			// 站点 isFlipped(idx) = allFlips.some(f => f.index===idx)：本局所有已翻格 [{index,tierId,tier}]。
			// 优先 allFlips；无则兜底 layout（按位置索引的 tierId 串：layout[i]=第 i+1 格 tierId，空=未翻）。
			if flips, ok := d["allFlips"].([]any); ok {
				for _, it := range flips {
					f, ok := it.(map[string]any)
					if !ok {
						continue
					}
					idx := int(longptNum(f["index"]))
					if idx == 0 {
						continue
					}
					state.Tiles = append(state.Tiles, FlipTile{
						Index:  idx,
						Flipped: true,
						TierID: firstNonEmpty(strField(f, "tierId"), tierIDOf(f)),
						ByUser: firstNonEmpty(strField(f, "user"), strField(f, "playerName"), strField(f, "player")),
					})
					state.FlippedCount++
				}
			} else if layout, ok := d["layout"].([]any); ok {
				for i, it := range layout {
					var tid string
					if s, ok := it.(string); ok {
						tid = s
					} else if m, ok := it.(map[string]any); ok {
						tid = firstNonEmpty(strField(m, "tierId"), tierIDOf(m))
					}
					if tid == "" {
						continue
					}
					state.Tiles = append(state.Tiles, FlipTile{Index: i + 1, Flipped: true, TierID: tid})
					state.FlippedCount++
				}
			}
		}
	}
	// getFlipTiers：6 档奖项
	tm, err := pandaFlipAjax(ctx, hc, baseURL, "getFlipTiers", cr.Cookie, nil)
	if err == nil {
		if arr, ok := tm["data"].([]any); ok {
			tiers := make([]FlipTier, 0, len(arr))
			for _, it := range arr {
				t, ok := it.(map[string]any)
				if !ok {
					continue
				}
				tiers = append(tiers, FlipTier{
					ID:         tierIDOf(t),
					Name:       firstNonEmpty(strField(t, "name"), pandaTierName(t)),
					Reward:     numField(t, "reward"),
					RewardType: strField(t, "reward_type"),
					Count:      int(longptNum(t["count"])),
					Color:      strField(t, "color"),
				})
			}
			if len(tiers) > 0 {
				state.Tiers = tiers
			}
		}
	}
	if state.CostPerFlip == 0 {
		state.CostPerFlip = 2000 // 页面默认 2000 魔力/次
	}
	// 本局翻牌记录：站点仅经 websocket 推送（无 HTTP 端点，初始页「暂无记录」）；
	// 但 gameState.layout 含每个已翻格 {index,tierId,user} —— 已翻格即本局记录，据此忠实重建。
	state.History = pandaFlipHistory(state.Tiles, state.Tiers)
	return state, nil
}

// pandaFlipHistory 由已翻格重建本局记录：每条 "{user} 翻了 #{index} · {tier}" + reward。
// 倒序（最近翻的在前，按 index 降序近似时间倒序）。
func pandaFlipHistory(tiles []FlipTile, tiers []FlipTier) []GameRecord {
	tierName := func(id string) (string, float64) {
		for _, t := range tiers {
			if t.ID == id {
				return t.Name, t.Reward
			}
		}
		return "", 0
	}
	out := make([]GameRecord, 0, len(tiles))
	for _, tl := range tiles {
		if !tl.Flipped {
			continue
		}
		name, reward := tierName(tl.TierID)
		who := tl.ByUser
		label := "#" + numToStr(tl.Index)
		if who != "" {
			label = who + " 翻了 #" + numToStr(tl.Index)
		}
		if name != "" {
			label += " · " + name
		}
		out = append(out, GameRecord{PrizeName: label, PrizeType: tl.TierID, Amount: reward})
	}
	// 倒序：index 大的（通常后翻）在前
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// tierIDOf 兼容 id/tid/tierId 多键（layout 项与 tier 配置键名可能不同）。
func tierIDOf(m map[string]any) string {
	if v := firstNonEmpty(strField(m, "id"), strField(m, "tid"), strField(m, "tierId")); v != "" {
		return v
	}
	if v, ok := m["id"]; ok {
		return numToStr(v)
	}
	return ""
}

// pandaTierName getFlipTiers 项可能无 name，按 reward_type + reward 兜底描述。
func pandaTierName(m map[string]any) string {
	r := numField(m, "reward")
	if r <= 0 {
		return ""
	}
	return formatFloat(r) + "魔力"
}

func (PandaFlipGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	tile := strings.TrimSpace(req.Selection)
	if tile == "" || tile == "0" {
		tile = "1"
	}
	// round_number 优先用前端传入（来自 FlipState），缺则现取 gameState。
	round := strings.TrimSpace(req.Extra["round_number"])
	if round == "" {
		if gm, err := pandaFlipAjax(ctx, hc, baseURL, "gameState", cr.Cookie, nil); err == nil {
			if d := pandaFlipData(gm); d != nil {
				round = numToStr(d["roundNumber"])
			}
		}
	}
	if round == "" {
		round = "1"
	}
	m, err := pandaFlipAjax(ctx, hc, baseURL, "recordFlip", cr.Cookie, map[string]string{
		"round_number": round,
		"tile_index":   tile,
	})
	if err != nil {
		return nil, err
	}
	res := &PlayResult{Success: false, Raw: m}
	if d := pandaFlipData(m); d != nil {
		// 站点成功：ret==0 && data.status==1。status≠1 多为「该格已被别人翻开」。
		ok := int(longptNum(m["ret"])) == 0 && (int(longptNum(d["status"])) == 1 || d["status"] == nil)
		res.Success = ok
		res.Balance = numField(d, "balance")
		if fr, ok := d["flipResult"].(map[string]any); ok {
			// flipResult.tier 可能是对象 {name,reward_type,reward} 也可能是字符串
			tierObj, _ := fr["tier"].(map[string]any)
			name := strField(fr, "tier")
			if name == "" && tierObj != nil {
				name = strField(tierObj, "name")
			}
			if name == "" {
				name = strField(fr, "tierId")
			}
			reward := numField(fr, "reward")
			if reward == 0 && tierObj != nil {
				reward = numField(tierObj, "reward")
			}
			rtype := strField(fr, "reward_type")
			if rtype == "" && tierObj != nil {
				rtype = strField(tierObj, "reward_type")
			}
			res.Prize = pandaFlipPrizeText(name, reward, rtype)
			if rtype == "none" || (name == "" && reward == 0) {
				res.Message = "未中奖，继续加油"
			}
		}
	}
	if res.Success && res.Prize == "" {
		res.Prize = "中奖"
	}
	if !res.Success && res.Message == "" {
		// 该格已被翻开等：提示后前端会 load() 刷新出最新格子状态
		res.Message = firstNonEmpty(strField(m, "msg"), "该格可能已被翻开，请刷新后再试")
	}
	return res, nil
}

// pandaFlipPrizeText 按奖励类型组装奖项描述（对齐站点 JS：coins +魔力 / medal 勋章 / vip 限时VIP / none 未中奖）。
func pandaFlipPrizeText(name string, reward float64, rtype string) string {
	switch rtype {
	case "none":
		if name != "" {
			return name
		}
		return ""
	case "medal":
		return firstNonEmpty(name, "中奖") + " 获得勋章"
	case "vip":
		return firstNonEmpty(name, "中奖") + " 获得限时VIP"
	default:
		if name == "" && reward <= 0 {
			return ""
		}
		if name == "" {
			return "+" + formatFloat(reward) + "魔力"
		}
		if reward > 0 {
			return name + " +" + formatFloat(reward) + "魔力"
		}
		return name
	}
}

func (PandaFlipGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return PandaFlipGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{Selection: "1"})
}

// Stats 余额兜底（hero 用；FlipState 提供更全数据）。
func (PandaFlipGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	gm, err := pandaFlipAjax(ctx, hc, baseURL, "gameState", cr.Cookie, nil)
	if err != nil {
		return &StatsResult{}, nil
	}
	if d := pandaFlipData(gm); d != nil {
		return &StatsResult{Balance: numField(d, "balance"), CostPerPlay: numField(d, "costPerFlip")}, nil
	}
	return &StatsResult{}, nil
}

// ── OurBits 赛事押注（ourbits.club，NexusPHP「博彩频道」菠菜插件） ────────
// bet.php 当前竞猜列表（主题/结束时间/剩余/庄家/下注总数/投注总额/我的下注/状态）
//   → bet_odds.php?id=N 盘口（<h1>主题名 + 选项表：每行 队名/赔率/押注表单/已押）
//   → POST bet_odds2.php  bonus=<魔力>&id=<选项id>  （无 csrf，纯 cookie；注意 id 是选项 id 非主题 id）
// bet_coupon.php 我的押注（押注选项/倍率/押注金额/可获金额/状态）。
type OurbitsBetGame struct{}

func (OurbitsBetGame) ID() string   { return "ourbits_bet" }
func (OurbitsBetGame) Name() string { return "赛事押注" }

// ourbitsBetCellRe 一个 <td>/<th> 单元（按列抽取用）。
var ourbitsBetCellRe = regexp.MustCompile(`(?s)<t[dh][^>]*>(.*?)</t[dh]>`)

// ourbitsBetCells 抽一行内所有单元格的纯文本。
func ourbitsBetCells(row string) []string {
	cells := make([]string, 0, 8)
	for _, c := range ourbitsBetCellRe.FindAllStringSubmatch(row, -1) {
		cells = append(cells, strings.TrimSpace(stripTags(c[1])))
	}
	return cells
}

func ourbitsAtoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(strings.ReplaceAll(s, ",", "")))
	return n
}
func ourbitsAtof(s string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(s, ",", "")), 64)
	return n
}

// ourbitsBetBalance 从页面顶部「魔力值 [ 使用 ]: 372,140.3」抠余额（去千分位逗号）。
func ourbitsBetBalance(body string) float64 {
	return ourbitsAtof(firstGroup([]byte(body), `魔力值[^:]*:\s*([\d,]+\.?\d*)`))
}

// ourbitsBetTRRe 切单个 <tr>...</tr> 块（.*? 非贪婪到最近 </tr>，不跨行）。
// bet.php/bet_odds.php 表头与数据行各自独立闭合，先切块再按内容过滤，
// 避免一个正则跨多 tr 把表头并入数据行、致 cells 翻倍字段错位。
var ourbitsBetTRRe = regexp.MustCompile(`(?s)<tr[^>]*>.*?</tr>`)
var ourbitsBetH1Re = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)

// ourbitsBetEventIDRe 定位每场赛事的「点击押注」链接（bet_odds.php?id=N）。
var ourbitsBetEventIDRe = regexp.MustCompile(`bet_odds\.php\?id=(\d+)`)

// ourbitsBetParseEvents 解析 bet.php 当前竞猜列表。
// bet.php 表头缺 </tr> 闭合（「状态</td>」直接接「<tr><tr> 数据行」），用 <tr>...</tr> 切块会把
// 表头并入首个数据行、致 cells 翻倍字段错位。改锚点法：每个 bet_odds.php?id=N 往前找最近 <tr
// （数据行起点）、往后找最近 </tr>（终点），只抠该数据行单元格，绕开缺闭合的表头。
func ourbitsBetParseEvents(body string) []BetEvent {
	out := make([]BetEvent, 0)
	for _, m := range ourbitsBetEventIDRe.FindAllStringSubmatchIndex(body, -1) {
		idStr := body[m[2]:m[3]]
		oddsIdx := m[0]
		trStart := strings.LastIndex(body[:oddsIdx], "<tr")
		if trStart < 0 {
			trStart = oddsIdx
		}
		relEnd := strings.Index(body[oddsIdx:], "</tr>")
		if relEnd < 0 {
			continue
		}
		trEnd := oddsIdx + relEnd + len("</tr>")
		cells := ourbitsBetCells(body[trStart:trEnd])
		ev := BetEvent{ID: ourbitsAtoi(idStr), Status: "open"}
		if len(cells) > 0 {
			ev.Title = cells[0]
		}
		// cells[1] = "2026-07-12 00:00:00. 剩余: 10天07:10:37"
		if len(cells) > 1 {
			if parts := strings.SplitN(cells[1], "剩余:", 2); len(parts) == 2 {
				ev.EndTime = strings.TrimSuffix(strings.TrimSpace(parts[0]), ".")
				ev.Remain = strings.TrimSpace(parts[1])
			} else {
				ev.EndTime = cells[1]
			}
		}
		if len(cells) > 2 {
			ev.Bookmaker = cells[2]
		}
		if len(cells) > 3 {
			ev.BetCount = ourbitsAtoi(cells[3])
		}
		if len(cells) > 4 {
			ev.TotalStake = ourbitsAtof(cells[4])
		}
		if len(cells) > 5 {
			ev.MyStake = cells[5]
		}
		// cells[6] 状态列：含「点击押注」= open，否则用其文本（已结束等）。
		if len(cells) > 6 && !strings.Contains(cells[6], "点击押注") {
			ev.Status = cells[6]
		}
		if ev.Title != "" {
			out = append(out, ev)
		}
	}
	return out
}

// ourbitsBetParseDetail 解析 bet_odds.php?id=N 盘口（主题名 + 选项赔率列表）。
// 每个含 name='id' value=N 的 <tr> = 一个选项（id 是选项 id，下单 POST 用，非主题 id）。
func ourbitsBetParseDetail(body string, eventID int) *BetEventDetail {
	d := &BetEventDetail{Event: BetEvent{ID: eventID, Status: "open"}}
	// 页面首个 <h1> 是「博彩频道」导航，主题名取最后一个 h1。
	if h1s := ourbitsBetH1Re.FindAllStringSubmatch(body, -1); len(h1s) > 0 {
		d.Event.Title = strings.TrimSpace(stripTags(h1s[len(h1s)-1][1]))
	}
	for _, tr := range ourbitsBetTRRe.FindAllString(body, -1) {
		optID := firstGroup([]byte(tr), `name=['"]id['"]\s+value=['"](\d+)['"]`)
		if optID == "" {
			continue
		}
		cells := ourbitsBetCells(tr)
		opt := BetOption{ID: optID}
		if len(cells) > 0 {
			opt.Name = cells[0]
		}
		if len(cells) > 1 {
			opt.Odds = ourbitsAtof(cells[1])
		}
		// cells[3] 已押注列：「----」=未押 / 金额文本=已押（标记 MyOption）。
		if len(cells) > 3 && cells[3] != "" && cells[3] != "----" {
			d.MyOption = opt.Name
		}
		d.Options = append(d.Options, opt)
	}
	return d
}

// ourbitsBetCouponRe bet_coupon.php 我的押注每行。
var ourbitsBetCouponRe = regexp.MustCompile(`(?s)<tr[^>]*>\s*<td[^>]*>.*?</tr>`)

// ourbitsBetParseHistory 解析 bet_coupon.php（我的押注券）。
// 表头：押注选项 / 倍率 / 押注金额 / 可获金额 / 状态。无押注时页面显示「你还没有参加」。
// 结构待真实「有押注」数据校准（当前样例为空）。
func ourbitsBetParseHistory(body string) []GameRecord {
	if strings.Contains(body, "你还没有参加") {
		return []GameRecord{}
	}
	out := make([]GameRecord, 0)
	for _, m := range ourbitsBetCouponRe.FindAllStringSubmatch(body, -1) {
		cells := ourbitsBetCells(m[0])
		if len(cells) < 4 || strings.Contains(cells[0], "押注选项") {
			continue
		}
		rec := GameRecord{PrizeName: cells[0]}
		if len(cells) > 1 {
			rec.PrizeType = cells[1] // 倍率
		}
		if len(cells) > 2 {
			rec.Cost = ourbitsAtof(cells[2]) // 押注金额
		}
		if len(cells) > 3 {
			rec.Amount = ourbitsAtof(cells[3]) // 可获金额
		}
		if len(cells) > 4 {
			rec.CreatedAt = cells[4] // 状态
		}
		out = append(out, rec)
	}
	return out
}

// ourbitsBetFailRe 押注失败关键词（魔力不足/已结束/未登录/权限/重复）。
var ourbitsBetFailRe = regexp.MustCompile(`失败|错误|不足|未登录|权限|已经|尚未|关闭|结束`)

// ourbitsBetMsg 抠页面里的押注结果提示（NexusPHP 通常以 <font class=em> 或 div 回显）。
func ourbitsBetMsg(body string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?s)<font[^>]*class=["']em["'][^>]*>(.*?)</font>`),
		regexp.MustCompile(`(?s)<(?:div|span)[^>]*class=["'](?:success|error|msg|notice|em)["'][^>]*>(.*?)</(?:div|span)>`),
	} {
		if m := re.FindStringSubmatch(body); len(m) > 1 {
			if s := strings.TrimSpace(stripTags(m[len(m)-1])); s != "" {
				return s
			}
		}
	}
	if s := strings.TrimSpace(stripTags(firstGroup([]byte(body), `(?s)(押注[^<。！\n]{0,40})`))); s != "" {
		return s
	}
	return ""
}

// Events 列当前竞猜主题（GET bet.php）。
func (OurbitsBetGame) Events(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]BetEvent, error) {
	u := joinPath(baseURL, "bet.php")
	resp, err := hc.GetWithHeaders(ctx, u, cr.Cookie, map[string]string{"Referer": u}, true)
	if err != nil {
		return nil, err
	}
	logResp("ourbits_bet events", u, resp)
	return ourbitsBetParseEvents(string(resp.Body)), nil
}

// EventDetail 某主题盘口（GET bet_odds.php?id=N）。
// bet_odds 页只含主题名+选项赔率，不含截止时间/统计 → 合并 bet.php 列表里同 eventID 的元信息。
func (OurbitsBetGame) EventDetail(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, eventID int) (*BetEventDetail, error) {
	u := joinPath(baseURL, "bet_odds.php?id="+strconv.Itoa(eventID))
	resp, err := hc.GetWithHeaders(ctx, u, cr.Cookie, map[string]string{"Referer": joinPath(baseURL, "bet.php")}, true)
	if err != nil {
		return nil, err
	}
	logResp("ourbits_bet detail", u, resp)
	d := ourbitsBetParseDetail(string(resp.Body), eventID)
	// 合并 bet.php 列表元信息（截止时间/剩余/庄家/下注统计）；bet.php 抓取失败则仅留主题名+选项。
	lpURL := joinPath(baseURL, "bet.php")
	if lp, lerr := hc.GetWithHeaders(ctx, lpURL, cr.Cookie, map[string]string{"Referer": lpURL}, true); lerr == nil {
		logResp("ourbits_bet detail-list", lpURL, lp)
		for _, ev := range ourbitsBetParseEvents(string(lp.Body)) {
			if ev.ID == eventID {
				d.Event.EndTime = ev.EndTime
				d.Event.Remain = ev.Remain
				d.Event.Bookmaker = ev.Bookmaker
				d.Event.BetCount = ev.BetCount
				d.Event.TotalStake = ev.TotalStake
				d.Event.MyStake = ev.MyStake
				d.Event.Status = ev.Status
				break
			}
		}
	}
	return d, nil
}

// Bet 下注：POST bet_odds2.php  bonus=<魔力>&id=<选项id>（无 csrf，纯 cookie）。
// req.Selection=选项id（见 EventDetail.Options[].id），req.Amount=魔力金额。
// 站点返回整页 HTML，成功/失败靠页面文本启发式判定（未命中失败关键词即成功，待真实下单页校准）。
func (OurbitsBetGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	optID := strings.TrimSpace(req.Selection)
	if optID == "" {
		return &PlayResult{Success: false, Message: "缺少选项 id（selection=选项id，见 EventDetail.Options[].id）"}, nil
	}
	if req.Amount <= 0 {
		return &PlayResult{Success: false, Message: "押注金额须 > 0（魔力值）"}, nil
	}
	u := joinPath(baseURL, "bet_odds2.php")
	body := "bonus=" + strconv.FormatFloat(req.Amount, 'f', -1, 64) + "&id=" + optID
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(body), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Referer":      joinPath(baseURL, "bet_odds.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("ourbits_bet bet", u, resp)
	rbody := string(resp.Body)
	msg := ourbitsBetMsg(rbody)
	success := msg == "" || !ourbitsBetFailRe.MatchString(msg)
	return &PlayResult{
		Success: success,
		Balance: ourbitsBetBalance(rbody),
		Prize:   "押注选项 #" + optID + " " + strconv.FormatFloat(req.Amount, 'f', -1, 64) + " 魔力",
		Message: firstNonEmpty(msg, "押注已提交，等待开奖"),
	}, nil
}

// Play 默认兜底：提示三步下注（不自动下注，避免误扣魔力）。
func (OurbitsBetGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return &PlayResult{Success: false, Message: "请先 Events 查看竞猜 → EventDetail 查看赔率 → Bet(selection=选项id, amount=魔力) 下注"}, nil
}

// Stats 余额（GET bet.php 顶部魔力值）。
func (OurbitsBetGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	u := joinPath(baseURL, "bet.php")
	resp, err := hc.GetWithHeaders(ctx, u, cr.Cookie, map[string]string{"Referer": u}, true)
	if err != nil {
		return nil, err
	}
	logResp("ourbits_bet stats", u, resp)
	return &StatsResult{Balance: ourbitsBetBalance(string(resp.Body))}, nil
}

// History 我的押注（GET bet_coupon.php）。
func (OurbitsBetGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	u := joinPath(baseURL, "bet_coupon.php")
	resp, err := hc.GetWithHeaders(ctx, u, cr.Cookie, map[string]string{"Referer": u}, true)
	if err != nil {
		return nil, err
	}
	logResp("ourbits_bet history", u, resp)
	return ourbitsBetParseHistory(string(resp.Body)), nil
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
