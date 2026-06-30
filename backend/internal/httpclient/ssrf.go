package httpclient

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// SSRF 全谱防护：阻止抓取目标解析到内网/环回/链路本地/元数据/CGNAT 等地址。
// 仅用于「站点抓取」类外部请求；下载器（qB/TR 通常在内网）应关闭此防护。

// IsPublicIP 报告该 IP 是否为公网地址。
func IsPublicIP(ip net.IP) bool {
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		// CGNAT 100.64.0.0/10
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return false
		}
	}
	return true
}

// AssertPublicURL 校验 URL scheme 合法且解析到的 IP 全部为公网。
// DNS 解析失败也视为不安全（拒绝）。
func AssertPublicURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("scheme not allowed: %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host")
	}
	// 先按字面 IP 判定（可能是整数 IP / 直接 IP）
	if literal := net.ParseIP(host); literal != nil && !IsPublicIP(literal) {
		return fmt.Errorf("blocked private/internal address %s", host)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if !IsPublicIP(ip) {
			return fmt.Errorf("blocked private/internal address %s for %s", ip, host)
		}
	}
	return nil
}
