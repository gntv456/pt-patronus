// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// FetchLoginPage 实现 site.LoginCapable.Step A：GET 登录页，探测验证码与登录模式。
// hc 由调用方注入，须配置 FollowRedirect=false；同一条登录的 Step A/B 共用同一 hc。
func (n *NexusPHP) FetchLoginPage(ctx context.Context, hc site.LoginHTTPDoer,
	def *site.SiteMetadata, baseURL string) (*site.CaptchaChallenge, error) {

	def = site.EnsureLoginDefaults(def)
	if !def.Login.Enabled {
		return nil, site.ErrLoginNotSupported
	}
	pageURL := joinURL(baseURL, def.Login.PagePath)
	resp, err := hc.Get(ctx, pageURL, "", true)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse login page: %w", err)
	}

	ch := &site.CaptchaChallenge{ExtraForm: map[string]string{}}
	fm := def.Login.Form

	// 探测挑战响应模式：有 hidden response 且无 password 字段名 → 纯密码 POST 不可行，
	// 须复刻客户端挑战算法（见 nexusphp_crypto.go）。注意：挑战模式仍可能要求图形验证码
	// （login.php 无条件 show_image_code()，取决于 $SECURITY['iv']），故不提前 return——
	// 继续往下探测验证码与 hidden 字段，确保挑战模式登录也能带验证码。
	hasResponseHidden := doc.Find(`input[type="hidden"][name="response"]`).Length() > 0
	hasPasswordField := doc.Find(`input[name="`+fm.Password+`"]`).Length() > 0
	if hasResponseHidden && !hasPasswordField {
		ch.ChallengeMode = true
	}

	// 探测验证码：hidden imagehash 存在即需验证码（对应好学 $iv=='yes'）
	sel := def.Login.Captcha.HashInputSelector
	if sel == "" {
		sel = `input[name="imagehash"]`
	}
	doc.Find(sel).Each(func(_ int, s *goquery.Selection) {
		if ch.ImageHash == "" {
			if v, ok := s.Attr("value"); ok {
				ch.ImageHash = strings.TrimSpace(v)
			}
		}
	})
	ch.NeedCaptcha = ch.ImageHash != ""

	// 收集须回带的 hidden 字段（returnto 等），排除：imagehash/two_step_code（已处理）、
	// response（挑战模式由 PostLogin 算后填，勿被空值覆盖）、password（挑战模式不提交）。
	skip := map[string]bool{
		fm.ImageHash: true, "imagehash": true, fm.TwoStepCode: true,
		"response": true, fm.Password: true, "password": true,
	}
	doc.Find(`input[type="hidden"]`).Each(func(_ int, s *goquery.Selection) {
		name, ok := s.Attr("name")
		if !ok || name == "" || skip[name] {
			return
		}
		if _, exists := ch.ExtraForm[name]; exists {
			return
		}
		if v, ok := s.Attr("value"); ok {
			ch.ExtraForm[name] = v
		}
	})

	// 取验证码图（DB 校验、无需 session）：image.php?action=regimage&imagehash=H&secret=S
	if ch.NeedCaptcha {
		imgURL := buildCaptchaURL(baseURL, def.Login.Captcha.GenPath, ch.ImageHash, ch.ExtraForm["secret"])
		imgResp, err := hc.Get(ctx, imgURL, "", true)
		if err != nil {
			return nil, fmt.Errorf("fetch captcha image: %w", err)
		}
		ch.ImageB64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(imgResp.Body)
	}
	slog.Info("hxpt login page probed",
		"challenge_mode", ch.ChallengeMode,
		"has_response_hidden", hasResponseHidden,
		"has_password_field", hasPasswordField,
		"need_captcha", ch.NeedCaptcha,
		"extra_form_keys", len(ch.ExtraForm))
	return ch, nil
}

