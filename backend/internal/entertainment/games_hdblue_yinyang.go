// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

// 蓝影论坛（hdblue.cc）阴阳契适配：出题对赌游戏。
// POST /api/yinyang-contract {"action":"create","question","optionA","optionB","correctOption","stakePoints"}
// （2026-09-06 实测；规则：押注 10-500 影币，对方选答案，选对赢彩头，税率 10%，日发起 5 次）。
// Play 语义：自动发起一局"抛硬币"式题目（两个等价选项），等待其他玩家来接——
// 返回的不是即时开奖，而是"已挂单"状态与当前影币余额。

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

type HdblueYinyangGame struct{}

func (HdblueYinyangGame) ID() string   { return "hdblue_yinyang" }
func (HdblueYinyangGame) Name() string { return "阴阳契" }

// 默认押注（影币）——保守取最低档 10，避免大额自动消耗。
const hdblueYinyangDefaultStake = 10

// hdblueYinyangEnvelope create 响应 data。
type hdblueYinyangEnvelope struct {
	Summary *hdblueYinyangSummary `json:"summary"`
}

type hdblueYinyangSummary struct {
	UserID        int     `json:"userId"`
	PointName     string  `json:"pointName"`
	Points        float64 `json:"points"`
	WinCount      int     `json:"winCount"`
	LoseCount     int     `json:"loseCount"`
	TodayProfit   float64 `json:"todayProfitPoints"`
	TodayLoss     float64 `json:"todayLossPoints"`
	TotalProfit   float64 `json:"totalProfitPoints"`
	TotalLoss     float64 `json:"totalLossPoints"`
	DailyCreate   int     `json:"dailyCreateLimit"`
	DailyAccept   int     `json:"dailyAcceptLimit"`
	CreatedToday  int     `json:"createdToday"`
	AcceptedToday int     `json:"acceptedToday"`
}

