package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 不可躺 猜大小（www.tangpt.top，plugins/big-small）────────────────
// 契约（2026-09 实测 bigsmall.js v4.6.2 + snapshot 真实响应）：
//
//	GET  /api/big-small/snapshot?period=<周期> → {success,data:{state,issues,my_bets,leaderboards}}
//	      state: balance/current_issue{issue_no,betting_open,draw_at,min_bet,odds,...}/recent_issues
//	POST /api/big-small/bets json {issue_no, side:"big"|"small", amount:"整数", idempotency_key}
//	      429 = 操作频繁； envelope {success,message,data}
//
// 每期 60 秒（betting_seconds），开奖号 0-9，0-4 小 / 5-9 大。下注扣魔力——只走 Bet 显式。
type TangptBigSmallGame struct{}

func (TangptBigSmallGame) ID() string   { return "tangpt_bigsmall" }
func (TangptBigSmallGame) Name() string { return "猜大小" }

// tangptBsGet GET JSON 端点。
func tangptBsGet(ctx context.Context, hc *httpclient.Client, baseURL, path, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, path)
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{
		"Accept":           "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "plugins/big-small/index.php"),
	}, true)
	if err != nil {
		return nil, err
	}
	logResp("tangpt_bigsmall GET "+path, u, resp)
	return parseRaw(resp.Body)
}

