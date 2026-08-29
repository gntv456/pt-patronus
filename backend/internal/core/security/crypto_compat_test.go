package security

import (
	"crypto/sha256"
	"testing"
)

// TestCrypto_HKDFDomainSeparation 域分隔：同 seed 下新派生密钥 != 旧直取派生，
// 且旧钥加密的存量密文仍可解（平滑迁移，安全审计 P1-1）。
func TestCrypto_HKDFDomainSeparation(t *testing.T) {
	seed := "test-seed-for-hkdf"
	c := NewCrypto(seed)

	// 旧实现：SHA-256 直取（升级前落库密文的派生方式）。
	sum := sha256.Sum256([]byte(seed))
	legacy := &Crypto{key: sum[:]}
	if string(c.key) == string(legacy.key) {
		t.Fatal("HKDF key must differ from legacy SHA-256 key (domain separation)")
	}

	// 旧钥加密 → 新实例必须能解开（回退兼容）。
	enc, err := legacy.Encrypt("存量cookie明文")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Decrypt(enc)
	if err != nil || got != "存量cookie明文" {
		t.Fatalf("legacy ciphertext should decrypt via fallback: got=%q err=%v", got, err)
	}

	// 新钥加密 → 新钥可解（常规往返）。
	enc2, err := c.Encrypt("新密文")
	if err != nil {
		t.Fatal(err)
	}
	got2, err := c.Decrypt(enc2)
	if err != nil || got2 != "新密文" {
		t.Fatalf("roundtrip failed: %q %v", got2, err)
	}
}
