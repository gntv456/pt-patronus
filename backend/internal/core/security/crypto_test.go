// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package security

import "testing"

func TestCryptoRoundTrip(t *testing.T) {
	c := NewCrypto("my-seed")
	cases := []string{
		"",
		"hello",
		"cookie=session=abc; path=/; HttpOnly",
		"中文 unicode 🔐",
	}
	for _, plain := range cases {
		enc, err := c.Encrypt(plain)
		if err != nil {
			t.Fatalf("encrypt %q: %v", plain, err)
		}
		if plain == "" {
			if enc != "" {
				t.Errorf("empty input should yield empty output, got %q", enc)
			}
			continue
		}
		dec, err := c.Decrypt(enc)
		if err != nil {
			t.Fatalf("decrypt %q: %v", plain, err)
		}
		if dec != plain {
			t.Errorf("roundtrip mismatch: got %q want %q", dec, plain)
		}
	}
}

func TestCryptoWrongKey(t *testing.T) {
	c1 := NewCrypto("seed1")
	enc, _ := c1.Encrypt("secret")
	if _, err := NewCrypto("seed2").Decrypt(enc); err == nil {
		t.Error("decrypt with wrong key should fail")
	}
}

func TestCryptoTamper(t *testing.T) {
	c := NewCrypto("seed")
	enc, _ := c.Encrypt("secret")
	if _, err := c.Decrypt(enc + "tampered"); err == nil {
		t.Error("tampered ciphertext should fail")
	}
}

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("s3cret!")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "s3cret!") {
		t.Error("correct password should verify")
	}
	if VerifyPassword(hash, "wrong") {
		t.Error("wrong password should not verify")
	}
	if VerifyPassword(hash, "") {
		t.Error("empty password should not verify")
	}
}
