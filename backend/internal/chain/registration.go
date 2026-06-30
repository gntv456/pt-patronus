package chain

import (
	"context"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
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
	Error   string `json:"error,omitempty"`
}

// CheckOne 检测单个目标是否开放注册。
func (c *RegistrationChain) CheckOne(ctx context.Context, w *models.RegistrationWatch) (bool, error) {
	path := w.SignupPath
	if path == "" {
		path = "signup.php"
	}
	u := joinURLSimple(w.URL, path)
	resp, err := c.hc.Get(ctx, u, "", true)
	if err != nil {
		return false, err
	}
	if resp.IsCloudflare() {
		return false, nil // 被 WAF 拦截，视为无法判定（不告警）
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
			return false, nil
		}
	}
	if strings.Contains(finalLower, "login") || strings.Contains(finalLower, "takelogin") {
		return false, nil
	}

	// 开放：命中表单/开放文案
	openTexts := append([]string{}, defaultOpenText...)
	if t := strings.TrimSpace(w.OpenText); t != "" {
		openTexts = append(openTexts, strings.ToLower(t))
	}
	for _, t := range openTexts {
		if strings.Contains(body, t) {
			return true, nil
		}
	}
	return false, nil
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
		open, e := c.CheckOne(ctx, w)
		r := RegCheckResult{WatchID: w.ID, Name: w.Name, Open: open}
		if e != nil {
			r.Error = e.Error()
		}
		out = append(out, r)
	}
	return out
}

// CheckAllAndUpdate 检测全部启用目标、回写状态，返回「新开放」的站点名
// （上次为关闭/从未检测 → 本次开放）。仅在状态转换时返回，供上层去重通知。
func (c *RegistrationChain) CheckAllAndUpdate(ctx context.Context) []string {
	watches, err := c.regOp.ListActive()
	if err != nil {
		return nil
	}
	var opened []string
	for i := range watches {
		w := &watches[i]
		open, e := c.CheckOne(ctx, w)
		if e != nil {
			continue
		}
		prev, err := c.regOp.UpdateStatus(w.ID, open)
		if err != nil {
			continue
		}
		wasOpen := prev != nil && *prev
		if open && !wasOpen {
			opened = append(opened, w.Name)
		}
	}
	return opened
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
