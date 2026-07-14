package forumsignin

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// —— NodeSeek：POST {base}/api/attendance/checkIn，JSON 响应 {success,message} ——
// TODO 联调: 真实字段名以 NodeSeek 当前 API 为准（社区逆向契约，httptest mock 覆盖）。

type nodeseekHandler struct{}

func (nodeseekHandler) Key() string   { return "nodeseek" }
func (nodeseekHandler) Label() string { return "NodeSeek" }

func (h nodeseekHandler) Signin(ctx context.Context, c Ctx) (*Feedback, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://www.nodeseek.com"
	}
	url := base + "/api/attendance/checkIn"
	headers := map[string]string{
		"Origin":       base,
		"Referer":      base + "/",
		"Content-Type": "application/json",
	}
	resp, err := c.HC.Do(ctx, "POST", url, nil, headers, c.Cookie, false)
	if err != nil {
		return nil, err
	}
	raw := string(resp.Body)
	fb := &Feedback{Raw: raw}
	if resp.StatusCode >= 400 {
		fb.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return fb, nil
	}
	var nr struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body, &nr); err != nil {
		// 非 JSON（可能登录失效被重定向到 HTML 登录页）
		fb.Message = classifySigninBody(raw)
		fb.Success = strings.Contains(fb.Message, "成功") || strings.Contains(fb.Message, "已签到")
		return fb, nil
	}
	fb.Success = nr.Success || strings.Contains(nr.Message, "签到") // 已签到也视作成功
	fb.Message = strings.TrimSpace(nr.Message)
	if fb.Message == "" {
		if fb.Success {
			fb.Message = "签到成功"
		} else {
			fb.Message = "签到失败"
		}
	}
	return fb, nil
}

// —— NexusPHP 系论坛区签到：GET {base}/{path}（默认 signin.php），HTML 关键词解析 ——
// TODO 联调: 各站签到路径与成功关键词以真实页面为准（path 可在配置行第 4 列覆盖）。

type nexusSignin struct {
	key, label string
}

func (h nexusSignin) Key() string   { return h.key }
func (h nexusSignin) Label() string { return h.label }

func (h nexusSignin) Signin(ctx context.Context, c Ctx) (*Feedback, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("%s: base url empty", h.key)
	}
	path := c.Path
	if path == "" {
		path = "signin.php"
	}
	url := base + "/" + strings.TrimLeft(path, "/")
	// NexusPHP signin 多为 AJAX/表单，GET 带 Referer 通常即可触发每日签到。
	resp, err := c.HC.Get(ctx, url, c.Cookie, false)
	if err != nil {
		return nil, err
	}
	raw := string(resp.Body)
	msg := classifySigninBody(raw)
	if msg == "" {
		if resp.StatusCode >= 400 {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		} else {
			msg = "未知响应（请在 Web 日志查看 Raw）"
		}
	}
	return &Feedback{
		Success: strings.Contains(msg, "成功") || strings.Contains(msg, "已签到"),
		Message: msg,
		Raw:     raw,
	}, nil
}

// classifySigninBody 按关键词粗判 NexusPHP 签到页结果。
func classifySigninBody(body string) string {
	switch {
	case strings.Contains(body, "请先登录"), strings.Contains(body, "请登录"), strings.Contains(body, "您还没有登录"):
		return "cookie 失效，请重新登录"
	case strings.Contains(body, "已经签到"), strings.Contains(body, "已经签过"), strings.Contains(body, "今日已"), strings.Contains(body, "重复签到"):
		return "今日已签到"
	case strings.Contains(body, "签到成功"), strings.Contains(body, "获得"), strings.Contains(body, "成功"):
		return "签到成功"
	}
	return ""
}

