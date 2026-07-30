package entertainment

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

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
	if amount > maxBetCeiling {
		amount = maxBetCeiling
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
		"userId":    "0",
		"username":  cr.Username,
		"content":   message,
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
