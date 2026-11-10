// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrInvalidInput 标记客户端输入校验错误（URL scheme 非法、缺 host 等），handler 据此返 400
// 而非 500（区分"用户输入错"与"服务器内部故障"）。
var ErrInvalidInput = errors.New("invalid input")

// validateURLScheme 仅允许 http/https，拒绝 file://、gopher://、ftp://、javascript: 等
// 可被用于 SSRF 或协议滥用的 scheme。用于用户可控的出站目标 URL（网盘 / 下载器）。
// invalidInputError 输入校验错误的载体：文案是写给人看的原因，
// Unwrap 到 ErrInvalidInput 让上层仍能 errors.Is 判定为 400。
// 不再用 fmt.Errorf("…: %w", ErrInvalidInput)——那会把哨兵的英文串
// ("invalid input") 拼进文案，而这些文案现在会直接进用户界面。
type invalidInputError struct{ msg string }

func (e *invalidInputError) Error() string { return e.msg }
func (e *invalidInputError) Unwrap() error { return ErrInvalidInput }

func invalidInput(format string, args ...any) error {
	return &invalidInputError{msg: fmt.Sprintf(format, args...)}
}

// InputReasonf 给 oper 之外的包（如 filter）造一条「写给人看的输入原因」。
// 载体仍是不导出的 invalidInputError：InputReasonErr 只认这一种，
// 于是分类层既能判 400，又不会把裸哨兵的英文串（"invalid input"）送进界面。
func InputReasonf(format string, args ...any) error {
	return invalidInput(format, args...)
}

// InputReasonErr 取出错误链上「我们写给人看的校验原因」（没有则 nil）。
// 裸哨兵 ErrInvalidInput 的 "invalid input" 只是分类标记， handler 据此判 400 可以，
// 原样进界面就成了用户读不懂的英文——所以只认 invalidInputError 这一种载体。
func InputReasonErr(err error) error {
	var ie *invalidInputError
	if errors.As(err, &ie) {
		return ie
	}
	return nil
}

func validateURLScheme(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return invalidInput("地址无法解析，请检查是否含空格或非法字符")
	}
	s := strings.ToLower(u.Scheme)
	if s != "http" && s != "https" {
		return invalidInput("地址必须是 http/https 协议（当前为 %q）", u.Scheme)
	}
	if u.Host == "" {
		return invalidInput("地址缺少主机名（形如 http://host:端口）")
	}
	return nil
}

// validateForType 按存储类型分派 URL 校验：webdav/alist 走 http(s)；smb 走 smb://；
// agent/local 无 URL；115/夸克/123 网盘为固定公网端点（cookie/token 鉴权），无自定义 URL；
// rclone 的 URL 字段承载 rclone 二进制路径（如 /usr/bin/rclone，可空），非网络地址不校验 scheme。
func validateForType(typ, raw string) error {
	switch typ {
	case "smb":
		return validateSMBURL(raw)
	case "agent", "local", "115", "quark", "123", "rclone":
		return nil
	default: // webdav | alist
		return validateURLScheme(raw)
	}
}

// validateSMBURL 校验 smb://host[:port]。
func validateSMBURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return invalidInput("SMB 地址无法解析，请检查是否含空格或非法字符")
	}
	if strings.ToLower(u.Scheme) != "smb" {
		return invalidInput("SMB 地址必须是 smb 协议（当前为 %q）", u.Scheme)
	}
	if u.Host == "" {
		return invalidInput("SMB 地址缺少主机名")
	}
	return nil
}
