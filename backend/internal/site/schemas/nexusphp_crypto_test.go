package schemas

import (
	"testing"
)

// 公开向量钉死原语正确性。

func TestSha256Hex(t *testing.T) {
	cases := map[string]string{
		"":    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"abc": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	}
	for in, want := range cases {
		if got := sha256Hex(in); got != want {
			t.Errorf("sha256Hex(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestHmacSHA256Hex(t *testing.T) {
	// Wikipedia 向量：HMAC-SHA256(key="key", msg="The quick brown fox jumps over the lazy dog")
	const want = "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"
	if got := hmacSHA256Hex("key", "The quick brown fox jumps over the lazy dog"); got != want {
		t.Errorf("hmacSHA256Hex = %s, want %s", got, want)
	}
}

// TestComputeChallengeFormula 钉死组合公式（参数顺序 + 拼接），与好学 JS/PHP 对齐：
//
//	h1=sha256(password); passhash=sha256(secret+h1); response=HMAC(key=challenge, msg=passhash)
//
// 这里用【显式重算】对照实现，若实现把 key/msg 写反或漏拼 h1，立即失败。
func TestComputeChallengeFormula(t *testing.T) {
	const secret, challenge, password = "s3cr3t", "ch4ll3nge", "p4ssw0rd"

	h1 := sha256Hex(password)
	passhash := sha256Hex(secret + h1)
	want := hmacSHA256Hex(challenge, passhash) // 正确顺序：key=challenge, msg=passhash

	got := computeChallengeResponse(secret, challenge, password)
	if got != want {
		t.Errorf("computeChallengeResponse = %s, want %s", got, want)
	}

	// 反向断言：key/msg 写反应得到不同结果（防止实现与期望同时写反）。
	if got == hmacSHA256Hex(passhash, challenge) {
		t.Error("response collides with swapped-arg HMAC — challenge/passhash may be mis-ordered")
	}
}
