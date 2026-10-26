package cookiecloud

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// encryptForTest 用与 Decrypt 相同的标准协议算法（key 模式 AES-128-CBC，IV=key，PKCS7）加密，
// 用于自洽往返验证。注意它复刻的是「标准 CookieCloud 加密」，不是本包任意自创算法。
func encryptForTest(t *testing.T, plain string, key []byte) string {
	t.Helper()
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append([]byte(plain), strings.Repeat(string(rune(pad)), pad)...)
	block, err := aes.NewCipher(key[:aes.BlockSize])
	if err != nil {
		t.Fatal(err)
	}
	iv := key[:aes.BlockSize]
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out)
}

// TestDeriveKeyStandardVector 钉死标准 CookieCloud key 派生公式：
// key = md5(uuid + "-" + password) 的 hex 前 16 字符。
// 用 crypto/md5 独立计算（不经 DeriveKey）比对，确保派生与 easychen 官方扩展一致而非自洽。
func TestDeriveKeyStandardVector(t *testing.T) {
	sum := md5.Sum([]byte("test-uuid-test-pass"))
	want := []byte(hex.EncodeToString(sum[:])[:16])
	got := DeriveKey("test-uuid", "test-pass")
	if !bytes.Equal(got, want) {
		t.Fatalf("DeriveKey = %q, want standard md5(uuid-password)[:16] = %q", got, want)
	}
	if len(got) != 16 {
		t.Fatalf("key len = %d, want 16", len(got))
	}
}

func TestDeriveKeyDeterministic(t *testing.T) {
	if !bytes.Equal(DeriveKey("u", "p"), DeriveKey("u", "p")) {
		t.Fatal("key not deterministic")
	}
	// uuid 或 password 变化必须改变 key（防退化成忽略 uuid 的旧错误实现）
	if bytes.Equal(DeriveKey("u1", "p"), DeriveKey("u2", "p")) {
		t.Error("different uuid produced same key")
	}
}

// TestDecryptRoundTrip 标准格式 {"cookie_data": {host: [cookies]}} 往返。
func TestDecryptRoundTrip(t *testing.T) {
	plain := `{"cookie_data":{"demo.pt":[{"name":"sess","value":"abc","domain":"demo.pt","path":"/","httpOnly":true}]},"update_time":1700000000}`
	key := DeriveKey("test-uuid", "test-pass")
	enc := encryptForTest(t, plain, key)

	payload, err := DecryptPayload(enc, "test-uuid", "test-pass")
	if err != nil {
		t.Fatal(err)
	}
	cookie := CookieStringForHost(payload, "demo.pt")
	if !strings.Contains(cookie, "sess=abc") {
		t.Errorf("cookie = %q, want sess=abc", cookie)
	}
}

