package entertainment

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── HDVIDEO 签到宝箱（/games/chest/）───────────────────────────────
// 契约（2026-09 实测页面）：连续签到 7/15/30 天解锁宝箱；
// POST action=open&m=<天数> → {ok,label,balance}（失败 {ok:false,error}）。
// 页面按 streak 渲染 cb-chest（locked 还差 N 天 / open 可开 / done 已开）。
type HdvideoChestGame struct{ HdvideoBase }

func (g HdvideoChestGame) ID() string   { return "hdvideo_chest" }
func (g HdvideoChestGame) Name() string { return "签到宝箱" }

// hdvChestPage 取宝箱页。
func hdvChestPage(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (string, error) {
	return hdvPage(ctx, hc, baseURL, "chest", cookie)
}

// hdvChestParse 解析连续天数 + 里程碑状态（7/15/30 → locked/open/done）。
func hdvChestParse(body string) (streak int, milestones []string) {
	if m := compileRegex(`当前连续签到[：:]\s*(\d+)\s*天`).FindStringSubmatch(body); m != nil {
		streak, _ = strconv.Atoi(m[1])
	}
	// 每个「连续 N 天」里程碑向后截片段判状态（RE2 无 lookahead，用锚点切片代替块匹配）
	for _, loc := range compileRegex(`连续\s*(\d+)\s*天`).FindAllStringSubmatchIndex(body, -1) {
		days := body[loc[2]:loc[3]]
		tail := body[loc[1]:]
		// 片段截止到下一个里程碑（避免跨块把下一格的 disabled/✅ 串进来）
		if nxt := compileRegex(`连续\s*\d+\s*天`).FindStringIndex(tail[20:]); nxt != nil {
			tail = tail[:20+nxt[0]]
		} else if len(tail) > 320 {
			tail = tail[:320]
		}
		// 按钮文本优先（还差=locked、已领取=done、立即开启=open）；
		// 窗口尾部可能带出下一格的 disabled/✅，故只在按钮文本段（前 150 字符）判定
		btn := tail
		if len(btn) > 150 {
			btn = btn[:150]
		}
		state := "open"
		switch {
		case strings.Contains(btn, "还差") && !strings.Contains(btn, "立即开启"):
			state = "locked"
		case strings.Contains(btn, "已领取"):
			state = "done"
		case strings.Contains(btn, "cb-btn\" disabled"):
			state = "locked"
		}
		milestones = append(milestones, days+"天:"+state)
	}
	return streak, milestones
}

// Play 只读：连续天数 + 里程碑。
func (g HdvideoChestGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	body, err := hdvChestPage(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: err.Error()}, nil
	}
	streak, ms := hdvChestParse(body)
	msg := fmt.Sprintf("连续签到 %d 天", streak)
	if len(ms) > 0 {
		msg += "；" + strings.Join(ms, "、")
	}
	return &PlayResult{Success: true, Balance: hdvBalance(body), Message: msg}, nil
}

// Bet 开箱：Selection=7|15|30（里程碑天数；未解锁站点会拒）。
func (g HdvideoChestGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	m := strings.TrimSpace(req.Selection)
	switch m {
	case "7", "15", "30":
	default:
		return &PlayResult{Success: false, Message: "里程碑仅 7/15/30 天"}, nil
	}
	resp, err := hdvPostForm(ctx, hc, baseURL, "chest", cr.Cookie, "action=open&m="+m)
	if err != nil {
		return nil, err
	}
	ok := boolField(resp, "ok")
	msg := firstNonEmpty(strField(resp, "label"), strField(resp, "error"))
	if ok && msg == "" {
		msg = "开箱成功"
	}
	return &PlayResult{Success: ok, Balance: numField(resp, "balance"), Message: msg, Raw: resp}, nil
}

// Stats 余额。
func (g HdvideoChestGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := hdvChestPage(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: hdvBalance(body)}, nil
}
