// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"encoding/base64"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// userIDRe 在 base64 解码后的 c_secure_pass payload JSON 里锚定 user_id。
var userIDRe = regexp.MustCompile(`"user_id"\s*:\s*"(\d+)"`)

// ParseCookieHeader 解析 "k=v; k2=v2" 为 map（同名后者覆盖）。
func ParseCookieHeader(s string) map[string]string {
	return parseCookieJar(s)
}

// FormatCookieHeader 将 cookie map 格式化为稳定顺序的 header 串。
func FormatCookieHeader(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	names := make([]string, 0, len(m))
	for k := range m {
		if k == "" {
			continue
		}
		names = append(names, k)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, k := range names {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, "; ")
}

// MergeCookieHeader 合并 cookie header：以 base 为底，overlay 同名覆盖。
// KeepAlive 回写场景下 jar 可能只含 Set-Cookie 轮换后的子集，必须与原 cookie 合并，
// 避免把 c_secure_* / remember_web_* 等未轮换键写丢。
func MergeCookieHeader(base, overlay string) string {
	base = strings.TrimSpace(base)
	overlay = strings.TrimSpace(overlay)
	if overlay == "" {
		return base
	}
	if base == "" {
		return FormatCookieHeader(ParseCookieHeader(overlay))
	}
	m := ParseCookieHeader(base)
	for k, v := range ParseCookieHeader(overlay) {
		m[k] = v
	}
	return FormatCookieHeader(m)
}

// CookieHeaderChanged 比较两段 cookie header 语义是否变化（忽略键顺序）。
func CookieHeaderChanged(a, b string) bool {
	ma, mb := ParseCookieHeader(a), ParseCookieHeader(b)
	if len(ma) != len(mb) {
		return true
	}
	for k, v := range ma {
		if mb[k] != v {
			return true
		}
	}
	return false
}

// ExtractUserIDFromCookie 从 NexusPHP cookie 串解析当前用户 ID。
// 优先 c_secure_pass（URL解码 → 去尾 .xxxx → base64解码 → JSON 正则 user_id），
// 回退 c_secure_uid（base64 数字串或纯数字）。失败返回空串。
//
// 用于 invite.php?id=<uid> / 被邀请人列表 等需自身 uid 的抓取。
func ExtractUserIDFromCookie(cookieStr string) string {
	cj := parseCookieJar(cookieStr)
	if pass, ok := cj["c_secure_pass"]; ok {
		if uid := uidFromSecurePass(pass); uid != "" {
			return uid
		}
	}
	if uid, ok := cj["c_secure_uid"]; ok {
		if d := tryBase64UID(uid); d != "" {
			return d
		}
		if isAllDigits(uid) {
			return uid
		}
	}
	return ""
}

// parseCookieJar 解析 "k=v; k2=v2" 为 map（同名后者覆盖）。
// 手动分割而非 net/http 的 (*http.Request).Cookies()：后者按 RFC 6265 严格校验每个字节，
// PT 站 cookie（c_secure_pass 的 JSON/base64 payload、PHP 序列化值等）常含合法的 '"'，
// 会触发 "net/http: invalid byte '"' in Cookie.Value; dropping invalid bytes" 刷屏日志、
// 且会静默丢弃该字节（破坏 cookie 值）。手动分割忠实保留原值，仅做键值拆分。
func parseCookieJar(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		name := strings.TrimSpace(kv[0])
		if name == "" {
			continue
		}
		out[name] = strings.TrimSpace(kv[1])
	}
	return out
}

func uidFromSecurePass(pass string) string {
	decoded, err := url.QueryUnescape(pass)
	if err != nil {
		decoded = pass
	}
	// 去尾部 ".xxxx" 后缀
	if i := strings.LastIndex(decoded, "."); i >= 0 {
		decoded = decoded[:i]
	}
	raw, err := base64.StdEncoding.DecodeString(padBase64(decoded))
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(padBase64(decoded))
		if err != nil {
			return ""
		}
	}
	if m := userIDRe.FindStringSubmatch(string(raw)); len(m) > 1 {
		return m[1]
	}
	return ""
}

// tryBase64UID 解码 c_secure_uid：纯数字直接用，否则 base64 解码后取数字。
func tryBase64UID(v string) string {
	v = strings.TrimSpace(v)
	if isAllDigits(v) {
		return v
	}
	decoded, err := url.QueryUnescape(v)
	if err != nil {
		decoded = v
	}
	raw, err := base64.StdEncoding.DecodeString(padBase64(decoded))
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(padBase64(decoded))
		if err != nil {
			return ""
		}
	}
	if d := strings.TrimSpace(string(raw)); isAllDigits(d) {
		return d
	}
	return ""
}

func padBase64(s string) string {
	if mod := len(s) % 4; mod != 0 {
		s += strings.Repeat("=", 4-mod)
	}
	return s
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
