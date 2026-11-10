// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package security

import "unicode"

// ValidatePassword 本地账号弱密码闸门：≥8 位且同时含字母与数字。
// 登录建号、改密、compose 引导共用这一份规则，避免口径漂移。
// 纯数字/纯字母/短口令在公网自托管实例上撑不住限流窗口内的爆破。
func ValidatePassword(pwd string) bool {
	if len(pwd) < 8 {
		return false
	}
	hasLetter, hasDigit := false, false
	for _, r := range pwd {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}
