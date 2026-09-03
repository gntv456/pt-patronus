package entertainment

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── HDVIDEO HiLo（/games/hilo/，状态机：start→guess(hi/lo)→cashout） ──
type HdvideoHiloGame struct{ HdvideoBase }

func (g HdvideoHiloGame) ID() string   { return "hdvideo_hilo" }
func (g HdvideoHiloGame) Name() string { return "HiLo" }

// hiloAction 提交动作并返回响应 map。
func (g HdvideoHiloGame) hiloAction(ctx context.Context, hc *httpclient.Client, baseURL, cookie, action, extra string) (map[string]any, error) {
	return hdvPostForm(ctx, hc, baseURL, "hilo", cookie, "action="+action+extra)
}

// Play 只读：页面 initState 内嵌（当前局状态）。简化为同步余额 + 提示。
func (g HdvideoHiloGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	body, err := hdvPage(ctx, hc, baseURL, "hilo", cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	res := &PlayResult{Success: true, Balance: hdvBalance(body)}
	hs := parseHiloState(body)
	if hs.Playing {
		res.Raw = map[string]any{"hilo_state": hs}
		res.Message = fmt.Sprintf("进行中：%s，彩池 %v 票（×%.2f，连胜 %d），可继续猜大小", hs.Card, hs.Pot, hs.Mult, hs.Streak)
	} else {
		res.Message = "HiLo 已同步，无进行中对局；开新局走下注面板"
	}
	return res, nil
}

// Bet：Amount>0 = 开新局（action=start&bet=N）；Selection=cashout = 兑现。
// guess（hi/lo）：Play 已带出当前牌面（hilo_state），前端展示后可决策（批17）。
func (g HdvideoHiloGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	sel := strings.ToLower(strings.TrimSpace(req.Selection))
	if sel == "cashout" {
		m, err := g.hiloAction(ctx, hc, baseURL, cr.Cookie, "cashout", "")
		if err != nil {
			return nil, err
		}
		ok := boolField(m, "ok")
		return &PlayResult{Success: ok, Balance: numField(m, "balance"),
			Message: firstNonEmpty(strField(m, "message"), firstNonEmpty(strField(m, "error"), ""))}, nil
	}
	if sel == "hi" || sel == "lo" {
		m, err := g.hiloAction(ctx, hc, baseURL, cr.Cookie, "guess", "&dir="+sel)
		if err != nil {
			return nil, err
		}
		ok := boolField(m, "ok")
		msg := firstNonEmpty(strField(m, "message"), strField(m, "error"))
		if ok {
			if card := strField(m, "card"); card != "" {
				msg = fmt.Sprintf("翻出 %s｜%s", card, msg)
			}
		}
		return &PlayResult{Success: ok, Message: msg, Balance: numField(m, "balance"), Raw: m}, nil
	}
	if req.Amount > 0 {
		m, err := g.hiloAction(ctx, hc, baseURL, cr.Cookie, "start", "&bet="+strconv.Itoa(int(req.Amount)))
		if err != nil {
			return nil, err
		}
		ok := boolField(m, "ok")
		msg := firstNonEmpty(strField(m, "message"), strField(m, "error"))
		if ok && msg == "" {
			msg = fmt.Sprintf("已开局，投入 %v 票；后续猜大小请在网页进行（需要看牌面）", req.Amount)
		}
		return &PlayResult{Success: ok, Message: msg, Balance: numField(m, "balance"), Raw: m}, nil
	}
	return &PlayResult{Success: false, Message: "Amount=票数开新局；Selection=cashout 兑现 / hi / lo 猜大小"}, nil
}

// Stats 余额。
func (g HdvideoHiloGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := hdvPage(ctx, hc, baseURL, "hilo", cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: hdvBalance(body)}, nil
}

// ── HDVIDEO 21点（/games/blackjack/，deal&bet / hit / stand / double） ──
type HdvideoBlackjackGame struct{ HdvideoBase }

func (g HdvideoBlackjackGame) ID() string   { return "hdvideo_blackjack" }
func (g HdvideoBlackjackGame) Name() string { return "二十一点" }

// hdvBjAction 提交动作返回响应 map（{ok, player:[], dealer:[], score, outcome,...}）。
func hdvBjAction(ctx context.Context, hc *httpclient.Client, baseURL, cookie, action, extra string) (map[string]any, error) {
	return hdvPostForm(ctx, hc, baseURL, "blackjack", cookie, "action="+action+extra)
}

// Blackjack 实现 Blackjackable。new=重读页面同步（不下注发牌）；deal 需赌注——
// 页面语义 deal&bet=N，此处 action=deal 用 Extra/amount 前端传 BetRequest 不达，
// 约定：new/hit/stand/double 直通；发牌走 Bet(amount=N)。
func (g HdvideoBlackjackGame) Blackjack(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string) (*BlackjackResult, error) {
	if action == "" {
		action = "new"
	}
	switch action {
	case "new":
		body, err := hdvPage(ctx, hc, baseURL, "blackjack", cr.Cookie)
		if err != nil {
			return nil, err
		}
		res := &BlackjackResult{Balance: hdvBalance(body)}
		if strings.Contains(body, "继续上一局") {
			res.Stage, res.Result, res.Message = "playing", "playing", "检测到进行中牌局，可继续要牌/停牌"
		} else {
			res.Stage, res.Result, res.Message = "idle", "", "无进行中对局；发牌（下注）走下注面板"
		}
		return res, nil
	case "hit", "stand", "double":
		m, err := hdvBjAction(ctx, hc, baseURL, cr.Cookie, action, "")
		if err != nil {
			return nil, err
		}
		return hdvBjParseResult(m), nil
	}
	return nil, fmt.Errorf("不支持的动作 %q（发牌走 Bet amount=赌注）", action)
}

// hdvBjParseResult 响应 map → BlackjackResult。
func hdvBjParseResult(m map[string]any) *BlackjackResult {
	res := &BlackjackResult{}
	if !boolField(m, "ok") {
		res.Message = firstNonEmpty(strField(m, "error"), "动作失败")
		res.Stage, res.Result = "idle", ""
		return res
	}
	res.Stage, res.Result = "playing", "playing"
	if cards, ok := m["player"].([]any); ok {
		for _, c := range cards {
			if len(res.PlayerCards) < 5 {
				res.PlayerCards = append(res.PlayerCards, fmt.Sprintf("%v", c))
			}
		}
	}
	if v := longptNum(m["score"]); v > 0 {
		res.PlayerScore = int(v)
	}
	if out := strField(m, "outcome"); out != "" && out != "playing" {
		res.Stage = "finished"
		switch out {
		case "win", "blackjack":
			res.Result = "win"
		case "push":
			res.Result = "push"
		default:
			res.Result = "lose"
		}
	}
	res.Message = firstNonEmpty(strField(m, "outcomeLabel"), "已同步牌局")
	return res
}

// Bet 发牌（deal&bet=N）。
func (g HdvideoBlackjackGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	if req.Amount <= 0 {
		return &PlayResult{Success: false, Message: "请填写发牌赌注（票）"}, nil
	}
	m, err := hdvBjAction(ctx, hc, baseURL, cr.Cookie, "deal", "&bet="+strconv.Itoa(int(req.Amount)))
	if err != nil {
		return nil, err
	}
	r := hdvBjParseResult(m)
	return &PlayResult{Success: r.Stage != "idle", Message: r.Message,
		Balance: r.Balance, Prize: r.Result, Raw: m}, nil
}

// Play 只读。
func (g HdvideoBlackjackGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	r, err := g.Blackjack(ctx, hc, baseURL, cr, "new")
	if err != nil {
		return nil, err
	}
	return &PlayResult{Success: true, Balance: r.Balance, Message: r.Message}, nil
}

// Stats 余额。
func (g HdvideoBlackjackGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := hdvPage(ctx, hc, baseURL, "blackjack", cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: hdvBalance(body)}, nil
}

// hdvHiloStateRe 页面 initState = {...};（进行中牌局：card/pot/mult/streak/options）。
var hdvHiloStateRe = regexp.MustCompile(`(?s)initState\s*=\s*(\{.*?\})\s*;`)

// HiloState HiLo 进行中牌局（批17：Play 只读带出，前端展示牌面后可 guess）。
type HiloState struct {
	Playing bool    `json:"playing"`
	Card    string  `json:"card,omitempty"`   // 当前牌面
	Pot     float64 `json:"pot,omitempty"`    // 累计彩池（票）
	Mult    float64 `json:"mult,omitempty"`   // 当前倍率
	Streak  int     `json:"streak,omitempty"` // 连胜次数
	HiAvail bool    `json:"hi_avail"`
	HiMult  float64 `json:"hi_mult,omitempty"`
	LoAvail bool    `json:"lo_avail"`
	LoMult  float64 `json:"lo_mult,omitempty"`
}

// parseHiloState 从 hilo 页面解析 initState（嵌套 options 用子串+正则抠）。
func parseHiloState(body string) *HiloState {
	m := hdvHiloStateRe.FindStringSubmatch(body)
	if m == nil {
		return &HiloState{}
	}
	raw := m[1]
	st := &HiloState{Playing: true}
	st.Card = firstGroup([]byte(raw), `"card"\s*:\s*"([^"]*)"`)
	if v, err := strconv.ParseFloat(jsonNumField(raw, "pot"), 64); err == nil {
		st.Pot = v
	}
	if v, err := strconv.ParseFloat(jsonNumField(raw, "mult"), 64); err == nil {
		st.Mult = v
	}
	if v, err := strconv.ParseFloat(jsonNumField(raw, "streak"), 64); err == nil {
		st.Streak = int(v)
	}
	st.HiAvail = regexp.MustCompile(`"hi"\s*:\s*\{[^}]*?"avail"\s*:\s*true`).MatchString(raw)
	st.LoAvail = regexp.MustCompile(`"lo"\s*:\s*\{[^}]*?"avail"\s*:\s*true`).MatchString(raw)
	if m2 := regexp.MustCompile(`"hi"\s*:\s*\{[^}]*?"mult"\s*:\s*([\d.]+)`).FindStringSubmatch(raw); m2 != nil {
		st.HiMult, _ = strconv.ParseFloat(m2[1], 64)
	}
	if m2 := regexp.MustCompile(`"lo"\s*:\s*\{[^}]*?"mult"\s*:\s*([\d.]+)`).FindStringSubmatch(raw); m2 != nil {
		st.LoMult, _ = strconv.ParseFloat(m2[1], 64)
	}
	return st
}

// jsonNumField 从 JSON 文本中抠顶层数值字段（无匹配返回空）。
func jsonNumField(raw, key string) string {
	m := regexp.MustCompile(`"` + key + `"` + `\s*:\s*(-?[\d.]+)`).FindStringSubmatch(raw)
	if m == nil {
		return ""
	}
	return m[1]
}
