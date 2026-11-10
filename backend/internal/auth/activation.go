// Package auth implements legacy activation codes (HMAC-signed) and session guards.
// 好学 VIP 准入已随站点关闭退役，激活码端点仅为历史兼容保留，不再作门禁。
// and the AUTH_SITE-style VIP gate. The HxptVIP scrape itself lands in phase 1
// (via internal/site adapter); this package provides the scaffolding + code flow.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// Activation manages APP-exclusive VIP activation codes.
type Activation struct {
	db     *gorm.DB
	secret string
}

// NewActivation builds an Activation manager. An empty secret falls back to a
// dev default (must be set via ACTIVATION_SECRET in production).
func NewActivation(db *gorm.DB, secret string) *Activation {
	if secret == "" {
		secret = "activation-insecure-secret"
	}
	return &Activation{db: db, secret: secret}
}

// Generate creates and persists a signed activation code.
// Format: <16 random hex>.<vipLevel>.<unix>.<hmac(16 hex)>.
func (a *Activation) Generate(vipLevel int, expiresAt *time.Time) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	body := fmt.Sprintf("%s.%d.%d", hex.EncodeToString(raw[:]), vipLevel, time.Now().Unix())
	code := body + "." + a.sign(body)
	rec := &models.ActivationCode{
		Code:      code,
		VipLevel:  vipLevel,
		ExpiresAt: expiresAt,
	}
	if err := a.db.Create(rec).Error; err != nil {
		return "", err
	}
	return code, nil
}

// Validate checks a code's signature & expiry without consuming it.
func (a *Activation) Validate(code string) error {
	parts := strings.Split(code, ".")
	if len(parts) != 4 {
		return errors.New("invalid code format")
	}
	body := parts[0] + "." + parts[1] + "." + parts[2]
	if !hmac.Equal([]byte(parts[3]), []byte(a.sign(body))) {
		return errors.New("invalid code signature")
	}
	return nil
}

// Activate consumes a code, binding it to a device, and returns the granted VIP level.
func (a *Activation) Activate(code, deviceID string) (int, error) {
	if err := a.Validate(code); err != nil {
		return 0, err
	}
	var rec models.ActivationCode
	if err := a.db.Where("code = ?", code).First(&rec).Error; err != nil {
		return 0, err
	}
	if rec.Revoked {
		return 0, errors.New("code revoked")
	}
	if rec.UsedAt != nil {
		return 0, errors.New("code already used")
	}
	if rec.ExpiresAt != nil && rec.ExpiresAt.Before(time.Now()) {
		return 0, errors.New("code expired")
	}
	now := time.Now()
	rec.UsedAt = &now
	if deviceID != "" {
		rec.DeviceID = deviceID
	}
	if err := a.db.Save(&rec).Error; err != nil {
		return 0, err
	}
	return rec.VipLevel, nil
}

// AnyActivated reports whether any valid activation exists (instance-level gate).
func (a *Activation) AnyActivated() bool {
	var count int64
	a.db.Model(&models.ActivationCode{}).
		Where("used_at IS NOT NULL AND revoked = ?", false).
		Count(&count)
	return count > 0
}

// IsActivated reports whether a specific device has an active code.
func (a *Activation) IsActivated(deviceID string) bool {
	if deviceID == "" {
		return false
	}
	var count int64
	a.db.Model(&models.ActivationCode{}).
		Where("device_id = ? AND used_at IS NOT NULL AND revoked = ?", deviceID, false).
		Count(&count)
	return count > 0
}

func (a *Activation) sign(body string) string {
	mac := hmac.New(sha256.New, []byte(a.secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}
