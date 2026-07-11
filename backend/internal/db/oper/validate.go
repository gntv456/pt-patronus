package oper

import (
	"fmt"
	"net/url"
	"strings"
)

// validateURLScheme 仅允许 http/https，拒绝 file://、gopher://、ftp://、javascript: 等
// 可被用于 SSRF 或协议滥用的 scheme。用于用户可控的出站目标 URL（网盘 / 下载器）。
func validateURLScheme(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	s := strings.ToLower(u.Scheme)
	if s != "http" && s != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("url must have a host")
	}
	return nil
}

// validateForType 按存储类型分派 URL 校验：webdav/alist 走 http(s)；smb 走 smb://；agent/local 无 URL（agent 靠 agent_id 关联，local 靠本机根目录）。
func validateForType(typ, raw string) error {
	switch typ {
	case "smb":
		return validateSMBURL(raw)
	case "agent", "local":
		return nil
	default: // webdav | alist
		return validateURLScheme(raw)
	}
}

// validateSMBURL 校验 smb://host[:port]。
func validateSMBURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid smb url: %w", err)
	}
	if strings.ToLower(u.Scheme) != "smb" {
		return fmt.Errorf("smb url scheme must be smb, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("smb url must have a host")
	}
	return nil
}
