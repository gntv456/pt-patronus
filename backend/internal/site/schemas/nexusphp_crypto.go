// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// 本文件实现好学挑战响应认证（use_challenge_response_authentication）所需的哈希原语，
// 逐字节对齐好学客户端 JS（public/js/common.js 的 sha256/hmacSha256，基于 CryptoJS）
// 与服务端 PHP（include/takelogin.php 的 hash()/hash_hmac()）。
//
// 算法（render_password_challenge_js + takelogin.php）：
//
//	h1       = sha256hex(password)
//	passhash = sha256hex(secret + h1)              // 字符串拼接；= 用户 passhash
//	response = hmac_sha256hex(key=challenge, msg=passhash)
//
// 服务端 expectedResponse = hash_hmac('sha256', passhash, challenge)，与此完全等价。

// sha256Hex 返回 sha256(message) 的小写十六进制。
// 对齐 JS：CryptoJS.SHA256(message).toString(CryptoJS.enc.Hex)
// 对齐 PHP：hash('sha256', message)
func sha256Hex(message string) string {
	sum := sha256.Sum256([]byte(message))
	return hex.EncodeToString(sum[:])
}

// hmacSHA256Hex 返回 HMAC-SHA256(key, message) 的小写十六进制。
// 对齐 JS：hmacSha256(key, message) = CryptoJS.HmacSHA256(message, key).toString(Hex)
// 对齐 PHP：hash_hmac('sha256', message, key)   // PHP: data=message, key=key
func hmacSHA256Hex(key, message string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// computeChallengeResponse 复刻好学登录页 JS 的挑战响应：
// h1=sha256(password) → passhash=sha256(secret+h1) → response=HMAC(key=challenge,msg=passhash)。
// secret/challenge 来自 POST /api/challenge {username}。
func computeChallengeResponse(secret, challenge, password string) string {
	h1 := sha256Hex(password)
	passhash := sha256Hex(secret + h1)
	return hmacSHA256Hex(challenge, passhash)
}