// PostLogin 实现 site.LoginCapable.Step B：POST takelogin.php，判定成功并抓用户资料。
// hc 须配置 FollowRedirect=false，以便据 302 Location 区分成败。
func (n *NexusPHP) PostLogin(ctx context.Context, hc site.LoginHTTPDoer, def *site.SiteMetadata,
	baseURL string, creds site.LoginCredentials, ch *site.CaptchaChallenge,
	imagestring, twoStepCode string) (*site.LoginSession, error) {

	def = site.EnsureLoginDefaults(def)
	if !def.Login.Enabled {
		return nil, site.ErrLoginNotSupported
	}
	fm := def.Login.Form

	vals := url.Values{}
	vals.Set(fm.Username, creds.Username)
	if ch != nil && ch.ChallengeMode {
		// 挑战响应模式：复刻好学 render_password_challenge_js（算法见 nexusphp_crypto.go）。
		// POST /api/challenge 取 secret+challenge → 算 response → 提交 response（不提交 password）。
		challenge, secret, err := n.fetchChallenge(ctx, baseURL, creds.Username)
		if err != nil {
			return nil, err
		}
		vals.Set("response", computeChallengeResponse(secret, challenge, creds.Password))
	} else {
		vals.Set(fm.Password, creds.Password)
	}
	if ch != nil && ch.NeedCaptcha && ch.ImageHash != "" {
		vals.Set(fm.ImageHash, ch.ImageHash)
		vals.Set(fm.ImageString, imagestring)
	}
	if twoStepCode != "" {
		vals.Set(fm.TwoStepCode, twoStepCode)
	}
	for k, v := range def.Login.ExtraFields {
		vals.Set(k, v)
	}
	if ch != nil {
		for k, v := range ch.ExtraForm {
			vals.Set(k, v)
		}
	}

	actionURL := joinURL(baseURL, def.Login.ActionPath)
	resp, err := hc.PostForm(ctx, actionURL, "", vals.Encode(), true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if !loginSucceeded(resp) {
		// 诊断：抽出 HTML 正文文本 + 匹配失败关键词，定位好学真实拒绝原因
		// （密码错/二级验证/验证码/锁定/封禁……），客户端仍只给笼统 401。
		txt := htmlText(resp.Body)
		// IP 锁定（失败次数过多）单独抛明确错误——继续重试只会加重锁定。
		if strings.Contains(txt, "锁定") || strings.Contains(txt, "封锁") {
			slog.Warn("hxpt takelogin ip locked", "status", resp.StatusCode,
				"matched_keyword", matchFailKeyword(txt))
			return nil, fmt.Errorf("登录失败次数过多，IP 已被好学锁定；请清除 loginattempts 表或稍后再试")
		}
		slog.Warn("hxpt takelogin rejected",
			"status", resp.StatusCode,
			"location", resp.Header.Get("Location"),
			"challenge_mode", ch != nil && ch.ChallengeMode,
			"had_captcha", ch != nil && ch.NeedCaptcha,
			"matched_keyword", matchFailKeyword(txt),
			"text_snippet", snippet(txt))
		return nil, site.ErrLoginFailed
	}

	// 登录成功：从 302 响应 Set-Cookie 解析凭据（确定性，不依赖 jar 对 3xx 的捕获行为）
	cookie := extractCookies(resp)
	if !hasSecureCookie(cookie) {
		return nil, site.ErrLoginFailed
	}

	ui, err := n.fetchUserInfoVia(ctx, hc, def, baseURL, cookie)
	if err != nil {
		return nil, fmt.Errorf("post-login userinfo: %w", err)
	}
	return &site.LoginSession{Cookie: cookie, UserInfo: ui}, nil
}

// loginSucceeded 判定 takelogin 响应是否登录成功。
// NexusPHP takelogin 成功必 302→index.php（带 Set-Cookie c_secure_*）；失败则 200 登录页/错误文本
// 或 302→login。故：3xx 且不跳 login 视为成功，其余视为失败。
func loginSucceeded(resp *httpclient.Response) bool {
	if resp.StatusCode == 302 || resp.StatusCode == 303 {
		return !strings.Contains(strings.ToLower(resp.Header.Get("Location")), "login")
	}
	body := string(resp.Body)
	if strings.Contains(body, "登录失败") || strings.Contains(body, "验证码") ||
		strings.Contains(body, "不正确") || strings.Contains(body, "二级验证") ||
		strings.Contains(body, "已锁定") {
		return false
	}
	return false // 200 一律视为未成功（成功必 302）
}

// hasSecureCookie 粗验是否拿到好学登录凭据（c_secure_* 或 auth_key）。
func hasSecureCookie(cookie string) bool {
	return strings.Contains(cookie, "c_secure_") || strings.Contains(cookie, "auth_key")
}

// extractCookies 从响应 Set-Cookie 头解析 name=value 对（丢弃 Path/Domain 等属性）。
func extractCookies(resp *httpclient.Response) string {
	parts := make([]string, 0, 4)
	for _, sc := range resp.Header.Values("Set-Cookie") {
		if i := strings.IndexByte(sc, ';'); i >= 0 {
			sc = sc[:i]
		}
		sc = strings.TrimSpace(sc)
		if sc != "" && strings.Contains(sc, "=") {
			parts = append(parts, sc)
		}
	}
	return strings.Join(parts, "; ")
}

// fetchUserInfoVia 用注入的 hc 抓用户资料（与 FetchUserInfo 同语义，复用于登录后取 Level 供 VIP 判定）。
func (n *NexusPHP) fetchUserInfoVia(ctx context.Context, hc site.LoginHTTPDoer,
	def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {

	def = site.EnsureDefaults(def)
	path := def.UserInfo.Path
	if path == "" {
		path = "usercp.php"
	}
	u := joinURL(baseURL, path)
	resp, err := hc.Get(ctx, u, cookie, true)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	return site.ParseUserInfo(doc, def), nil
}

func buildCaptchaURL(base, genPath, imagehash, secret string) string {
	q := url.Values{}
	q.Set("action", "regimage")
	q.Set("imagehash", imagehash)
	if secret != "" {
		q.Set("secret", secret)
	}
	return joinURL(base, genPath) + "?" + q.Encode()
}

// challengeAPIResponse 是好学 POST /api/challenge 的响应外层（NexusPHP API 信封：ret=0 成功）。
type challengeAPIResponse struct {
	Ret  int    `json:"ret"`
	Msg  string `json:"msg"`
	Data struct {
		Challenge string `json:"challenge"`
		Secret    string `json:"secret"`
	} `json:"data"`
}

// fetchChallenge POST /api/challenge {username} 取挑战响应所需的 challenge 与该用户 secret。
// 走 adapter 自带共享 hc（/api/challenge 无状态、不依赖登录 jar），ssrf=true。
// 对应好学 AuthenticateController::challenge：challenge=mksecret() 缓存 "challenge:"+username 300s。
func (n *NexusPHP) fetchChallenge(ctx context.Context, baseURL, username string) (challenge, secret string, err error) {
	body, _ := json.Marshal(map[string]string{"username": username})
	u := joinURL(baseURL, "api/challenge")
	resp, err := n.hc.Do(ctx, http.MethodPost, u, body,
		map[string]string{"Content-Type": "application/json"}, "", true)
	if err != nil {
		return "", "", fmt.Errorf("fetch challenge: %w", err)
	}
	var cr challengeAPIResponse
	if err := json.Unmarshal(resp.Body, &cr); err != nil {
		return "", "", fmt.Errorf("parse challenge: %w", err)
	}
	if cr.Ret != 0 || cr.Data.Challenge == "" {
		msg := cr.Msg
		if msg == "" {
			msg = "challenge API returned non-zero ret"
		}
		return "", "", fmt.Errorf("%s", msg)
	}
	return cr.Data.Challenge, cr.Data.Secret, nil
}

// snippet 压缩空白并截断为最多 n 字符，供日志输出响应体片段（避免 HTML 刷屏）。
func snippet(s string) string {
	const n = 300
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// htmlText 抽出 HTML 正文文本（goquery），供日志定位好学错误页的真实文字。
func htmlText(b []byte) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(b))
	if err != nil {
		return string(b)
	}
	return strings.TrimSpace(doc.Text())
}

// matchFailKeyword 返回好学 takelogin 错误页命中的失败关键词（空=未命中），按特异性排序。
func matchFailKeyword(text string) string {
	for _, kw := range []string{"二级验证", "二级密码", "需要二级", "验证码", "已锁定", "锁定",
		"封禁", "未确认", "未激活", "不正确", "密码错误", "登录失败", "失败"} {
		if strings.Contains(text, kw) {
			return kw
		}
	}
	return ""
}
