// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"log/slog"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 默认判定文案（中英文）。可在单条监控目标上用 ClosedText/OpenText 覆盖/补充。
var (
	defaultClosedText = []string{"关闭注册", "暂停注册", "停止注册", "invite only", "邀请注册", "尚未开放", "register closed"}
	defaultOpenText   = []string{"takesignup", "name=\"wantinvite\"", "name=\"email\""}
)

// RegistrationChain 编排注册开放检测：GET signup 页判定开放/关闭。
type RegistrationChain struct {
	Base
	hc    *httpclient.Client
	regOp *oper.RegistrationOper
}

// NewRegistrationChain 构造。
func NewRegistrationChain(b Base, hc *httpclient.Client, regOp *oper.RegistrationOper) *RegistrationChain {
	return &RegistrationChain{Base: b, hc: hc, regOp: regOp}
}

// RegCheckResult 是单个监控目标的检测结果。
type RegCheckResult struct {
	WatchID uint   `json:"watch_id"`
	Name    string `json:"name"`
	Open    bool   `json:"open"`
	Unknown bool   `json:"unknown"` // 无法判定（被拦截 / 页面认不出标记）
	Error   string `json:"error,omitempty"`
}

// CheckOne 检测单个目标的注册状态，返回 (open, decided, err)。
// decided=false 表示「无法判定」：被 WAF/CF 拦截，或页面既没命中关闭文案也没命中开放表单
// （站点改版、返回错误页）。这不能等同于「关闭」——写回关闭后下一轮读通就又是一次
// 关闭→开放转换，一个从未关过的站会反复弹「开放注册」提醒。
func (c *RegistrationChain) CheckOne(ctx context.Context, w *models.RegistrationWatch) (bool, bool, error) {
	path := w.SignupPath
	if path == "" {
		path = "signup.php"
	}
	u := joinURLSimple(w.URL, path)
	resp, err := c.hc.Get(ctx, u, "", true)
	if err != nil {
		return false, false, err
	}
	if resp.IsCloudflare() {
		return false, false, nil // 被 WAF 拦截：无法判定（不告警，也不改状态）
	}
	body := strings.ToLower(string(resp.Body))
	final := resp.FinalURL
	if l := resp.Header.Get("Location"); l != "" {
		final = l
	}
	finalLower := strings.ToLower(final)

	// 关闭：命中关闭文案 / 重定向到登录 / invite-only
	closedTexts := append([]string{}, defaultClosedText...)
	if t := strings.TrimSpace(w.ClosedText); t != "" {
		closedTexts = append(closedTexts, strings.ToLower(t))
	}
	for _, t := range closedTexts {
		if strings.Contains(body, t) {
			return false, true, nil
		}
	}
	if strings.Contains(finalLower, "login") || strings.Contains(finalLower, "takelogin") {
		return false, true, nil
	}

	// 开放：命中表单/开放文案
	openTexts := append([]string{}, defaultOpenText...)
	if t := strings.TrimSpace(w.OpenText); t != "" {
		openTexts = append(openTexts, strings.ToLower(t))
	}
	for _, t := range openTexts {
		if strings.Contains(body, t) {
			return true, true, nil
		}
	}
	// 两边都没命中：页面不是我们认识的注册页（改版/错误页/跳转别的站）
	return false, false, nil
}

// CheckAll 检测全部监控目标。错误隔离：单条失败不影响其它。
func (c *RegistrationChain) CheckAll(ctx context.Context) []RegCheckResult {
	watches, err := c.regOp.List()
	if err != nil {
		return nil
	}
	out := make([]RegCheckResult, 0, len(watches))
	for i := range watches {
		w := &watches[i]
		open, decided, e := c.CheckOne(ctx, w)
		r := RegCheckResult{WatchID: w.ID, Name: w.Name, Open: open, Unknown: !decided}
		if e != nil {
			r.Error = errtext.Text(e)
		}
		out = append(out, r)
	}
	return out
}

// CheckAllAndUpdate 检测全部启用目标、回写状态，返回「新开放」的站点名
// （上次为关闭/从未检测 → 本次开放）。仅在状态转换时返回，供上层去重通知。
// 判定不了的目标只刷新检测时间、保留原状态：否则一次拦截就会把已开放的站写成关闭，
// 下一轮读通再报一次「开放注册」。
// 监控目标清单读失败交回错误（上层发通知自曝），静默 = 开放提醒永远不再响。
func (c *RegistrationChain) CheckAllAndUpdate(ctx context.Context) ([]string, error) {
	watches, err := c.regOp.ListActive()
	if err != nil {
		slog.Warn("registration list watches failed", "err", err)
		return nil, err
	}
	var opened []string
	for i := range watches {
		w := &watches[i]
		open, decided, e := c.CheckOne(ctx, w)
		if e != nil || !decided {
			_ = c.regOp.TouchChecked(w.ID) // 记「查过了」，但不动 last_open、不提醒
			continue
		}
		prev, err := c.regOp.UpdateStatus(w.ID, open)
		if err != nil {
			// 状态没写进去＝这一站的「开/关」没落账，下一轮还会读旧值；
			// 而且这条 watch 不会进本轮结果清单，界面上像「没这个站」而不是「这次没更新」。
			slog.Warn("registration: update status failed", "watch", w.Name, "open", open, "err", err)
			continue
		}
		wasOpen := prev != nil && *prev
		if open && !wasOpen {
			opened = append(opened, w.Name)
		}
	}
	return opened, nil
}

// joinURLSimple 把 base 与相对 path 拼成绝对 URL（schema 与 site 同义，chain 不依赖 site 包）。
func joinURLSimple(base, path string) string {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + "/" + path
}
