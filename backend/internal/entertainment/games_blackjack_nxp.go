package entertainment

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── NexusPHP 二十一点整页表单插件（2026-09 全站普查发现 10 站） ────────────
// 交互：GET blackjack.php（整页 HTML 含牌面/按钮）→ POST game=hit|stand|double&start=yes
// （52PT/HDArea/HDU/PTHome/CyanBug/TTG 同款；SSD 另带 join=1&amount=N 显式下注）。
// 响应为整页 HTML（非 Audiences 的 ajax JSON），本适配器按 HTML 解析。
//
// 风险控制：Play=开局仅当「页面无进行中对局」时提交 start=yes；amount 类站（SSD）
// 不自动下注——需要下注的局由用户在网页完成，本游戏只做已有局的 hit/stand/double。
type NxpBlackjackGame struct{}

func (NxpBlackjackGame) ID() string   { return "nxp_blackjack" }
func (NxpBlackjackGame) Name() string { return "二十一点" }

// nxpBjGet 取整页。
func nxpBjGet(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (string, error) {
	u := joinPath(baseURL, "blackjack.php")
	resp, err := hc.GetWithHeaders(ctx, u, cookie, map[string]string{"Referer": u}, true)
	if err != nil {
		return "", err
	}
	logResp("nxp_blackjack page", u, resp)
	return string(resp.Body), nil
}

// nxpBjPost 提交动作（hit/stand/double），start 参数仅开局语义需要。
func nxpBjPost(ctx context.Context, hc *httpclient.Client, baseURL, cookie, action string, extra url.Values) (string, error) {
	u := joinPath(baseURL, "blackjack.php")
	form := url.Values{}
	form.Set("game", action)
	for k, vs := range extra {
		for _, v := range vs {
			form.Set(k, v)
		}
	}
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte(form.Encode()), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          u,
	}, cookie, true)
	if err != nil {
		return "", err
	}
	logResp("nxp_blackjack "+action, u, resp)
	return string(resp.Body), nil
}

var (
	// 牌面两种形态：花色符号文本（♠10）或图片文件名（pic/cards/as.gif=黑桃A、jd.gif=方块J…）。
	nxpBjCardRe  = regexp.MustCompile(`[♠♥♦♣]\s*([0-9AJQK]{1,2})`)
	nxpBjCardImg = regexp.MustCompile(`[\w/.-]*cards/((?:10|[2-9]|[ajqk])[shdc])\.gif`)
	nxpBjScoreRe = regexp.MustCompile(`(?:点数|目前.{0,6}点|Score)\s*[:：]?\s*(\d{1,2})`)
	nxpBjBonusRe = regexp.MustCompile(`(?:魔力值|bonus|Bonus)[^0-9]{0,12}([\d,.]+)`)
	nxpBjFormRe  = regexp.MustCompile(`(?s)<form[^>]*>.*?</form>`)
	nxpBjInputRe = regexp.MustCompile(`name=['"]?game['"]?[^>]*value=['"]?(\w+)`)
	nxpBjStartRe = regexp.MustCompile(`name=['"]?start['"]?[^>]*value=['"]?yes`)
)

// nxpCardName 图片名（as/jd/10h…）转牌面文本：末位 s/h/d/c=花色，前缀=点数。
func nxpCardName(n string) string {
	if len(n) < 2 {
		return strings.ToUpper(n)
	}
	suit := map[string]string{"s": "♠", "h": "♥", "d": "♦", "c": "♣"}[n[len(n)-1:]]
	if suit == "" {
		return strings.ToUpper(n)
	}
	return suit + strings.ToUpper(n[:len(n)-1])
}

// nxpBjParse 从整页 HTML 解析牌局（玩家牌与点数、余额、是否进行中）。
// 状态判据（真实联调校准）：「无局」页面也带 game=hit&start=yes 表单（开局表单）——
// 表单含 start=yes 即开局入口（idle）；含 hit/stand/double 而无 start=yes 才是进行中动作表单。
func nxpBjParse(body string) *BlackjackResult {
	res := &BlackjackResult{}
	// 牌面：花色符号文本优先，无则解析图片名。
	cards := nxpBjCardRe.FindAllStringSubmatch(body, -1)
	for _, c := range cards {
		if len(res.PlayerCards) < 5 {
			res.PlayerCards = append(res.PlayerCards, strings.TrimSpace(c[0]))
		}
	}
	if len(res.PlayerCards) == 0 {
		for _, m := range nxpBjCardImg.FindAllStringSubmatch(body, -1) {
			if len(res.PlayerCards) < 5 {
				res.PlayerCards = append(res.PlayerCards, nxpCardName(m[1]))
			}
		}
	}
	if m := nxpBjScoreRe.FindStringSubmatch(body); m != nil {
		res.PlayerScore, _ = strconv.Atoi(m[1])
	}
	if m := nxpBjBonusRe.FindStringSubmatch(body); m != nil {
		res.Balance, _ = strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	}
	// 状态：逐个表单看 game 值与 start=yes。
	inGame := false
	for _, fm := range nxpBjFormRe.FindAllString(body, -1) {
		if !nxpBjStartRe.MatchString(fm) {
			if mm := nxpBjInputRe.FindStringSubmatch(fm); mm != nil {
				switch mm[1] {
				case "hit", "stand", "double":
					inGame = true
				}
			}
		}
	}
	hasCards := len(res.PlayerCards) > 0
	switch {
	case inGame:
		res.Stage = "playing"
		res.Result = "playing"
	case hasCards:
		res.Stage = "finished"
		low := strings.ToLower(body)
		switch {
		case strings.Contains(body, "你赢") || strings.Contains(body, "獲勝") || strings.Contains(low, "you win"):
			res.Result = "win"
		case strings.Contains(body, "平"):
			res.Result = "push"
		default:
			res.Result = "lose"
		}
	default:
		res.Stage = "idle"
	}
	if res.Message == "" {
		res.Message = "已同步牌局"
	}
	return res
}

// Blackjack 实现 Blackjackable：new=重读页面（不下注开局）；hit/stand/double=提交动作后解析新页面。
func (NxpBlackjackGame) Blackjack(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string) (*BlackjackResult, error) {
	if action == "" {
		action = "new"
	}
	if action == "new" {
		// 只读当前状态：进行中则继续（不重复开局），无局则提示到网页开局（避免误下注）。
		body, err := nxpBjGet(ctx, hc, baseURL, cr.Cookie)
		if err != nil {
			return nil, err
		}
		res := nxpBjParse(body)
		if res.Stage == "idle" {
			res.Message = "当前无进行中对局：开局可能涉及下注，请在网页完成开局后回来继续要牌/停牌"
		}
		return res, nil
	}
	body, err := nxpBjPost(ctx, hc, baseURL, cr.Cookie, action, url.Values{})
	if err != nil {
		return nil, err
	}
	return nxpBjParse(body), nil
}

// Play（LotteryGame 兜底）= 读当前牌局。
func (NxpBlackjackGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	r, err := NxpBlackjackGame{}.Blackjack(ctx, hc, baseURL, cr, "new")
	if err != nil {
		return nil, err
	}
	return &PlayResult{Success: true, Balance: r.Balance, Prize: r.Result, Message: r.Message}, nil
}

// Stats 余额（blackjack.php 页内 bonus）。
func (NxpBlackjackGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := nxpBjGet(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	st := &StatsResult{}
	if m := nxpBjBonusRe.FindStringSubmatch(body); m != nil {
		st.Balance, _ = strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	}
	return st, nil
}
