package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload.
type Claims struct {
	UserID        uint   `json:"uid"`
	Username      string `json:"usr"`
	IsAdmin       bool   `json:"adm"`
	TokenVersion  uint   `json:"tver,omitempty"` // 与 User.TokenVersion 比对，不符即视为已登出/失效
	VipLevel      int    `json:"vip,omitempty"`  // VIP 等级快照（Guard 快速判定）
	VipVerifiedAt int64  `json:"vvat,omitempty"` // VIP 校验时间 unix 秒（Guard 判宽限期）
	jwt.RegisteredClaims
}

// JWT issues and verifies HS256 tokens.
type JWT struct {
	secret  []byte
	expires time.Duration
}

// NewJWT builds a JWT helper with the given signing secret and expiry (minutes).
func NewJWT(secret string, expireMin int) *JWT {
	return &JWT{secret: []byte(secret), expires: time.Duration(expireMin) * time.Minute}
}

// Issue creates a signed token for the given user. tokenVersion 用于登出失效，
// vipLevel/vipVerifiedAt 为 VIP 快照（供 Guard 快速判定，真值以 User 表为准）。
func (j *JWT) Issue(userID uint, username string, isAdmin bool, tokenVersion uint, vipLevel int, vipVerifiedAt time.Time) (string, error) {
	claims := Claims{
		UserID:       userID,
		Username:     username,
		IsAdmin:      isAdmin,
		TokenVersion: tokenVersion,
		VipLevel:     vipLevel,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.expires)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	if !vipVerifiedAt.IsZero() {
		claims.VipVerifiedAt = vipVerifiedAt.Unix()
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(j.secret)
}

// Parse validates the token and returns its claims.
func (j *JWT) Parse(token string) (*Claims, error) {
	c := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return c, nil
}
