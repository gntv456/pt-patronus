package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// newVIPTestClient 构造可打 127.0.0.1 测试服务器的 httpclient。
func newVIPTestClient(t *testing.T) *httpclient.Client {
	t.Helper()
	c, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// verifyAccessHandler 按 /api/verifyAccess.php 契约返回固定 JSON（isVip 由参数定）。
func verifyAccessHandler(t *testing.T, isVip bool, uid int, msg string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/verifyAccess.php" || r.Method != http.MethodPost {
			w.WriteHeader(404)
			return
		}
		if msg != "" { // 模拟失败响应
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"reason":"NO_COOKIE","message":"` + msg + `"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"token":"t.sig","expiresIn":1800,"payload":{"uid":` + itoa(uid) + `,"username":"alice","isVip":` + boolStrJS(isVip) + `,"vipExpireAt":0,"iat":1,"exp":2}}`))
	})
}

func TestHxptVIPVerifyAccess_VIP(t *testing.T) {
	srv := httptest.NewServer(verifyAccessHandler(t, true, 8, ""))
	defer srv.Close()
	h := NewHxptVIP(nil, "hxpt", srv.URL, 0, newVIPTestClient(t))

	st := h.Verify(context.Background(), "c_secure_uid=8; c_secure_pass=abc")
	if !st.Verified {
		t.Fatalf("expected VIP, got %+v", st)
	}
	if st.ID != "8" {
		t.Errorf("ID = %q, want 8", st.ID)
	}
	if st.Level != "APP专属VIP" {
		t.Errorf("Level = %q, want APP专属VIP", st.Level)
	}
	if st.Username != "alice" {
		t.Errorf("Username = %q", st.Username)
	}
}

func TestHxptVIPVerifyAccess_NonVIP(t *testing.T) {
	srv := httptest.NewServer(verifyAccessHandler(t, false, 99, ""))
	defer srv.Close()
	h := NewHxptVIP(nil, "hxpt", srv.URL, 0, newVIPTestClient(t))

	st := h.Verify(context.Background(), "c_secure_uid=99; c_secure_pass=abc")
	if st.Verified {
		t.Fatalf("expected non-VIP, got %+v", st)
	}
	if st.ID != "99" {
		t.Errorf("ID = %q, want 99", st.ID)
	}
}

func TestHxptVIPVerifyAccess_EmptyCookie(t *testing.T) {
	h := NewHxptVIP(nil, "hxpt", "https://www.hxpt.org", 0, newVIPTestClient(t))
	st := h.Verify(context.Background(), "")
	if st.Verified {
		t.Fatal("empty cookie must not verify")
	}
}

func TestHxptVIPVerifyAccess_FailureResponse(t *testing.T) {
	srv := httptest.NewServer(verifyAccessHandler(t, false, 0, "未检测到有效登录凭证"))
	defer srv.Close()
	h := NewHxptVIP(nil, "hxpt", srv.URL, 0, newVIPTestClient(t))

	st := h.Verify(context.Background(), "stale=cookie")
	if st.Verified {
		t.Fatal("failure response must not verify")
	}
	if st.Message == "" {
		t.Error("expected non-empty message on failure")
	}
}

// itoa / boolStrJS 是测试用的小工具（避免引入 strconv 以保持文件聚焦）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func boolStrJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// makeSecurePass 构造好学格式 c_secure_pass=base64(tokenJSON+"."+signature)。
func makeSecurePass(uid int) string {
	tokenJSON := `{"user_id":` + itoa(uid) + `,"expires":9999999999}`
	return base64.StdEncoding.EncodeToString([]byte(tokenJSON + ".fakesig"))
}

// TestEnsureVerifyCookie 验证从 c_secure_pass 反解 uid 补入 c_secure_uid（修复 NO_COOKIE）。
func TestEnsureVerifyCookie(t *testing.T) {
	in := "c_secure_pass=" + makeSecurePass(8) + "; c_lang_folder=chs"
	out := ensureVerifyCookie(in)
	m := parseCookieMap(out)
	if m["c_secure_uid"] != "8" {
		t.Errorf("c_secure_uid = %q, want 8 (derived from c_secure_pass)", m["c_secure_uid"])
	}
	if m["c_secure_pass"] == "" {
		t.Error("c_secure_pass must be preserved")
	}
	if m["c_lang_folder"] != "chs" {
		t.Error("other cookies must be preserved")
	}
}

// TestEnsureVerifyCookie_AlreadyHasUID 已有 c_secure_uid 时不动。
func TestEnsureVerifyCookie_AlreadyHasUID(t *testing.T) {
	in := "c_secure_uid=42; c_secure_pass=" + makeSecurePass(8)
	if ensureVerifyCookie(in) != in {
		t.Error("must return unchanged when c_secure_uid already present")
	}
}

// TestUIDFromSecurePass_TolerantArtifact 验证容错解码：cookie 值尾部有 base64 伪影
// （如 + 被当空格注入），仍能从已解出的前段 tokenJson 取到 user_id（修复 synth_uid 空）。
func TestUIDFromSecurePass_TolerantArtifact(t *testing.T) {
	clean := makeSecurePass(8)
	// 在尾部注入一个空格（模拟 +→空格伪影），破坏 base64 末段
	artifact := clean[:len(clean)-3] + " " + clean[len(clean)-3:]
	uid, ok := uidFromSecurePass(artifact)
	if !ok || uid != "8" {
		t.Errorf("uidFromSecurePass(artifact) = %q,%v, want 8,true", uid, ok)
	}
}

// TestHxptVIPVerify_NoJarReuse 回归：共享 client 的 cookie jar 中残留的真实凭证不得被
// Verify 复用。先在同一 client 上用有效 cookie 请求一次（种入 jar，模拟先前的真实登录/抓取），
// 再用无效 cookie "x" 校验——必须返回未通过，证明 verifyAccess 只携带本次提交的 cookie。
// 修复前 Verify 走共享 jar，seedCookies 对无 '=' 的 "x" 不更新 jar，请求沿用残留的
// c_secure_pass 通过校验 → Verified=true（越权登录为残留用户）。
func TestHxptVIPVerify_NoJarReuse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if ck, _ := r.Cookie("c_secure_pass"); ck == nil || ck.Value == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"reason":"NO_COOKIE","message":"未检测到有效登录凭证"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"payload":{"uid":8,"username":"gntv","isVip":true}}`))
	}))
	defer srv.Close()
	hc := newVIPTestClient(t)
	// 先在同一共享 client 上用有效 cookie 请求，把 c_secure_pass 种入 jar（模拟历史残留）。
	if _, err := hc.Do(context.Background(), http.MethodPost, srv.URL+"/api/verifyAccess.php",
		nil, map[string]string{"Content-Type": "application/json"},
		"c_secure_uid=8; c_secure_pass=realtoken", false); err != nil {
		t.Fatalf("seed jar: %v", err)
	}
	h := NewHxptVIP(nil, "hxpt", srv.URL, 0, hc)

	st := h.Verify(context.Background(), "x")
	if st.Verified {
		t.Fatalf("invalid cookie must NOT verify via jar reuse: verified=true id=%q user=%q (认证绕过)", st.ID, st.Username)
	}
}

// TestVerifyAccess_ReceivesSynthesizedUID 端到端：浏览器 cookie（仅 c_secure_pass）
// 经 Verify 后，verifyAccess 收到补入的 c_secure_uid 并返回 VIP。
func TestVerifyAccess_ReceivesSynthesizedUID(t *testing.T) {
	var gotUID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUID = r.FormValue("c_secure_uid") // FormValue 也读 Cookie？否——验证实际 Cookie 头
		// 读 Cookie 头里的 c_secure_uid
		ck, _ := r.Cookie("c_secure_uid")
		gotUID = ck.Value
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"payload":{"uid":8,"username":"alice","isVip":true}}`))
	}))
	defer srv.Close()
	h := NewHxptVIP(nil, "hxpt", srv.URL, 0, newVIPTestClient(t))

	st := h.Verify(context.Background(), "c_secure_pass="+makeSecurePass(8))
	if gotUID != "8" {
		t.Errorf("verifyAccess received c_secure_uid=%q, want 8", gotUID)
	}
	if !st.Verified {
		t.Fatalf("expected VIP after uid synthesis, got %+v", st)
	}
}
