package site

import (
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// RegisteredDomain 提取 URL 或裸 host 的注册域（eTLD+1，publicsuffix）。
// 比「取最后两段」更准：正确处理 .co.uk / .com.cn 等多段公共后缀。无法判定时回退小写裸 host。
//
// 作为站点唯一性 key：同注册域视为同一站点——用于添加/导入去重（对齐 MoviePilot 按 domain
// 去重），无论 name / 子域 / path / trailing-slash 写法如何，同站只能有一条记录。
func RegisteredDomain(rawURLOrHost string) string {
	h := strings.TrimSpace(rawURLOrHost)
	if strings.Contains(h, "://") {
		if u, err := url.Parse(h); err == nil && u.Host != "" {
			h = u.Host
		}
	}
	h = strings.TrimPrefix(strings.ToLower(h), ".")
	if d, err := publicsuffix.EffectiveTLDPlusOne(h); err == nil {
		return d
	}
	return h
}