// tangptBsSnapshot 拉全景快照（data 层）。
func tangptBsSnapshot(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (map[string]any, error) {
	m, err := tangptBsGet(ctx, hc, baseURL, "api/big-small/snapshot", cookie)
	if err != nil {
		return nil, err
	}
	if d, ok := m["data"].(map[string]any); ok {
		return d, nil
	}
	return nil, fmt.Errorf("快照获取失败：%s", firstNonEmpty(strField(m, "message"), strField(m, "msg"), "可能未登录"))
}

// BigSmallState 全景快照（复用通用猜大小富 UI 结构）。
func (g TangptBigSmallGame) BigSmallState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*BigSmallState, error) {
	d, err := tangptBsSnapshot(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &BigSmallState{HasHouse: false, Currency: "魔力"}
	if s, ok := d["state"].(map[string]any); ok {
		st.Balance = longptNum(s["balance"])
		// 快捷金额（页面快捷按钮）
		if qa, ok := s["quick_amounts"].([]any); ok {
			for _, v := range qa {
				st.QuickAmounts = append(st.QuickAmounts, longptNum(v))
			}
		}
		if ci, ok := s["current_issue"].(map[string]any); ok {
			round := BigSmallRound{
				GameNo:    strField(ci, "issue_no"),
				Status:    strField(ci, "status"),
				TotalBet:  longptNum(ci["total_stake"]),
				Countdown: int(longptNum(ci["betting_seconds"])),
			}
			if anyBool(ci["betting_open"]) {
				round.Status = "betting"
			} else {
				round.Status = "sealed"
			}
			st.Round = round
			st.MinBet = longptNum(ci["min_bet"])
			if v := longptNum(ci["max_issue_bet"]); v > 0 {
				st.MaxBet = v
			}
			st.Payout = fmt.Sprintf("%v", ci["odds"])
			// 双方总注（押大/押小池）
			st.BigBets = []BigSmallBetSpot{{Username: "押大池", Amount: longptNum(ci["total_big"])}}
			st.SmallBets = []BigSmallBetSpot{{Username: "押小池", Amount: longptNum(ci["total_small"])}}
		}
		// 最近开奖走势（recent_issues：result_number/result_side）
		if ri, ok := s["recent_issues"].([]any); ok {
			for _, r := range ri {
				rm, ok := r.(map[string]any)
				if !ok {
					continue
				}
				switch strings.ToLower(strField(rm, "result_side")) {
				case "big":
					st.RecentResults = append(st.RecentResults, "big")
					st.Trend10.BigCount++
				case "small":
					st.RecentResults = append(st.RecentResults, "small")
					st.Trend10.SmallCount++
				default:
					st.RecentResults = append(st.RecentResults, "house")
				}
				if len(st.RecentResults) >= 20 {
					break
				}
			}
		}
	}
	// 我的当期下注
	if mb, ok := d["my_bets"].(map[string]any); ok {
		if items, ok := mb["items"].([]any); ok {
			for _, it := range items {
				im, ok := it.(map[string]any)
				if !ok {
					continue
				}
				st.MyBet = &BigSmallMyBet{
					GameNo:  strField(im, "issue_no"),
					BetType: strings.ToLower(strField(im, "side")),
					Amount:  longptNum(im["amount"]),
				}
				break
			}
		}
	}
	// 排行榜（leaderboards.profit/…，尽力取第一个）
	if lbs, ok := d["leaderboards"].(map[string]any); ok {
		for key, v := range lbs {
			lst, ok := v.([]any)
			if !ok {
				continue
			}
			rank := BigSmallRank{Key: key, Title: key}
			for _, e := range lst {
				em, ok := e.(map[string]any)
				if !ok {
					continue
				}
				rank.Entries = append(rank.Entries, BigSmallRankEntry{
					Username: strField(em, "username"),
					Amount:   longptNum(em["net_profit"]),
					Count:    int(longptNum(em["bet_count"])),
				})
			}
			st.Rankings = append(st.Rankings, rank)
			if len(st.Rankings) >= 2 {
				break
			}
		}
	}
	return st, nil
}

// Bet 下注：Selection=big|small，Amount≥min_bet。
func (g TangptBigSmallGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	side := strings.ToLower(strings.TrimSpace(req.Selection))
	if side != "big" && side != "small" {
		return &PlayResult{Success: false, Message: "请选 big 或 small"}, nil
	}
	// 取当期期号与最低注额
	d, err := tangptBsSnapshot(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	s, _ := d["state"].(map[string]any)
	ci, _ := s["current_issue"].(map[string]any)
	if ci == nil || !anyBool(ci["betting_open"]) {
		return &PlayResult{Success: false, Message: "当前封盘或无进行中期次，稍后再试"}, nil
	}
	issueNo := strField(ci, "issue_no")
	minBet := longptNum(ci["min_bet"])
	amount := req.Amount
	if amount <= 0 {
		amount = minBet
	}
	if minBet > 0 && amount < minBet {
		return &PlayResult{Success: false, Message: fmt.Sprintf("最低注额 %v 魔力", minBet)}, nil
	}
	payload, _ := json.Marshal(map[string]any{
		"issue_no":        issueNo,
		"side":            side,
		"amount":          fmt.Sprintf("%d", int64(amount)),
		"idempotency_key": fmt.Sprintf("ptpatronus-%d-%s", int64(amount), issueNo),
	})
	u := joinPath(baseURL, "api/big-small/bets")
	resp, err := hc.Do(ctx, http.MethodPost, u, payload, map[string]string{
		"Content-Type":     "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "plugins/big-small/index.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("tangpt_bigsmall bet", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "响应解析失败"}, nil
	}
	ok := boolField(m, "success")
	msg := firstNonEmpty(strField(m, "message"), strField(m, "msg"))
	if !ok && msg == "" {
		msg = "下注失败"
	}
	if ok && msg == "" {
		msg = fmt.Sprintf("下注成功：押%s %v 魔力", map[string]string{"big": "大", "small": "小"}[side], amount)
	}
	return &PlayResult{Success: ok, Message: msg, Prize: side, Raw: m}, nil
}

// 其余 BigSmallable 能力（聊天/任务/道具/红包）躺平未提供——走只读降级。

// BigSmallChat 躺平无聊天，返回空。
func (g TangptBigSmallGame) BigSmallChat(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, since int) ([]BigSmallChat, error) {
	return nil, nil
}

// BigSmallSendChat 躺平无聊天。
func (g TangptBigSmallGame) BigSmallSendChat(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, message string) (*PlayResult, error) {
	return &PlayResult{Success: false, Message: "躺平猜大小不支持聊天"}, nil
}

// BigSmallTaskAction 躺平无任务系统。
func (g TangptBigSmallGame) BigSmallTaskAction(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string, taskID int) (*PlayResult, error) {
	return &PlayResult{Success: false, Message: "躺平猜大小没有任务"}, nil
}

// BigSmallUseItem 躺平无道具。
func (g TangptBigSmallGame) BigSmallUseItem(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, tag string) (*PlayResult, error) {
	return &PlayResult{Success: false, Message: "躺平猜大小没有道具"}, nil
}

// BigSmallRedpacket 躺平红包是站内另一套 /api/redpacket/*（挂聊天室），暂不接入该面板。
func (g TangptBigSmallGame) BigSmallRedpacket(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string, extra map[string]string) (*PlayResult, error) {
	return &PlayResult{Success: false, Message: "躺平红包请在网页聊天室使用"}, nil
}

// Play 只读快照（LotteryGame 兜底）。
func (g TangptBigSmallGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	st, err := g.BigSmallState(ctx, hc, baseURL, cr)
	if err != nil {
		return nil, err
	}
	res := &PlayResult{Success: true, Balance: st.Balance}
	switch st.Round.Status {
	case "betting":
		res.Message = fmt.Sprintf("第 %s 期正在接受下注（%s 后开奖），当前池 押大 %v / 押小 %v",
			st.Round.GameNo, formatDuration(st.Round.Countdown), poolSum(st.BigBets), poolSum(st.SmallBets))
	case "sealed":
		res.Message = fmt.Sprintf("第 %s 期已封盘，等待开奖", st.Round.GameNo)
	default:
		res.Message = "等待新一期开局"
	}
	return res, nil
}

// Stats 余额。
func (g TangptBigSmallGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	d, err := tangptBsSnapshot(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{}
	if s, ok := d["state"].(map[string]any); ok {
		st.Balance = longptNum(s["balance"])
	}
	return st, nil
}

// poolSum 池合计。
func poolSum(spots []BigSmallBetSpot) float64 {
	var sum float64
	for _, s := range spots {
		sum += s.Amount
	}
	return sum
}

// formatDuration 秒数转 1m05s 形态。
func formatDuration(sec int) string {
	if sec <= 0 {
		return "0s"
	}
	if sec >= 60 {
		return fmt.Sprintf("%dm%02ds", sec/60, sec%60)
	}
	return fmt.Sprintf("%ds", sec)
}

// anyBool any 布尔取值（boolField 的单参版）。
func anyBool(v any) bool {
	b, _ := v.(bool)
	return b
}
