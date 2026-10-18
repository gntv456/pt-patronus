// Package security provides crypto primitives: AES-GCM secret encryption and bcrypt hashing.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"

	"golang.org/x/crypto/bcrypt"
)

// Crypto provides AES-GCM symmetric encryption for secrets at rest
// (cookie / apikey / passkey / downloader passwords).
type Crypto struct {
	key       []byte // 32 bytes，HKDF 域分隔派生（当前）
	legacyKey []byte // 32 bytes，旧 SHA-256 直派生（仅 Decrypt 兼容回退用）
}

// hkdfInfoAES 静态加密密钥的 HKDF info（域分隔）：与 JWT 签名密钥同源不同派生，
// 轮换/泄漏其中一个不再波及另一个（安全审计 P1-1）。
const hkdfInfoAES = "ptpatronus:aes-at-rest:v1"

// NewCrypto derives a 32-byte AES key from the given seed via HKDF-SHA256
// （域分隔 info，与 JWT 签名密钥同源不同派生）。legacyKey 保留旧 SHA-256 直派生
// 供 Decrypt 回退解历史密文。
func NewCrypto(seed string) *Crypto {
	sum := sha256.Sum256([]byte(seed))
	return &Crypto{key: deriveKey(seed, hkdfInfoAES), legacyKey: sum[:]}
}

// hkdfInfoJWT JWT 签名密钥的 HKDF info（域分隔）。
//
// #406 补记：hkdfInfoAES 上面那句「与 JWT 签名密钥同源不同派生」此前**只在 AES 侧成立** ——
// Crypto 派生过，而 NewJWT 直接吃 []byte(SECRET_KEY)。也就是说 JWT 签名密钥与
// AES-at-rest 密钥的派生方式完全不同、却没有真正隔离。现在 JWT 侧也走 HKDF（信息串不同），
// 两者才是真的「同源不同派生」。
const hkdfInfoJWT = "ptpatronus:jwt-hs256:v1"

// hkdfInfoDirectLink 云盘直链签名密钥（302 公开端点即凭证，同样要隔离）。
//
// 这条链路此前是 SetDirectLinkSignKey([]byte(cfg.SecretKey)) —— 第三个直接吃
// SECRET_KEY 原始字节的地方。直链 URL 会出现在 .strm 文件里、能被播放器读出去，
// 所以它的密钥泄漏面比 JWT 更宽，更不该与 AES/JWT 共用同一段原始密钥。
const hkdfInfoDirectLink = "ptpatronus:directlink-hmac:v1"

// DeriveKey 暴露按域分隔派生 32 字节密钥的口子。
//
// 给 JWT 签名与直链 HMAC 用（见 #406）：它们此前都直接吃 []byte(cfg.SecretKey)，
// 与 AES-at-rest 没有任何派生隔离。deriveKey 本来是包内私有的，加这个导出口是为了
// 让「派生规则集中在 crypto.go 一处」——新增用途时必须给新的 info 串，不许复用既有的。
//
// 注意：改了 info 串等于换了密钥 ⇒ 已签发的 JWT / 已生成的 .strm 直链会失效。
// 所以 info 串带 vN 版本后缀，改算法时升版本号，别就地改。
func DeriveKey(seed, info string) []byte { return deriveKey(seed, info) }

// HKDFInfoJWT / HKDFInfoDirectLink 供 app 装配时用（避免信息串散落两处写错）。
func HKDFInfoJWT() string        { return hkdfInfoJWT }
func HKDFInfoDirectLink() string { return hkdfInfoDirectLink }

// deriveKeyHKDF 用 HKDF-SHA256 从 seed 派生 32 字节密钥。
func deriveKey(seed, info string) []byte {
	prk := hkdfExtract(seed)
	okm := hkdfExpand(prk, []byte(info), 32)
	return okm
}

// hkdfExtract HKDF-Extract(salt=nil, ikm=seed)：HMAC-SHA256(nil-key, seed)。
func hkdfExtract(seed string) []byte {
	mac := hmac.New(sha256.New, nil)
	mac.Write([]byte(seed))
	return mac.Sum(nil)
}

// hkdfExpand HKDF-Expand 单块（N<=255*HashLen，32 字节只需 1 块）。
func hkdfExpand(prk, info []byte, length int) []byte {
	mac := hmac.New(sha256.New, prk)
	mac.Write(info)
	mac.Write([]byte{1})
	return mac.Sum(nil)[:length]
}

// Encrypt encrypts plaintext with AES-GCM and returns a base64 string (nonce||ciphertext).
// Empty input yields empty output (no record stored).
func (c *Crypto) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plain), nil)
	out := append(nonce, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt decrypts a base64 AES-GCM ciphertext produced by Encrypt.
// 兼容回退：HKDF 新派生解不开时，用旧派生（SHA-256 直取）重试一次，
// 以解开升级前落库的存量密文（安全审计 P1-1 域分隔改造的平滑迁移）。
func (c *Crypto) Decrypt(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	pt, err := gcmOpen(c.key, raw)
	if err != nil {
		if pt2, err2 := gcmOpen(c.legacyKey, raw); err2 == nil {
			return string(pt2), nil
		}
		return "", err
	}
	return string(pt), nil
}

// gcmOpen 用给定 key 做 AES-GCM open。
func gcmOpen(key, raw []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns+1 {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, raw[:ns], raw[ns:], nil)
}

// HashPassword bcrypt-hashes a password.
func HashPassword(pwd string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	return string(b), err
}

// VerifyPassword reports whether the password matches the bcrypt hash.
func VerifyPassword(hash, pwd string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pwd)) == nil
}