// —— 吾爱破解(52pojie.cn)：Discuz! task 系统每日签到 ——
// 吾爱破解已关闭 dsu_paulsign 插件（plugin.php?id=dsu_paulsign:sign 返回"插件不存在或已关闭"），
// 改走 Discuz 内置 task 系统：首页"领取今日签到奖励" → GET home.php?mod=task&do=apply&id=2。
// TODO 联调: apply 响应关键词（成功/已签/未登录）以真实站点当前契约为准，httptest mock 覆盖主路径。

type pojieHandler struct{}

func (pojieHandler) Key() string   { return "52pojie" }
func (pojieHandler) Label() string { return "吾爱破解" }

func (h pojieHandler) Signin(ctx context.Context, c Ctx) (*Feedback, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://www.52pojie.cn"
	}

	// 先 GET 首页提取 formhash（Discuz 防 CSRF；task apply 通常需附带）。
	fh := ""
	if hr, err := c.HC.GetWithHeaders(ctx, base+"/", c.Cookie, map[string]string{"Referer": base + "/"}, false); err == nil {
		fh = extractFormhash(string(hr.Body))
	}

	// GET task apply 触发每日签到（任务 id=2 = 每日签到）。
	applyURL := base + "/home.php?mod=task&do=apply&id=2"
	if fh != "" {
		applyURL += "&formhash=" + fh
	}
	resp, err := c.HC.GetWithHeaders(ctx, applyURL, c.Cookie, map[string]string{"Referer": base + "/home.php?mod=task"}, false)
	if err != nil {
		return nil, err
	}
	raw := string(resp.Body)
	if resp.IsCloudflare() {
		return &Feedback{Success: false, Message: "被 Cloudflare 拦截，稍后重试", Raw: raw}, nil
	}
	msg := classifyPojieTask(raw)
	if msg == "" {
		if resp.StatusCode >= 400 {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		} else {
			msg = "未知响应（请在 Web 日志查看 Raw）"
		}
	}
	return &Feedback{
		Success: strings.Contains(msg, "成功") || strings.Contains(msg, "已签到"),
		Message: msg,
		Reward:  extractPojieReward(raw),
		Raw:     raw,
	}, nil
}

// formhashRe 从 Discuz 页面提取隐藏表单域 formhash（hex），兼容 input 与 JS 两种写法。
var formhashRe = regexp.MustCompile(`(?:name="formhash"[^>]*value="|formhash=)([0-9a-f]{6,16})`)

func extractFormhash(body string) string {
	if m := formhashRe.FindStringSubmatch(body); len(m) >= 2 {
		return m[1]
	}
	return ""
}

// pojieDaysRe 提取"已签到 N 天 / 连续签到 N 天"。
var pojieDaysRe = regexp.MustCompile(`(?:已签到|连续签到)\s*(\d+)\s*天`)

// extractPojieReward 从签到响应摘要连续签到天数（可选）。
func extractPojieReward(body string) string {
	if m := pojieDaysRe.FindStringSubmatch(body); len(m) >= 2 {
		return "连续签到 " + m[1] + " 天"
	}
	return ""
}

// classifyPojieTask 按关键词判定 52pojie/Discuz task 签到结果。
func classifyPojieTask(body string) string {
	switch {
	case strings.Contains(body, "您需要先登录"),
		strings.Contains(body, "必须先登录"),
		strings.Contains(body, "请先登录"),
		strings.Contains(body, "您还没有登录"):
		return "cookie 失效，请重新登录"
	case strings.Contains(body, "已经完成"),
		strings.Contains(body, "已经领取"),
		strings.Contains(body, "今日已"),
		strings.Contains(body, "不可重复"),
		strings.Contains(body, "已经签到"):
		return "今日已签到"
	case strings.Contains(body, "签到成功"),
		strings.Contains(body, "恭喜"),
		strings.Contains(body, "任务完成"),
		strings.Contains(body, "已完成"),
		strings.Contains(body, "获得"),
		strings.Contains(body, "领取成功"):
		return "签到成功"
	}
	return classifySigninBody(body) // 回退到通用 NexusPHP 关键词
}
