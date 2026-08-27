package cloud

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"io"
	"testing"
)

func newTestRand() io.Reader { return rand.Reader }

// TestM115RoundTrip 加解密回环：请求侧密钥加密 → 同密钥解密还原（CBC+PKCS7 正确性）。
func TestM115RoundTrip(t *testing.T) {
	k := m115RequestKeys()
	plain := []byte(`{"pickcode":"abc123"}`)
	enc, err := m115Encrypt(k, plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == "" {
		t.Fatal("empty ciphertext")
	}
	dec, err := m115Decrypt(k, enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("roundtrip mismatch: %q", dec)
	}
}

// TestM115ECDHKeyDerivation 双端各自生成密钥对 → 互派生 → 密钥一致（ECDH 语义）。
func TestM115ECDHKeyDerivation(t *testing.T) {
	alice, alicePubHex, err := m115GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	bob, err := ecdh.P256().GenerateKey(newTestRand())
	if err != nil {
		t.Fatal(err)
	}
	bobPubHex := hex.EncodeToString(bob.PublicKey().Bytes())

	k1, err := m115DeriveKeys(alice, bobPubHex)
	if err != nil {
		t.Fatal(err)
	}
	// bob 侧：用 alice 公钥派生（等价路径——直接 ECDH 后同派生）。
	shared, err := bob.ECDH(alice.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	k2Bytes := m115Sum(shared)
	if !bytes.Equal(k1.aesKey, k2Bytes[:16]) || !bytes.Equal(k1.iv, k2Bytes[16:32]) {
		t.Fatal("both sides must derive identical key/iv")
	}

	// 公钥 hex 形态：04 || X || Y 共 65 字节 → 130 hex 字符。
	if len(alicePubHex) != 130 || alicePubHex[:2] != "04" {
		t.Fatalf("bad pub hex form: len=%d", len(alicePubHex))
	}
}

// TestM115DeriveKeysRejectsBadPublicKey 非法服务端公钥报错（不 panic）。
func TestM115DeriveKeysRejectsBadPublicKey(t *testing.T) {
	priv, _, _ := m115GenerateKeyPair()
	cases := []string{"", "zz", "0400", "0500" + hex.EncodeToString(make([]byte, 64))}
	for _, c := range cases {
		if _, err := m115DeriveKeys(priv, c); err == nil {
			t.Fatalf("bad key %q must error", c)
		}
	}
}

// TestM115DecryptRejectsBadCiphertext 非法密文/填充报错（不 panic）。
func TestM115DecryptRejectsBadCiphertext(t *testing.T) {
	k := m115RequestKeys()
	for _, c := range []string{"not-base64!!!", "", "AAAA"} {
		if _, err := m115Decrypt(k, c); err == nil {
			t.Fatalf("bad ciphertext %q must error", c)
		}
	}
}

// TestPKCS7 填充/去填充边界。
func TestPKCS7(t *testing.T) {
	for _, n := range []int{0, 1, 15, 16, 17, 32} {
		data := bytes.Repeat([]byte{0x61}, n)
		padded := pkcs7Pad(data, 16)
		if len(padded)%16 != 0 {
			t.Fatalf("pad len %d not multiple of 16", len(padded))
		}
		back, err := pkcs7Unpad(padded)
		if err != nil || !bytes.Equal(back, data) {
			t.Fatalf("unpad n=%d: %v", n, err)
		}
	}
	// 破坏填充 → 报错
	bad := []byte{1, 2, 3, 99}
	if _, err := pkcs7Unpad(bad); err == nil {
		t.Fatal("corrupted padding must error")
	}
}

// TestTrimHexPrefix 公钥前缀形态归一。
func TestTrimHexPrefix(t *testing.T) {
	if trimHexPrefix("04:abcd") != "abcd" {
		t.Fatal("04: prefix not stripped")
	}
	if trimHexPrefix("0xabcd") != "abcd" {
		t.Fatal("0x prefix not stripped")
	}
	if trimHexPrefix("abcd") != "abcd" {
		t.Fatal("plain hex must pass through")
	}
}
