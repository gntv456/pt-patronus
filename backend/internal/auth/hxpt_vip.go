package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// VIPStatus 描述好学 VIP 校验结果。
type VIPStatus struct {
	Verified bool   `json:"verified"`
	ID       string `json:"id"`    // 好学用户 ID（外部身份，cookie 登录 upsert 依据）
	Level    string `json:"level"` // VIP 标签（APP专属VIP / 空），仅日志可读
	Username string `json:"username"`
	Message  string `json:"message"`
}

// HxptVIP 通过好学官方 /api/verifyAccess.php 校验 APP 专属 VIP。
//
// 为什么用 API 而非刮 userdetails.php「等级」行：APP专属VIP 是数据库字段
// app_only_vip==1 && app_only_vip_expire>NOW()（见好学 AppVipService），不是
// 显示在等级行的用户头衔——刮等级文本比 "APP专属VIP" 永远不等（历史误判根因）。
// verifyAccess.php 正是好学 APP 自己用的权威 VIP 校验入口，读 cookie 里的
// c_secure_uid/c_secure_pass，返回 {payload:{isVip,...}}。
type HxptVIP struct {
	registry    *site.Registry
	defID       string             // 默认 "hxpt"
	fallbackURL string             // 无定义 URL 时的回退（cfg.AuthSiteURL）
	required    int                // 要求的最低等级（保留字段）
	hc          *httpclient.Client // 调 verifyAccess.php 用
}

// NewHxptVIP 构造校验器。hc 用于 POST verifyAccess.php（生产注入共享 httpclient）。
func NewHxptVIP(registry *site.Registry, defID, fallbackURL string, requiredLevel int, hc *httpclient.Client) *HxptVIP {
	if defID == "" {
		defID = "hxpt"
	}
	return &HxptVIP{registry: registry, defID: defID, fallbackURL: fallbackURL, required: requiredLevel, hc: hc}
}

// RequiredLevel 返回配置的最低等级。
func (h *HxptVIP) RequiredLevel() int { return h.required }