// TestDecryptPayload_NoWrapper 向后兼容无 cookie_data 包裹的直接 {host: [cookies]} 格式。
func TestDecryptPayload_NoWrapper(t *testing.T) {
	plain := `{"demo.pt":[{"name":"sess","value":"xyz","domain":"demo.pt","path":"/"}]}`
	key := DeriveKey("u", "p")
	enc := encryptForTest(t, plain, key)

	payload, err := DecryptPayload(enc, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	cookie := CookieStringForHost(payload, "demo.pt")
	if !strings.Contains(cookie, "sess=xyz") {
		t.Errorf("cookie = %q, want sess=xyz", cookie)
	}
}

// TestDecodeEnvelope 解析服务器响应 envelope {encrypted:...}。
func TestDecodeEnvelope(t *testing.T) {
	plain := `{"cookie_data":{"host.io":[{"name":"t","value":"v","domain":"host.io","path":"/"}]}}`
	enc := encryptForTest(t, plain, DeriveKey("u", "p"))
	body := []byte(`{"encrypted":"` + enc + `"}`)

	payload, err := DecodeEnvelope(body, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	if got := CookieStringForHost(payload, "host.io"); !strings.Contains(got, "t=v") {
		t.Errorf("got %q", got)
	}
}

func TestHostMatches(t *testing.T) {
	if !hostMatches(".demo.pt", "demo.pt") {
		t.Error("should match exact (dot-prefixed)")
	}
	if !hostMatches("demo.pt", "www.demo.pt") {
		t.Error("should match subdomain")
	}
	if hostMatches("demo.pt", "evil-demo.pt") {
		t.Error("should not match suffix trap")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plain := `{"cookie_data":{"demo.pt":[{"name":"sess","value":"xyz","domain":"demo.pt","path":"/"}]},"update_time":1}`
	enc, err := Encrypt([]byte(plain), DeriveKey("u", "p"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := DecryptPayload(enc, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(payload["demo.pt"]) != 1 || payload["demo.pt"][0].Value != "xyz" {
		t.Fatalf("payload=%v", payload)
	}
}

func TestEntriesFromCookieHeader(t *testing.T) {
	es := EntriesFromCookieHeader("demo.pt", "a=1; b=2")
	if len(es) != 2 || es[0].Name != "a" || es[1].Value != "2" {
		t.Fatalf("entries=%v", es)
	}
	if es[0].Domain != "demo.pt" || es[0].Path != "/" || !es[0].Secure {
		t.Fatalf("defaults wrong: %+v", es[0])
	}
}

func TestMergeCookieData(t *testing.T) {
	dst := map[string][]CookieEntry{"old.io": {{Name: "o", Value: "1"}}}
	src := map[string][]CookieEntry{"demo.pt": {{Name: "s", Value: "2"}}}
	got := MergeCookieData(dst, src)
	if len(got["old.io"]) != 1 || len(got["demo.pt"]) != 1 {
		t.Fatalf("merge=%v", got)
	}
}

// TestDecryptLegacyCryptoJSVector 用 CryptoJS（官方扩展 legacy 算法）生成的真实密文做交叉验证：
// CryptoJS.AES.encrypt(data, the_key) → "U2FsdGVkX1..."（OpenSSL 兼容格式，Salted__ + 盐 +
// AES-256-CBC，EvpKDF(MD5,1轮) 从 the_key 派生 32B key + 16B IV）。向量由 Node crypto-js 4.x
// 独立生成钉死，防实现自洽。
func TestDecryptLegacyCryptoJSVector(t *testing.T) {
	const (
		vecUUID   = "legacy-uuid"
		vecPass   = "legacy-pass"
		vecCipher = "U2FsdGVkX1/qFh/VdGNhvZGWlZ1LL45JXemK4n1nyiJxDN085/9wO+F6hMxZdo7Ukt+a/Lw9FHY0HSs6TwB7FnzwPXN/hQS9RUipleJk6Qml6eWNxdBj11c7qXiCKyuUHLwx9Dt2PfKx9mlH6BXIgOpQwk2WxvKYkZX9yxYZgCYFlWVSaw7YXYI8Suk1RRlZpHVDuR7nDm5o5Hjpm3CZF3YvYtoT3epY2BGNZNxIMfU="
	)
	// the_key 独立按公式重算（与 DeriveKey 相同公式），密文本身来自 CryptoJS
	key := DeriveKey(vecUUID, vecPass)
	plain, err := Decrypt(vecCipher, key)
	if err != nil {
		t.Fatal(err)
	}
	s := string(plain)
	if !strings.Contains(s, `"demo.pt"`) || !strings.Contains(s, `"sess"`) || !strings.Contains(s, `"abc"`) {
		t.Fatalf("legacy decrypt content mismatch: %.120s", s)
	}
}

// TestDecryptLegacyFullPayload 走 DecryptPayload 全链路（含 JSON 解析）验证 legacy 向量。
func TestDecryptLegacyFullPayload(t *testing.T) {
	cipher := "U2FsdGVkX1+5t9sK+9sxHaT11/GVUe5FSh+XloHEJmtP1wUI28GVZsV7vk0qU747" +
		"DvjbuEB0BlEY9G6TiOuzEwjI9uKLx7GGG5QJRXDirK+3ARb+LRaytTIyL2JELA/N" +
		"Lc6sEJmys+zI7S9EboWZX4Mxf58C2tGH/EyVUVZklicuF9KtkPVScIIhDHSNdfMM"
	payload, err := DecryptPayload(cipher, "fixed-iv-uuid", "p2-secret")
	if err != nil {
		t.Fatal(err)
	}
	if got := CookieStringForHost(payload, "www.a.pt"); !strings.Contains(got, "uid=1") {
		t.Errorf("www.a.pt cookies = %q", got)
	}
	if got := CookieStringForHost(payload, "b.org"); !strings.Contains(got, "sess=xyz") {
		t.Errorf("b.org cookies = %q", got)
	}
}

// TestDecodeEnvelope_CryptoTypeField 服务端响应带 crypto_type=legacy 标记（新 api/app.js）时
// 同样能解（按密文前缀自动识别，字段仅作元信息）。
func TestDecodeEnvelope_CryptoTypeField(t *testing.T) {
	plain := `{"cookie_data":{"host.io":[{"name":"t","value":"v","domain":"host.io","path":"/"}]}}`
	enc := encryptForTest(t, plain, DeriveKey("u", "p"))
	body := []byte(`{"encrypted":"` + enc + `","crypto_type":"legacy"}`)

	payload, err := DecodeEnvelope(body, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	if got := CookieStringForHost(payload, "host.io"); !strings.Contains(got, "t=v") {
		t.Errorf("got %q", got)
	}
}

// 密码不对时必须是可归类的 ErrBadPassword（上层据此给「密码错或数据损坏」的人话），
// 而不再是裸英文 "bad pkcs7 padding"。
func TestDecryptPayloadWrongPasswordSentinel(t *testing.T) {
	enc, err := EncryptPayload(map[string][]CookieEntry{
		"demo.pt": {{Name: "sess", Value: "v", Domain: "demo.pt", Path: "/"}},
	}, "u-sentinel", "right-pw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptPayload(enc, "u-sentinel", "wrong-pw"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("want ErrBadPassword, got %v", err)
	}
	if _, err := DecryptPayload("not-base64!!!", "u-sentinel", "right-pw"); !errors.Is(err, ErrCorruptPayload) {
		t.Fatalf("bad base64 should be ErrCorruptPayload, got %v", err)
	}
}
