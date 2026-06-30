package security

import (
	"testing"
	"time"
)

func TestJWTRoundTrip(t *testing.T) {
	j := NewJWT("signing-secret", 60)
	tok, err := j.Issue(7, "alice", true, 0, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := j.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.UserID != 7 || c.Username != "alice" || !c.IsAdmin {
		t.Errorf("claims = %+v", c)
	}
}

func TestJWTWrongKey(t *testing.T) {
	tok, _ := NewJWT("key-a", 60).Issue(1, "u", false, 0, 0, time.Time{})
	if _, err := NewJWT("key-b", 60).Parse(tok); err == nil {
		t.Error("parse with wrong key should fail")
	}
}

func TestJWTTampered(t *testing.T) {
	j := NewJWT("k", 60)
	tok, _ := j.Issue(1, "u", false, 0, 0, time.Time{})
	if _, err := j.Parse(tok + "x"); err == nil {
		t.Error("tampered token should fail")
	}
}

func TestJWTExpired(t *testing.T) {
	// 过期：用负 expire 构造（绕过直接签发过期 token）
	j := NewJWT("k", -1)
	tok, err := j.Issue(1, "u", false, 0, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Parse(tok); err == nil {
		t.Error("expired token should fail")
	}
}
