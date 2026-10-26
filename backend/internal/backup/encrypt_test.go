package backup

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
)

// TestEncryptDecryptRoundTrip 加密→解密还原原文；密文中无明文。
func TestEncryptDecryptRoundTrip(t *testing.T) {
	plain := []byte(`{"version":1,"sites":[{"cookie":"secret-cookie"}]}`)
	enc, err := Encrypt(plain, "口令123!@#")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(enc, []byte("secret-cookie")) {
		t.Fatal("ciphertext contains plaintext")
	}
	if !NeedsPassword(enc) {
		t.Fatal("NeedsPassword should detect envelope")
	}
	got, err := Decrypt(enc, "口令123!@#")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("roundtrip mismatch:\n%s", got)
	}
}

// TestDecryptWrongPassword 错口令 → ErrBadPassword。
func TestDecryptWrongPassword(t *testing.T) {
	enc, err := Encrypt([]byte(`{"a":1}`), "right")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(enc, "wrong"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("want ErrBadPassword, got %v", err)
	}
}

// 坏 IV 长度过去直接 panic（gcm.Open 对非 12 字节 nonce 会 panic），
// Gin 兜底成 500 空响应；它也不是「口令错误」，必须按「文件损坏」归类。
func TestDecryptMalformedIVIsCorruptNotPassword(t *testing.T) {
	enc, err := Encrypt([]byte(`{"a":1}`), "pw")
	if err != nil {
		t.Fatal(err)
	}
	var env backupEncEnvelope
	if err := json.Unmarshal(enc, &env); err != nil {
		t.Fatal(err)
	}
	env.IV = base64.StdEncoding.EncodeToString([]byte("abc"))
	blob, _ := json.Marshal(env)
	if _, err := Decrypt(blob, "pw"); err == nil {
		t.Fatal("坏 IV 应报错（且不能 panic）")
	} else if errors.Is(err, ErrBadPassword) {
		t.Fatalf("坏 IV 是文件损坏，不该判成口令错误: %v", err)
	}
	// 正控：原封包仍可正常解密
	if _, err := Decrypt(enc, "pw"); err != nil {
		t.Fatalf("原文应可解密: %v", err)
	}
}

// 密文短于 GCM 认证标签：无论口令是什么都不可能解出 → 文件损坏。
func TestDecryptShortCiphertextIsCorrupt(t *testing.T) {
	enc, err := Encrypt([]byte(`{"a":1}`), "pw")
	if err != nil {
		t.Fatal(err)
	}
	var env backupEncEnvelope
	_ = json.Unmarshal(enc, &env)
	env.Data = base64.StdEncoding.EncodeToString([]byte("abcd"))
	blob, _ := json.Marshal(env)
	if _, err := Decrypt(blob, "pw"); err == nil || errors.Is(err, ErrBadPassword) {
		t.Fatalf("过短密文应归文件损坏，got %v", err)
	}
}

// TestDecryptPlaintextPassthrough 旧明文备份原样透传（导入兼容）。
func TestDecryptPlaintextPassthrough(t *testing.T) {
	plain := []byte(`{"version":1,"sites":[]}`)
	got, err := Decrypt(plain, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("plaintext backup should pass through unchanged")
	}
	if NeedsPassword(plain) {
		t.Fatal("plaintext should not require password")
	}
}

// TestDecryptNoPassword 加密备份缺口令 → ErrPasswordRequired。
func TestDecryptNoPassword(t *testing.T) {
	enc, _ := Encrypt([]byte(`{"a":1}`), "pw")
	if _, err := Decrypt(enc, ""); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("want ErrPasswordRequired, got %v", err)
	}
	var env map[string]any
	_ = json.Unmarshal(enc, &env)
	if env["ptpatronus_backup_enc"] == nil {
		t.Fatal("envelope magic missing")
	}
}
