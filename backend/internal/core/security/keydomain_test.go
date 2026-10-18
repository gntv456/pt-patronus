package security

// #406 JWT 签名密钥与 AES-at-rest / 直链签名三处共用同一段原始 SECRET_KEY 字节。
//
// 旧形状：
//   NewJWT(secret)                        → secret = []byte(cfg.SecretKey)
//   NewCrypto(secret)                     → key   = HKDF(secret, "aes-at-rest:v1")
//   SetDirectLinkSignKey([]byte(SecretKey)) → 直接原始字节
//
// 而 crypto.go 里 hkdfInfoAES 上面那句注释写着「与 JWT 签名密钥同源不同派生，
// 轮换/泄漏其中一个不再波及另一个」—— 实际 JWT 侧压根没派生，这句注释在描述
// 一个不存在的隔离。
//
// 风险：拿到 AES 侧密钥的路径（备份文件/内存）与能验 JWT 签名的路径重合，
// 域分隔本该把两者隔开；直链签名更糟——URL 会写进 .strm、被播放器读出去。

import (
	"bytes"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

const seed = "shared-master-seed-for-406"

// TestDerivedKeysAreDistinct 三处派生出来的密钥必须两两不同。
func TestDerivedKeysAreDistinct(t *testing.T) {
	aes := DeriveKey(seed, hkdfInfoAES)
	jwt := DeriveKey(seed, HKDFInfoJWT())
	direct := DeriveKey(seed, hkdfInfoDirectLink)

	if bytes.Equal(aes, jwt) {
		t.Fatal("AES 与 JWT 派生出了同一把密钥 —— 域分隔没生效（#406）")
	}
	if bytes.Equal(aes, direct) {
		t.Fatal("AES 与直链派生出了同一把密钥（#406）")
	}
	if bytes.Equal(jwt, direct) {
		t.Fatal("JWT 与直链派生出了同一把密钥（#406）")
	}
	// 三把都必须与原始字节不同（否则等于没派生）。
	if bytes.Equal(jwt, []byte(seed)) {
		t.Fatal("JWT 密钥仍等于原始 SECRET_KEY 字节（#406）")
	}
}

// TestJWTSigningKeyIsDomainSeparated 钉住 NewJWT 真的走了派生。
//
// 只测 DeriveKey 是不够的：那条只证明 HKDF 能区分域，证明不了 JWT 用的是它。
// 此前 JWT 侧压根没派生，而 crypto.go 的注释一直假装有 —— 这次把断言钉在
// JWT 自己的签发密钥上。
func TestJWTSigningKeyIsDomainSeparated(t *testing.T) {
	j := NewJWT(seed, 60)

	if bytes.Equal(j.secret, []byte(seed)) {
		t.Fatal("JWT secret 仍是原始 SECRET_KEY 字节，#406 域分隔未生效")
	}
	if bytes.Equal(j.secret, DeriveKey(seed, hkdfInfoAES)) {
		t.Fatal("JWT 与 AES-at-rest 仍是同一把密钥，域分隔未生效")
	}
	if bytes.Equal(j.secret, DeriveKey(seed, hkdfInfoDirectLink)) {
		t.Fatal("JWT 与直链签名仍是同一把密钥，域分隔未生效")
	}
}

// TestJWTUpgradeKeepsOldTokensValid 升级不能变成全站登出。
//
// 存量 token 是用旧的原始 SECRET_KEY 字节签的；改成派生后验签要同时接受两把，
// 否则升级当天所有人被踢出登录（而这个「安全改进」没有任何用户可见提示）。
func TestJWTUpgradeKeepsOldTokensValid(t *testing.T) {
	j := NewJWT(seed, 60)

	// 1) 新签发的：能验过。
	tok, err := j.Issue(7, "alice", false, 1, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Parse(tok); err != nil {
		t.Fatalf("新签发的 token 必须能验过：%v", err)
	}

	// 2) 存量 token：用升级前的密钥（原始字节）签一份，必须仍能验过。
	old := signedToken(t, []byte(seed))
	if _, err := j.Parse(old); err != nil {
		t.Fatalf("升级前的存量 token 必须仍能验过（否则升级即全站登出）：%v", err)
	}

	// 3) 别的密钥签的：仍必须拒（兼容层不许变成万能钥匙）。
	other := signedToken(t, []byte("another-seed"))
	if _, err := j.Parse(other); err == nil {
		t.Fatal("别的密钥签的 token 必须被拒（兼容层只认新旧两把，不是万能钥匙）")
	}
}

// TestJWTLegacySecretOnlyForVerification 兼容钥只准验、不准签。
//
// 若签发也用 legacySecret，新 token 依旧与 AES-at-rest 同源，这次改造等于没做。
func TestJWTLegacySecretOnlyForVerification(t *testing.T) {
	j := NewJWT(seed, 60)

	if !bytes.Equal(j.legacySecret, []byte(seed)) {
		t.Fatal("legacySecret 应是升级前的原始 SECRET_KEY 字节（存量 token 要靠它验）")
	}
	if bytes.Equal(j.legacySecret, j.secret) {
		t.Fatal("legacySecret 与 secret 相同 ⇒ 派生没生效")
	}

	// 拿 legacySecret 签出来的 token 能验过（证明兼容层真的在用）。
	if _, err := j.Parse(signedToken(t, j.legacySecret)); err != nil {
		t.Fatalf("legacySecret 签的 token 必须仍能验过：%v", err)
	}
	// 而新签发的必须不是 legacy 签的形状。
	fresh, err := j.Issue(7, "alice", false, 1, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	probe := &JWT{secret: j.legacySecret, expires: j.expires}
	if _, err := probe.Parse(fresh); err == nil {
		t.Fatal("新签发的 token 能被 legacySecret 验过 ⇒ 签发仍在用兼容钥，域分隔形同虚设")
	}
}

// signedToken 用给定密钥签一份 HS256 token（模拟「升级前签发」/「别的密钥」）。
func signedToken(t *testing.T, key []byte) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		UserID:   7,
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
