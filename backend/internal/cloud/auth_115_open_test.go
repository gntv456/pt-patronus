// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// TestPKCEVerifier PKCE 编码：verifier URL 安全、challenge=RawURL(b64(sha256(verifier)))。
func TestPKCEVerifier(t *testing.T) {
	v, c, err := PKCEVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if len(v) < 43 || strings.ContainsAny(v, "+/=") {
		t.Errorf("verifier not url-safe: %q", v)
	}
	sum := sha256.Sum256([]byte(v))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if c != want {
		t.Errorf("challenge mismatch")
	}
	// 每次不同
	v2, _, _ := PKCEVerifier()
	if v2 == v {
		t.Error("verifier should be random")
	}
}

// TestTokenPairCodec 双令牌 JSON 编解码 + 裸 access_token 兼容。
func TestTokenPairCodec(t *testing.T) {
	tp := TokenPair{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 7776000}
	s := EncodeTokenPair(tp)
	if !strings.Contains(s, "refresh_token") {
		t.Errorf("encode lost refresh: %s", s)
	}
	got := DecodeTokenPair(s)
	if got.AccessToken != "at" || got.RefreshToken != "rt" {
		t.Errorf("decode: %+v", got)
	}
	// 裸串兼容（旧数据/手工填）
	if got := DecodeTokenPair("plain-token"); got.AccessToken != "plain-token" || got.RefreshToken != "" {
		t.Errorf("plain decode: %+v", got)
	}
	// 空串
	if got := DecodeTokenPair("  "); got.AccessToken != "" {
		t.Errorf("empty decode: %+v", got)
	}
}

// authFakeDoer 按调用序返回预设响应（授权流：status→token 两段）。
type authFakeDoer struct {
	responses []string
	calls     []string
}

func (f *authFakeDoer) Do(_ context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error) {
	f.calls = append(f.calls, rawURL)
	if len(f.responses) == 0 {
		return &httpclient.Response{StatusCode: 200, Body: []byte(`{}`)}, nil
	}
	r := f.responses[0]
	f.responses = f.responses[1:]
	return &httpclient.Response{StatusCode: 200, Body: []byte(r)}, nil
}

// TestDeviceCodeFlow 授权三段：authDeviceCode → 轮询(2 等待/3 确认) → deviceCodeToToken。
func TestDeviceCodeFlow(t *testing.T) {
	doer := &authFakeDoer{responses: []string{
		`{"uid":123,"time":456,"qrcode":"https://qrcode.115.com/xyz","sign":"s1"}`,
	}}
	code, verifier, err := StartDeviceCode115(context.TODO(), doer, "app-1")
	if err != nil {
		t.Fatal(err)
	}
	if code.QRCode == "" || verifier == "" {
		t.Fatalf("code/verifier empty: %+v %q", code, verifier)
	}
	// form 含 client_id 与 sha256 method
	if !strings.Contains(doer.calls[0], "authDeviceCode") {
		t.Errorf("unexpected call: %s", doer.calls[0])
	}

	// 状态=2（已扫待确认）
	doer.responses = []string{`{"status":2,"msg":"scan"}`}
	ok, st, _, err := PollDeviceCode115(context.TODO(), doer, code, verifier)
	if err != nil || ok || st != 2 {
		t.Fatalf("poll status=2: ok=%v st=%d err=%v", ok, st, err)
	}
	// 状态=3 → 换 token
	doer.responses = []string{`{"status":3,"msg":"ok"}`, `{"access_token":"at9","refresh_token":"rt9","expires_in":3600}`}
	ok, st, tp, err := PollDeviceCode115(context.TODO(), doer, code, verifier)
	if err != nil || !ok || tp.AccessToken != "at9" || tp.RefreshToken != "rt9" {
		t.Fatalf("poll confirmed: ok=%v tp=%+v err=%v", ok, tp, err)
	}
	_ = st
}

// TestRefresh115Open refresh 轮换解析。
func TestRefresh115Open(t *testing.T) {
	doer := &authFakeDoer{responses: []string{
		`{"access_token":"new-at","refresh_token":"new-rt","expires_in":7776000}`,
	}}
	tp, err := Refresh115Open(context.TODO(), doer, "old-rt")
	if err != nil || tp.AccessToken != "new-at" || tp.RefreshToken != "new-rt" {
		t.Fatalf("refresh: %+v err=%v", tp, err)
	}
}

// Test115OpenBearerAndRefresh client 侧 bearer 解析 + 刷新回写。
func Test115OpenBearerAndRefresh(t *testing.T) {
	c := &client115Open{drive: Drive{Token: EncodeTokenPair(TokenPair{AccessToken: "a1", RefreshToken: "r1"})}}
	if c.bearer() != "a1" {
		t.Errorf("bearer=%q", c.bearer())
	}
	var written TokenPair
	c.onTokenRefresh = func(tp TokenPair) { written = tp }
	c.hc = &authFakeDoer{responses: []string{`{"access_token":"a2","refresh_token":"r2"}`}}
	if err := c.tryRefresh(context.TODO()); err != nil {
		t.Fatal(err)
	}
	if c.bearer() != "a2" || written.AccessToken != "a2" {
		t.Errorf("after refresh bearer=%q written=%+v", c.bearer(), written)
	}
	// 无 refresh_token 报错
	c2 := &client115Open{drive: Drive{Token: "plain"}}
	if err := c2.tryRefresh(context.TODO()); err == nil {
		t.Error("plain token without refresh should error")
	}
}

// TestAuthInvalid115Open 失效判定。
func TestAuthInvalid115Open(t *testing.T) {
	if !authInvalid115Open([]byte(`{"state":false,"errno":401,"error":"err"}`)) {
		t.Error("errno 401 should be invalid")
	}
	if !authInvalid115Open([]byte(`{"state":false,"code":99}`)) {
		t.Error("code 99 should be invalid")
	}
	if authInvalid115Open([]byte(`{"state":true}`)) {
		t.Error("ok response should not be invalid")
	}
}
