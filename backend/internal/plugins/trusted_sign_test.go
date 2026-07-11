package plugins

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// TestVerifySignatureTrusted 验证受信公钥集签名验证（P1-S2 修复核心）：
// 公钥不再取自市场索引，而来自独立信任锚（内置官方+配置），防市场源自带公钥绕过。
func TestVerifySignatureTrusted(t *testing.T) {
	trustedPub, trustedPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	otherPub, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	payload := []byte("plugin archive content")

	trustedKeyB64 := base64.StdEncoding.EncodeToString(trustedPub)
	otherKeyB64 := base64.StdEncoding.EncodeToString(otherPub)
	trustedSigB64 := base64.StdEncoding.EncodeToString(ed25519.Sign(trustedPriv, payload))
	otherSigB64 := base64.StdEncoding.EncodeToString(ed25519.Sign(otherPriv, payload))

	// 1. 受信公钥 + 正确签名 → 通过
	if err := verifySignatureTrusted([]string{trustedKeyB64}, trustedSigB64, payload); err != nil {
		t.Fatalf("trusted key + valid sig should pass: %v", err)
	}
	// 2. 签名有效但公钥不在受信集 → 拒绝（核心：防市场源自带公钥+签名绕过）
	if err := verifySignatureTrusted([]string{trustedKeyB64}, otherSigB64, payload); err == nil {
		t.Fatal("sig from non-trusted key must fail")
	}
	// 3. 受信集含多个公钥，任一匹配 → 通过
	if err := verifySignatureTrusted([]string{otherKeyB64, trustedKeyB64}, otherSigB64, payload); err != nil {
		t.Fatalf("any trusted key match should pass: %v", err)
	}
	// 4. 未签名 → ErrUnsigned
	if err := verifySignatureTrusted([]string{trustedKeyB64}, "", payload); !errors.Is(err, ErrUnsigned) {
		t.Fatalf("unsigned must return ErrUnsigned, got %v", err)
	}
	// 5. 受信集空 + 有签名 → 拒绝（无信任锚则任何签名都不信）
	if err := verifySignatureTrusted([]string{}, trustedSigB64, payload); err == nil {
		t.Fatal("empty trusted set must fail even with valid sig")
	}
}

// TestManagerDefaultTrustAnchor 验证默认信任锚：内置官方公钥 + AllowUnsigned=false（严格）。
func TestManagerDefaultTrustAnchor(t *testing.T) {
	m := &Manager{} // cfgOp nil → 只含内置官方
	keys := m.TrustedPublicKeys()
	if len(keys) != 1 || keys[0] != OfficialMarketPublicKey {
		t.Fatalf("default trusted keys should be [OfficialMarketPublicKey], got %v", keys)
	}
	if m.AllowUnsigned() {
		t.Fatal("default AllowUnsigned must be false (strict)")
	}
}

// trustTestPublicKey 把测试公钥注入 m 的受信集（经 SystemConfig plugin.trusted_public_keys）。
// 反映生产信任模型：第三方签名插件需用户显式把其公钥加入受信集才能安装——测试自签插件同理。
func trustTestPublicKey(t *testing.T, db *gorm.DB, m *Manager, pub ed25519.PublicKey) {
	t.Helper()
	cfgOp := oper.NewSystemConfigOper(db)
	keys, _ := json.Marshal([]string{base64.StdEncoding.EncodeToString(pub)})
	if err := cfgOp.Set(cfgKeyTrustedPublicKeys, string(keys)); err != nil {
		t.Fatalf("set trusted keys: %v", err)
	}
	m.SetCfgOp(cfgOp)
}
