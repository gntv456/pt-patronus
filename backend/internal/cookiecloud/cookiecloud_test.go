package cookiecloud

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"strings"
	"testing"
)

// encryptForTest 用与 Decrypt 相同的算法加密，用于自洽往返验证。
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

func TestDeriveKeyDeterministic(t *testing.T) {
	k1 := DeriveKey("mypass")
	k2 := DeriveKey("mypass")
	if string(k1) != string(k2) {
		t.Fatal("key not deterministic")
	}
	if len(k1) != 16 {
		t.Fatalf("key len = %d", len(k1))
	}
}

func TestDecryptRoundTrip(t *testing.T) {
	plain := `{"demo.pt":[{"name":"sess","value":"abc","domain":"demo.pt","path":"/","httpOnly":true}]}`
	key := DeriveKey("mypass")
	enc := encryptForTest(t, plain, key)

	payload, err := DecryptPayload(enc, "mypass")
	if err != nil {
		t.Fatal(err)
	}
	cookie := CookieStringForHost(payload, "demo.pt")
	if !strings.Contains(cookie, "sess=abc") {
		t.Errorf("cookie = %q", cookie)
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
