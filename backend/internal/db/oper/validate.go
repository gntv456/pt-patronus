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
func validateURLScheme(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url: %w: %w", err, ErrInvalidInput)
	}
	s := strings.ToLower(u.Scheme)
	if s != "http" && s != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q: %w", u.Scheme, ErrInvalidInput)
	}
	if u.Host == "" {
		return fmt.Errorf("url must have a host: %w", ErrInvalidInput)
	}
	return nil
}

// validateForType 按存储类型分派 URL 校验：webdav/alist 走 http(s)；smb 走 smb://；
// agent/local 无 URL；115/夸克/123 网盘为固定公网端点（cookie/token 鉴权），无自定义 URL。
func validateForType(typ, raw string) error {
	switch typ {
	case "smb":
		return validateSMBURL(raw)
	case "agent", "local", "115", "quark", "123":
		return nil
	default: // webdav | alist
		return validateURLScheme(raw)
	}
}

// validateSMBURL 校验 smb://host[:port]。
func validateSMBURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid smb url: %w: %w", err, ErrInvalidInput)
	}
	if strings.ToLower(u.Scheme) != "smb" {
		return fmt.Errorf("smb url scheme must be smb, got %q: %w", u.Scheme, ErrInvalidInput)
	}
	if u.Host == "" {
		return fmt.Errorf("smb url must have a host: %w", ErrInvalidInput)
	}
	return nil
}
