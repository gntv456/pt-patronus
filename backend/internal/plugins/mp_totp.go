// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

type mpTOTPAccount struct {
	Name   string
	Secret string
	URL    string
}

func mpParseTOTPAccounts(raw string) ([]mpTOTPAccount, error) {
	var out []mpTOTPAccount
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			return nil, fmt.Errorf("账号配置格式错误：%s", line)
		}
		account := mpTOTPAccount{Name: strings.TrimSpace(parts[0]), Secret: strings.TrimSpace(parts[1])}
		if len(parts) > 2 {
			account.URL = strings.TrimSpace(parts[2])
		}
		if account.Name == "" || account.Secret == "" {
			return nil, fmt.Errorf("账号配置无效：%s", line)
		}
		out = append(out, account)
	}
	if len(out) == 0 {
		return nil, errors.New("没有有效账号配置")
	}
	return out, nil
}

func mpTOTPCode(secret string, at time.Time, digits, period int) (string, int, error) {
	if digits <= 0 {
		digits = 6
	}
	if digits > 10 {
		digits = 10
	}
	if period <= 0 {
		period = 30
	}
	key, err := mpDecodeTOTPSecret(secret)
	if err != nil {
		return "", 0, err
	}
	counter := uint64(at.Unix() / int64(period))
	code := mpHOTP(key, counter, digits)
	remaining := period - int(at.Unix()%int64(period))
	return code, remaining, nil
}

func mpCleanTOTPSecret(secret string) string {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secret = strings.NewReplacer(" ", "", "-", "", "=", "").Replace(secret)
	return secret
}

func mpDecodeTOTPSecret(secret string) ([]byte, error) {
	secret = mpCleanTOTPSecret(secret)
	if secret == "" {
		return nil, errors.New("TOTP 密钥不能为空")
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
}

func mpHOTP(key []byte, counter uint64, digits int) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, bin%mod)
}
