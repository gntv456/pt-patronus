package backup

import (
	"bytes"
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
