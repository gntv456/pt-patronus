// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── OurBits 赛事押注（ourbits.club，NexusPHP「博彩频道」菠菜插件） ────────
// bet.php 当前竞猜列表（主题/结束时间/剩余/庄家/下注总数/投注总额/我的下注/状态）
//
//	→ bet_odds.php?id=N 盘口（<h1>主题名 + 选项表：每行 队名/赔率/押注表单/已押）
//	→ POST bet_odds2.php  bonus=<魔力>&id=<选项id>  （无 csrf，纯 cookie；注意 id 是选项 id 非主题 id）
//
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