func (HdblueYinyangGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	// 优先应战：大厅有公开挑战则逐个尝试（选 A——多数出题人把正确答案放 A）；
	// 某个挑战失败（已被他人应战/竞态）换下一个，全部失败带出原因，不再盲目发起
	// （发起还要押 10 影币，避免余额被耗尽）。无可接挑战再自己发起抛硬币题。
	if out, _ := hdblueYinyangTryAccept(ctx, hc, baseURL, cr); out != nil {
		return out, nil
	}

	// 发起：抛硬币式题目（正/反二选一，正确侧随机）——纯娱乐对赌，不涉及知识。
	correct := "A"
	if rand.Intn(2) == 1 {
		correct = "B"
	}
	body := fmt.Sprintf(`{"action":"create","question":%q,"optionA":"正面","optionB":"反面","correctOption":%q,"stakePoints":%d}`,
		"抛硬币——正面还是反面？（PTPatronus 自动发起）", correct, hdblueYinyangDefaultStake)

	u := strings.TrimRight(baseURL, "/") + "/api/yinyang-contract"
	resp, err := hc.Do(ctx, "POST", u, []byte(body), map[string]string{
		"Content-Type": "application/json",
		"Referer":      strings.TrimRight(baseURL, "/") + "/funs/yinyang-contract",
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("hdblue_yinyang", u, resp)

	var out hdblueResp
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("响应非 JSON（可能未登录或 session 失效）: %w", err)
	}
	if out.Code != 0 {
		return &PlayResult{Success: false, Message: out.Message}, nil
	}
	var env hdblueYinyangEnvelope
	_ = json.Unmarshal(out.Data, &env)

	res := &PlayResult{
		Success: true,
		Message: out.Message,
		Prize:   fmt.Sprintf("已发起阴阳契（押 %d 影币·正/反抛硬币），等待其他玩家应战", hdblueYinyangDefaultStake),
	}
	if env.Summary != nil {
		res.Balance = env.Summary.Points
		res.Message = fmt.Sprintf("%s；今日已发起 %d/%d", out.Message, env.Summary.CreatedToday, env.Summary.DailyCreate)
	}
	return res, nil
}

// hdblueYinyangChallenge 大厅公开挑战（应战用字段子集）。
type hdblueYinyangChallenge struct {
	ID        string `json:"id"`
	Status    string `json:"status"` // OPEN
	Question  string `json:"question"`
	OptionA   string `json:"optionA"`
	OptionB   string `json:"optionB"`
	CreatorID int    `json:"creatorId"`
	StakePts  int    `json:"stakePoints"`
}

// hdblueYinyangRSC 拉一次游戏页 RSC 并抽 initialData JSON（含 summary+openChallenges）。
func hdblueYinyangRSC(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (json.RawMessage, error) {
	u := strings.TrimRight(baseURL, "/") + "/funs/yinyang-contract"
	resp, err := hc.GetWithHeaders(ctx, u, cr.Cookie, map[string]string{"RSC": "1"}, true)
	if err != nil {
		return nil, err
	}
	body := string(resp.Body)
	seg := hdblueExtractJSON(body, `"initialData":{`)
	if seg == nil {
		seg = hdblueExtractJSON(body, `\"initialData\":{`)
	}
	if seg == nil {
		return nil, fmt.Errorf("未取到阴阳契数据（可能未登录）")
	}
	return unescapeRSC(seg), nil
}

// hdblueYinyangTryAccept 尝试应战大厅公开挑战（跳过自己发起的）。可接时返回非 nil 结果；
// 无可接挑战返回 nil,nil（调用方降级发起）。
func hdblueYinyangTryAccept(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	raw, err := hdblueYinyangRSC(ctx, hc, baseURL, cr)
	if err != nil {
		return nil, nil // 拉不到 RSC：调用方降级发起
	}
	var data struct {
		Summary        *hdblueYinyangSummary    `json:"summary"`
		OpenChallenges []hdblueYinyangChallenge `json:"openChallenges"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.Summary == nil {
		return nil, nil
	}
	me := data.Summary.UserID
	var lastFail *PlayResult
	for _, ch := range data.OpenChallenges {
		if ch.Status != "OPEN" || (me != 0 && ch.CreatorID == me) {
			continue
		}
		// 押注超过当前余额的挑战直接跳过（站点必 409），不浪费请求。
		if data.Summary != nil && float64(ch.StakePts) > data.Summary.Points {
			continue
		}
		// 选 A（无知识判断依据；纯娱乐）。
		body := fmt.Sprintf(`{"action":"accept","challengeId":%q,"selectedOption":"A"}`, ch.ID)
		u := strings.TrimRight(baseURL, "/") + "/api/yinyang-contract"
		resp, err := hc.Do(ctx, "POST", u, []byte(body), map[string]string{
			"Content-Type": "application/json",
			"Referer":      strings.TrimRight(baseURL, "/") + "/funs/yinyang-contract",
		}, cr.Cookie, true)
		if err != nil {
			continue // 该挑战网络失败：尝试下一个
		}
		logResp("hdblue_yinyang accept", u, resp)
		var out hdblueResp
		if err := json.Unmarshal(resp.Body, &out); err != nil {
			continue
		}
		if out.Code != 0 {
			// 该挑战业务失败（已被他人应战/竞态等）：换下一个；最后一次原因留作兜底提示。
			lastFail = &PlayResult{Success: false, Message: out.Message}
			continue
		}
		res := &PlayResult{Success: true, Message: out.Message}
		var env hdblueYinyangEnvelope
		_ = json.Unmarshal(out.Data, &env)
		if env.Summary != nil {
			res.Balance = env.Summary.Points
		}
		res.Prize = fmt.Sprintf("已应战「%s」（彩头 %d 影币，选 A）——立即结算", truncateRunes(ch.Question, 24), ch.StakePts)
		return res, nil
	}
	if lastFail != nil {
		return lastFail, nil
	}
	return nil, nil
}

// truncateRunes 按 rune 截断（对齐 truncTitle 语义）。
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// Stats 阴阳契状态（RSC initialData.summary）。
func (HdblueYinyangGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	u := strings.TrimRight(baseURL, "/") + "/funs/yinyang-contract"
	resp, err := hc.GetWithHeaders(ctx, u, cr.Cookie, map[string]string{"RSC": "1"}, true)
	if err != nil {
		return nil, err
	}
	body := string(resp.Body)
	// RSC 转义形态与裸键两种形态（同 gobang）
	seg := hdblueExtractJSON(body, `"summary":{"userId"`)
	if seg == nil {
		seg = hdblueExtractJSON(body, `\"summary\":{\"userId\"`)
	}
	if seg == nil {
		return nil, fmt.Errorf("未取到阴阳契状态（可能未登录）")
	}
	var s hdblueYinyangSummary
	if err := json.Unmarshal(unescapeRSC(seg), &s); err != nil {
		return nil, err
	}
	return &StatsResult{
		Balance:     s.Points,
		TodaySpins:  s.CreatedToday,
		MaxDaily:    s.DailyCreate,
		CostPerPlay: hdblueYinyangDefaultStake,
	}, nil
}
