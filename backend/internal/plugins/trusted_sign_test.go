// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"archive/zip"
	"bytes"
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

// TestUnzipSafeRejectsBomb 验证 zip 炸弹防护：单条目解压超 32MB 上限必须被拒（P1 zip 炸弹修复）。
// 全零数据高度可压缩：33MB 源压缩后极小，但解压超 32MB → LimitReader 截断 → n>上限 → 拒。
func TestUnzipSafeRejectsBomb(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("big.bin")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	zero := make([]byte, 1<<20) // 1MB
	for i := 0; i < 33; i++ {   // 33MB > 32MB 单条目上限
		if _, err := w.Write(zero); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	dst := t.TempDir()
	if err := unzipSafe(buf.Bytes(), dst); err == nil {
		t.Fatal("zip entry exceeding per-file limit (33MB) must be rejected")
	}
}

// TestUnzipSafeAcceptsNormal 验证正常 zip（小条目）照常通过，防护不影响合法插件。
func TestUnzipSafeAcceptsNormal(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("plugin.py")
	_, _ = w.Write([]byte("print('ok')\n"))
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	dst := t.TempDir()
	if err := unzipSafe(buf.Bytes(), dst); err != nil {
		t.Fatalf("normal zip should pass: %v", err)
	}
}
