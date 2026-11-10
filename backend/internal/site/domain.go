// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// RegisteredDomain 提取 URL 或裸 host 的注册域（eTLD+1，publicsuffix）。
// 比「取最后两段」更准：正确处理 .co.uk / .com.cn 等多段公共后缀。无法判定时回退小写裸 host。
// 带端口写法（hxpt.org:8080）先剥端口，否则 EffectiveTLDPlusOne 原样带端口返回，
// 同域判定（去重/同站归并）会被端口变体绕过。
func RegisteredDomain(rawURLOrHost string) string {
	h := strings.TrimSpace(rawURLOrHost)
	if strings.Contains(h, "://") {
		if u, err := url.Parse(h); err == nil && u.Host != "" {
			h = u.Host
		}
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	h = strings.TrimPrefix(strings.ToLower(h), ".")
	if d, err := publicsuffix.EffectiveTLDPlusOne(h); err == nil {
		return d
	}
	return h
}
