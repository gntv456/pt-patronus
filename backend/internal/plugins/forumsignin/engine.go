package forumsignin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ForumCred 是配置行解析出的单站凭据。
type ForumCred struct {
	Key     string
	BaseURL string
	Cookie  string
	Path    string
}

// SiteResult 是单站签到结果。
type SiteResult struct {
	Label   string `json:"label"`
	Key     string `json:"key"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Engine 编排多论坛签到：解析凭据 → 逐站 Dispatch → 失败重试 → 通知。
// 由 plugins 层 forum-signin 的 ExecuteAction/Schedule 触发。
type Engine struct {
	rt  Runtime
	hc  *httpclient.Client // 可空：lazy New 兜底
	cfg map[string]any
}

func NewEngine(rt Runtime, hc *httpclient.Client, cfg map[string]any) *Engine {
	return &Engine{rt: rt, hc: hc, cfg: cfg}
}

func (e *Engine) client() (*httpclient.Client, error) {
	if e.hc != nil {
		return e.hc, nil
	}
	c, err := httpclient.New(httpclient.Options{FollowRedirect: true, Timeout: 20 * time.Second})
	if err != nil {
		return nil, err
	}
	e.hc = c
	return c, nil
}

// Run 主任务：跑全部已配置论坛的签到。
func (e *Engine) Run(ctx context.Context) []SiteResult {
	hc, err := e.client()
	if err != nil {
		e.rt.Log(logError, "run", "http client unavailable: "+err.Error(), nil)
		return nil
	}
	creds := parseForums(strCfg(e.cfg, "forums"))
	if len(creds) == 0 {
		e.rt.Log(logWarning, "run", "no forums configured (forums empty)", nil)
		return nil
	}

	var results []SiteResult
	var failed []ForumCred
	for _, cred := range creds {
		h := Lookup(cred.Key)
		if h == nil {
			e.rt.Log(logWarning, "run", "unknown forum key: "+cred.Key, nil)
			continue
		}
		if strings.TrimSpace(cred.Cookie) == "" {
			e.rt.Log(logWarning, "run", h.Label()+" cookie empty, skip", nil)
			continue
		}
		c := Ctx{HC: hc, Key: cred.Key, BaseURL: cred.BaseURL, Cookie: cred.Cookie, Path: cred.Path, Cfg: e.cfg}
		fb, sErr := h.Signin(ctx, c)
		res := SiteResult{Label: h.Label(), Key: cred.Key}
		if sErr != nil {
			res.Message = "失败：" + sErr.Error()
			failed = append(failed, cred)
		} else if fb != nil {
			res.Success = fb.Success
			res.Message = fb.Message
			if !fb.Success {
				failed = append(failed, cred)
			}
		} else {
			res.Message = "无响应"
			failed = append(failed, cred)
		}
		e.rt.Log(logInfo, "signin", h.Label()+": "+res.Message, map[string]any{"key": cred.Key, "success": res.Success})
		results = append(results, res)
	}

	if boolCfg(e.cfg, "notify", true) && len(results) > 0 {
		e.rt.Notice(ctx, "论坛签到任务完成", composeReport(results), levelFor(results))
	}
	if len(failed) > 0 {
		e.enqueueRetry(ctx, failed)
	}
	e.rt.Log(logInfo, "run", fmt.Sprintf("forum signin: %d sites, %d failed", len(results), len(failed)), nil)
	return results
}

func composeReport(results []SiteResult) string {
	var b strings.Builder
	ok, fail := 0, 0
	for _, r := range results {
		if r.Success {
			ok++
		} else {
			fail++
		}
	}
	fmt.Fprintf(&b, "成功 %d / 失败 %d：\n", ok, fail)
	for _, r := range results {
		mark := "✓"
		if !r.Success {
			mark = "✗"
		}
		fmt.Fprintf(&b, "%s %s：%s\n", mark, r.Label, r.Message)
	}
	return b.String()
}

func levelFor(results []SiteResult) string {
	for _, r := range results {
		if !r.Success {
			return "warning"
		}
	}
	return "info"
}

// parseForums 解析配置文本：每行 `key|base|cookie[|path]`（cookie 为第 3 列，可含除换行外任意字符）。
// 以 # 开头或空行忽略；2 列形式 `key|cookie` 视作 base 空。
func parseForums(text string) []ForumCred {
	var out []ForumCred
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		var cred ForumCred
		switch len(parts) {
		case 4:
			cred = ForumCred{Key: strings.TrimSpace(parts[0]), BaseURL: strings.TrimSpace(parts[1]), Cookie: parts[2], Path: strings.TrimSpace(parts[3])}
		case 3:
			cred = ForumCred{Key: strings.TrimSpace(parts[0]), BaseURL: strings.TrimSpace(parts[1]), Cookie: parts[2]}
		case 2:
			cred = ForumCred{Key: strings.TrimSpace(parts[0]), Cookie: parts[1]}
		default:
			continue
		}
		if cred.Key == "" {
			continue
		}
		cred.Cookie = strings.TrimSpace(cred.Cookie)
		out = append(out, cred)
	}
	return out
}

// —— 失败重试（KV 持久化 + time.AfterFunc 一次性递归，对标 groupchat）——

const kvFailed = "failed_forums"

func (e *Engine) enqueueRetry(ctx context.Context, failed []ForumCred) {
	retryCount := intCfg(e.cfg, "retry_count", 1)
	if retryCount <= 0 || len(failed) == 0 {
		_ = e.rt.KVSet(kvFailed, []ForumCred{})
		return
	}
	_ = e.rt.KVSet(kvFailed, failed)
	retryInterval := intCfg(e.cfg, "retry_interval", 10)
	next := time.Now().Add(time.Duration(retryInterval) * time.Minute)
	e.rt.Log(logInfo, "retry", fmt.Sprintf("enqueued %d failed, retry at %s", len(failed), next.Format("15:04:05")), nil)
	time.AfterFunc(time.Duration(retryInterval)*time.Minute, func() {
		e.executeRetry(context.Background(), retryCount-1)
	})
}

func (e *Engine) executeRetry(ctx context.Context, passesLeft int) {
	var failed []ForumCred
	if err := e.rt.KVGet(kvFailed, &failed); err != nil || len(failed) == 0 {
		return
	}
	hc, err := e.client()
	if err != nil {
		e.rt.Log(logError, "retry", "http client unavailable: "+err.Error(), nil)
		return
	}
	var still []ForumCred
	for _, cred := range failed {
		h := Lookup(cred.Key)
		if h == nil {
			continue
		}
		c := Ctx{HC: hc, Key: cred.Key, BaseURL: cred.BaseURL, Cookie: cred.Cookie, Path: cred.Path, Cfg: e.cfg}
		fb, sErr := h.Signin(ctx, c)
		if sErr != nil || fb == nil || !fb.Success {
			still = append(still, cred)
		}
	}
	if len(still) == 0 {
		_ = e.rt.KVSet(kvFailed, []ForumCred{})
		e.rt.Log(logInfo, "retry", "all failed signins recovered", nil)
		return
	}
	_ = e.rt.KVSet(kvFailed, still)
	if passesLeft > 0 {
		retryInterval := intCfg(e.cfg, "retry_interval", 10)
		time.AfterFunc(time.Duration(retryInterval)*time.Minute, func() {
			e.executeRetry(context.Background(), passesLeft-1)
		})
		e.rt.Log(logInfo, "retry", fmt.Sprintf("%d still failed, %d passes left", len(still), passesLeft), nil)
	} else {
		_ = e.rt.KVSet(kvFailed, []ForumCred{})
		if boolCfg(e.cfg, "notify", true) {
			e.rt.Notice(ctx, "论坛签到重试终止", fmt.Sprintf("仍有 %d 个论坛签到失败，已达重试上限。", len(still)), "warning")
		}
	}
}

// —— 辅助（与 groupchat 同款）——

func boolCfg(cfg map[string]any, key string, def bool) bool {
	v, ok := cfg[key]
	if !ok || v == nil {
		return def
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "1" || x == "yes"
	default:
		return def
	}
}

func intCfg(cfg map[string]any, key string, def int) int {
	v, ok := cfg[key]
	if !ok || v == nil {
		return def
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return def
}

func strCfg(cfg map[string]any, key string) string {
	v, ok := cfg[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}
