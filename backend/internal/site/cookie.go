package site

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// userIDRe 在 base64 解码后的 c_secure_pass payload JSON 里锚定 user_id。
var userIDRe = regexp.MustCompile(`"user_id"\s*:\s*"(\d+)"`)

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

// parseCookieJar 解析 "k=v; k2=v2" 为 map。
func parseCookieJar(s string) map[string]string {
	out := map[string]string{}
	h := http.Header{}
	h.Add("Cookie", s)
	req := http.Request{Header: h}
	for _, c := range req.Cookies() {
		out[c.Name] = c.Value
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