// verifyAccessResponse 是 /api/verifyAccess.php 的 JSON 外层结构。
type verifyAccessResponse struct {
	Success bool   `json:"success"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Payload struct {
		UID         json.Number `json:"uid"`
		Username    string      `json:"username"`
		IsVip       bool        `json:"isVip"`
		VipExpireAt int64       `json:"vipExpireAt"`
	} `json:"payload"`
}

// Verify 用给定 cookie 调好学 verifyAccess.php 判定 APP 专属 VIP。
// 签名保持 (ctx, cookie) *VIPStatus，故 cookie 登录 / VIP 重验 cron 零改动。
func (h *HxptVIP) Verify(ctx context.Context, cookie string) *VIPStatus {
	if cookie == "" {
		return &VIPStatus{Verified: false, Message: "未提供好学 cookie"}
	}
	if h.hc == nil {
		return &VIPStatus{Verified: false, Message: "VIP 校验未启用（httpclient 未注入）"}
	}
	// 好学 logincookie() 只下发 c_secure_pass，但 verifyAccess.php 直接 (int) 读 c_secure_uid
	// （浏览器会话无此 cookie）。从 c_secure_pass 反解 user_id 补入，否则恒为 NO_COOKIE。
	hasPass := parseCookieMap(cookie)["c_secure_pass"] != ""
	passFormat := securePassFormat(parseCookieMap(cookie)["c_secure_pass"])
	cookie = ensureVerifyCookie(cookie)
	synthUID := parseCookieMap(cookie)["c_secure_uid"]
	u := joinPath(h.resolveBaseURL(), "/api/verifyAccess.php")
	// PostWithCookie 用 Jar=nil 的临时 client + 显式 Cookie 头校验，杜绝共享 jar 残留
	// cookie 被复用——否则提交无效 cookie 仍可能带上历史真实凭证通过校验（认证绕过）。
	resp, err := h.hc.PostWithCookie(ctx, u, cookie)
	if err != nil {
		slog.Warn("hxpt verifyAccess debug", "marker", "vip-verify-v2",
			"stage", "request_failed", "err", err.Error(),
			"had_c_secure_pass", hasPass, "synth_uid", synthUID)
		return &VIPStatus{Verified: false, Message: "校验请求失败：" + err.Error()}
	}
	var body verifyAccessResponse
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		slog.Warn("hxpt verifyAccess debug", "marker", "vip-verify-v2",
			"stage", "parse_failed", "status", resp.StatusCode,
			"had_c_secure_pass", hasPass, "synth_uid", synthUID,
			"body", snippetVerify(string(resp.Body)))
		return &VIPStatus{Verified: false, Message: "校验响应解析失败"}
	}
	if !body.Success || resp.StatusCode != http.StatusOK {
		slog.Warn("hxpt verifyAccess debug", "marker", "vip-verify-v2",
			"stage", "api_rejected", "http_status", resp.StatusCode,
			"api_success", body.Success, "api_reason", body.Reason,
			"had_c_secure_pass", hasPass, "synth_uid", synthUID,
			"pass_format", passFormat,
			"body", snippetVerify(string(resp.Body)))
		msg := body.Message
		if msg == "" {
			msg = "非 VIP 或 cookie 无效"
		}
		return &VIPStatus{Verified: false, Message: msg}
	}
	st := &VIPStatus{
		Verified: body.Payload.IsVip,
		ID:       string(body.Payload.UID),
		Username: body.Payload.Username,
		Message:  "非好学 APP 专属 VIP",
	}
	if st.Verified {
		st.Level = "APP专属VIP"
		st.Message = "好学 APP 专属 VIP 验证通过"
	}
	return st
}

// resolveBaseURL 取好学 baseURL（定义 URL 优先，回退 fallbackURL）。
func (h *HxptVIP) resolveBaseURL() string {
	if h.registry != nil {
		if def, ok := h.registry.Definition(h.defID); ok && len(def.URLs) > 0 {
			return def.URLs[0]
		}
	}
	return h.fallbackURL
}

// joinPath 拼接 baseURL 与 path（去重复斜杠）。
func joinPath(base, path string) string {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + "/" + path
}

// ensureVerifyCookie 保证 cookie 带 c_secure_uid：好学 logincookie() 只下发 c_secure_pass，
// 而 verifyAccess.php 直接 (int)$_COOKIE["c_secure_uid"] 读 uid（浏览器/账号登录会话均无此 cookie）。
// 从 c_secure_pass=base64(tokenJson+"."+signature) 的 tokenJson.user_id 反解 uid 并补入；
// uid 本就来自服务端签名 token，属合法提取。已有 c_secure_uid 或无法反解时原样返回。
func ensureVerifyCookie(cookie string) string {
	parts := parseCookieMap(cookie)
	if _, ok := parts["c_secure_uid"]; ok {
		return cookie
	}
	pass, ok := parts["c_secure_pass"]
	if !ok {
		return cookie
	}
	uid, ok := uidFromSecurePass(pass)
	if !ok {
		return cookie
	}
	parts["c_secure_uid"] = uid
	return joinCookieMap(parts)
}

// uidFromSecurePass 从 c_secure_pass 反解 user_id（对齐好学 get_user_id_and_signature_from_cookie）。
// 容错策略：cookie 值尾部常有 base64 伪影（如 + 被当空格），Go 严格解码会在出错处停下，
// 但 tokenJson 在最前面、通常已完整解出——只要解出含 "." 的前段即可取 user_id。
// 先解原值；仅当原值无 "." 时才清洗（还原 + 等）后整段重解，避免删中间字符导致错位。
func uidFromSecurePass(pass string) (string, bool) {
	dec, _ := base64.StdEncoding.DecodeString(pass)
	idx := bytes.IndexByte(dec, '.')
	if idx < 0 {
		dec, _ = base64.StdEncoding.DecodeString(cleanBase64Cookie(pass))
		if idx = bytes.IndexByte(dec, '.'); idx < 0 {
			return "", false
		}
	}
	var t struct {
		UserID json.Number `json:"user_id"`
	}
	if err := json.Unmarshal(dec[:idx], &t); err != nil {
		return "", false
	}
	s := string(t.UserID)
	return s, s != ""
}

// cleanBase64Cookie 清理 cookie 值常见伪影：空白、+ 被当作空格（form-url-encoding）、
// 以及其它非 base64 字符，尽量让 base64 解码成功。
func cleanBase64Cookie(s string) string {
	s = strings.ReplaceAll(s, " ", "+")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '+' || r == '/' || r == '=' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// parseCookieMap 把 "k=v; k2=v2" 解析为 map（值可含 '='，按首个 '=' 切分）。
func parseCookieMap(cookie string) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		m[strings.TrimSpace(kv[0])] = kv[1]
	}
	return m
}

// joinCookieMap 把 map 拼回 "k=v; k2=v2"。
func joinCookieMap(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

// snippetVerify 截断响应体供日志（verifyAccess 响应小且不含密钥，仅裁长度）。
func snippetVerify(s string) string {
	const n = 300
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// securePassFormat 描述 c_secure_pass 的结构（不泄露密钥值），用于诊断 uid 反解失败原因：
// 长度、能否标准 base64 解码、解码错误、解码后长度、是否含 "." 分隔、是否以 "{" 开头。
func securePassFormat(pass string) string {
	if pass == "" {
		return "empty"
	}
	dec, err := base64.StdEncoding.DecodeString(pass)
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	return fmt.Sprintf("len=%d base64_ok=%v base64_err=%q decoded_len=%d dot=%v brace=%v",
		len(pass), err == nil, errStr, len(dec),
		bytes.Contains(dec, []byte(".")),
		len(dec) > 0 && dec[0] == '{')
}
