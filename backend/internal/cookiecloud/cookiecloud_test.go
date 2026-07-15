package cookiecloud

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
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
