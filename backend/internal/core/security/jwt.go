package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrSessionExpired 会话超过绝对寿命上限，须重新登录（非普通 token 过期）。
var ErrSessionExpired = errors.New("session exceeded absolute lifetime")

// Claims is the JWT payload.
type Claims struct {
	UserID        uint   `json:"uid"`
	Username      string `json:"usr"`
	IsAdmin       bool   `json:"adm"`
	TokenVersion  uint   `json:"tver,omitempty"` // 与 User.TokenVersion 比对，不符即视为已登出/失效
	VipLevel      int    `json:"vip,omitempty"`  // VIP 等级快照（Guard 快速判定）
	VipVerifiedAt int64  `json:"vvat,omitempty"` // 历史快照，闸门已退役
	FirstIssuedAt int64  `json:"fiat,omitempty"` // 会话首次签发时间（滑动续期不得突破绝对寿命上限）
	jwt.RegisteredClaims
}

// absoluteLifetime 滑动续期的绝对寿命上限（安全审计 P1-2：无上限的滑动续期
// 等于泄漏 token 永久有效）。7 天后强制重新走一次本地账号登录。
const absoluteLifetime = 7 * 24 * time.Hour

// JWT issues and verifies HS256 tokens.
type JWT struct {
	secret []byte
	// legacySecret 升级前的原始 SECRET_KEY 字节（#406）。只用于**验签**：
	// 存量 token 用旧密钥签的，改派生后必须仍能验过，否则升级即全站登出。
	// 签发一律用 secret（新派生），不再用 legacy —— 否则新 token 仍与 AES 同源。
	legacySecret []byte
	expires      time.Duration
}

// NewJWT builds a JWT helper with the given signing secret and expiry (minutes).
//
// #406 域分隔：此前是 []byte(secret) 直接当 HMAC 密钥，与 AES-at-rest 的 HKDF
// 派生出来的密钥毫无隔离 —— 而 aes-at-rest 那侧的注释一直写着「与 JWT 签名密钥
// 同源不同派生」，实际并没有。现在签名密钥也走 HKDF（不同的 info 串）。
//
// 兼容：验签同时接受旧的原始字节，所以升级不需要用户重新登录。
func NewJWT(secret string, expireMin int) *JWT {
	return &JWT{
		secret:       DeriveKey(secret, hkdfInfoJWT),
		legacySecret: []byte(secret),
		expires:      time.Duration(expireMin) * time.Minute,
	}
}

// Expires reports the token lifetime（登录 cookie MaxAge 与之对齐）。
func (j *JWT) Expires() time.Duration { return j.expires }

// Issue creates a signed token for the given user. tokenVersion 用于登出失效，
// vipLevel/vipVerifiedAt 为 VIP 快照（供 Guard 快速判定，真值以 User 表为准）。
// firstIssued 非 0 表示这是滑动续期：保留会话首次签发时间并钳制新过期时间
// 不超过 firstIssued+absoluteLifetime。
func (j *JWT) Issue(userID uint, username string, isAdmin bool, tokenVersion uint, vipLevel int, vipVerifiedAt time.Time) (string, error) {
	return j.issue(userID, username, isAdmin, tokenVersion, vipLevel, vipVerifiedAt, 0)
}

// IssueSliding 滑动续期签发：沿用旧 token 的首次签发时间，过期时间再延一个
// 完整窗口但不超过绝对寿命上限（到达上限返回错误，调用方应要求重新登录）。
func (j *JWT) IssueSliding(old *Claims, tokenVersion uint, vipLevel int, vipVerifiedAt time.Time) (string, error) {
	first := old.FirstIssuedAt
	if first == 0 {
		first = time.Now().Unix()
	}
	if time.Since(time.Unix(first, 0)) >= absoluteLifetime {
		return "", ErrSessionExpired
	}
	return j.issue(old.UserID, old.Username, old.IsAdmin, tokenVersion, vipLevel, vipVerifiedAt, first)
}

func (j *JWT) issue(userID uint, username string, isAdmin bool, tokenVersion uint, vipLevel int, vipVerifiedAt time.Time, firstIssued int64) (string, error) {
	now := time.Now()
	exp := now.Add(j.expires)
	if firstIssued != 0 {
		if capTime := time.Unix(firstIssued, 0).Add(absoluteLifetime); exp.After(capTime) {
			exp = capTime
		}
	}
	claims := Claims{
		UserID:       userID,
		Username:     username,
		IsAdmin:      isAdmin,
		TokenVersion: tokenVersion,
		VipLevel:     vipLevel,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	if firstIssued != 0 {
		claims.FirstIssuedAt = firstIssued
	}
	if !vipVerifiedAt.IsZero() {
		claims.VipVerifiedAt = vipVerifiedAt.Unix()
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(j.secret)
}

// Parse validates the token and returns its claims.
func (j *JWT) Parse(token string) (*Claims, error) {
	// #406 兼容：先用新派生密钥验（升级后签发的一律是它），失败再试旧的原始字节。
	// 两段都要，是因为存量 token 用旧密钥签的 —— 只认新密钥会让升级变成全站登出。
	// 两段密钥都验不过才返回错误；错误取最后一次的即可（签名不符时信息一致）。
	var lastErr error
	for _, key := range [][]byte{j.secret, j.legacySecret} {
		if len(key) == 0 {
			continue
		}
		c := &Claims{}
		parsed, err := jwt.ParseWithClaims(token, c, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return key, nil
		})
		if err != nil {
			lastErr = err
			continue
		}
		if !parsed.Valid {
			lastErr = errors.New("invalid token")
			continue
		}
		return c, nil
	}
	if lastErr == nil {
		lastErr = errors.New("invalid token")
	}
	return nil, lastErr
}
