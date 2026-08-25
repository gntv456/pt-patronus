package cookiecloud

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestMemServerStoreRoundTrip 存取往返 + 未注册 uuid miss。
func TestMemServerStoreRoundTrip(t *testing.T) {
	s := NewMemServerStore()
	env := ServerEnvelope{Encrypted: base64Std("cipher"), CryptoType: "legacy"}
	s.Put("uuid-1", env)
	got, ok := s.Get("uuid-1")
	if !ok || got != env {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
	if _, ok := s.Get("not-exist"); ok {
		t.Error("unknown uuid should miss")
	}
	// 覆盖更新
	s.Put("uuid-1", ServerEnvelope{Encrypted: base64Std("new")})
	if got, _ := s.Get("uuid-1"); got.Encrypted != base64Std("new") {
		t.Errorf("overwrite failed: %+v", got)
	}
}

// TestParseUpdateRequest /update body 解析校验矩阵。
func TestParseUpdateRequest(t *testing.T) {
	// 合法（含 crypto_type）
	req, err := ParseUpdateRequest([]byte(`{"uuid":"abc","encrypted":"` + base64Std("hello") + `","crypto_type":"legacy"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.UUID != "abc" || req.CryptoType != "legacy" {
		t.Fatalf("req=%+v", req)
	}
	// 合法（无 crypto_type，老客户端）
	if _, err := ParseUpdateRequest([]byte(`{"uuid":"abc","encrypted":"` + base64Std("x") + `"}`)); err != nil {
		t.Fatal(err)
	}
	// 缺 uuid / 缺 encrypted / 非 base64 / 非法 json / 超长
	if _, err := ParseUpdateRequest([]byte(`{"encrypted":"` + base64Std("x") + `"}`)); err == nil {
		t.Error("missing uuid should fail")
	}
	if _, err := ParseUpdateRequest([]byte(`{"uuid":"a"}`)); err == nil {
		t.Error("missing encrypted should fail")
	}
	if _, err := ParseUpdateRequest([]byte(`{"uuid":"a","encrypted":"!!!"}`)); err == nil {
		t.Error("invalid base64 should fail")
	}
	if _, err := ParseUpdateRequest([]byte(`{`)); err == nil {
		t.Error("invalid json should fail")
	}
	big := strings.Repeat("A", maxEncryptedLen+1)
	if _, err := ParseUpdateRequest([]byte(`{"uuid":"a","encrypted":"` + big + `"}`)); err == nil {
		t.Error("oversized encrypted should fail")
	}
}

// TestServerEndToEnd 加密推送 → store → MarshalEnvelope → DecodeEnvelope 拉取解密全链路
// （模拟「扩展推上来、chain 拉下来」的完整内置服务端数据面，不含 HTTP）。
func TestServerEndToEnd(t *testing.T) {
	const uuid, pass = "u1", "p1"
	enc, err := EncryptPayload(map[string][]CookieEntry{
		"demo.pt": {{Name: "sess", Value: "abc", Domain: "demo.pt", Path: "/"}},
	}, uuid, pass)
	if err != nil {
		t.Fatal(err)
	}
	// update 存储
	req, err := ParseUpdateRequest([]byte(`{"uuid":"` + uuid + `","encrypted":"` + enc + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemServerStore()
	store.Put(req.UUID, ServerEnvelope{Encrypted: req.Encrypted, CryptoType: req.CryptoType})
	// get 取回 + 拉取端解密
	env, ok := store.Get(uuid)
	if !ok {
		t.Fatal("envelope missing")
	}
	body, err := MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := DecodeEnvelope(body, uuid, pass)
	if err != nil {
		t.Fatal(err)
	}
	if got := CookieStringForHost(payload, "demo.pt"); !strings.Contains(got, "sess=abc") {
		t.Errorf("payload cookie=%q", got)
	}
}

func base64Std(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
