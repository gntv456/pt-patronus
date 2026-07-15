package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// SSRF 全谱防护：阻止抓取目标解析到内网/环回/链路本地/元数据/CGNAT 等地址。
// 仅用于「站点抓取」类外部请求；下载器（qB/TR 通常在内网）应关闭此防护。

// resolveTimeout 限制 AssertPublicURL 内单次 DNS 解析的耗时。
// net.LookupIP 走系统 resolver 且无 ctx 超时上限——容器内上游 DNS（Docker 的 127.0.0.11）
// 不通时，一次查询可能挂到系统默认的数十秒，表现为 "i/o timeout"。这里施加有界超时，
// 配合 [Client.assertPublicWithRetry] 的重试，把瞬时 DNS 抖动变成可恢复而非硬失败。
const resolveTimeout = 8 * time.Second

// ErrBlocked 表示一次确定的、不可重试的 SSRF 目标拒绝：目标解析到内网/环回/链路本地/
// 元数据/CGNAT 等私有地址，或 scheme 非法、主机为空。
//
// 与之相对的是 DNS 解析超时 / NXDOMAIN / 网络瞬时错误——这些是临时性的、可重试的，
// 因此 AssertPublicURL 返回它们时【不】包裹 ErrBlocked。调用方用 errors.Is(err, ErrBlocked)
// 区分：命中即立即拒绝、绝不重试；未命中即可重试（见 [Client.assertPublicWithRetry]）。
var ErrBlocked = errors.New("ssrf: blocked private/internal address")

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

// lookupIPFunc 解析主机名为 IP 列表，抽象出来便于测试注入（替换 net.DefaultResolver，
// 使分类测试不依赖真实 DNS）。
type lookupIPFunc func(ctx context.Context, host string) ([]net.IP, error)

// defaultLookupIP 是 AssertPublicURL 使用的真实解析器（net.DefaultResolver）。
// 超时由 assertPublicURL 内的 context.WithTimeout 统一施加，此处只做透传。
func defaultLookupIP(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

// AssertPublicURL 校验 URL scheme 合法且解析到的 IP 全部为公网。
// DNS 解析失败也视为不安全（拒绝），但与「解析到私有地址」严格区分：
//   - 解析到私有/内网地址、scheme 非法、空 host → 包裹 [ErrBlocked]（确定的拒绝，不可重试）；
//   - DNS 解析超时 / NXDOMAIN / 网络错误 → 不包裹 ErrBlocked（临时性，可重试）。
func AssertPublicURL(rawURL string) error {
	return assertPublicURL(rawURL, defaultLookupIP)
}

// assertPublicURL 是 AssertPublicURL 的可注入内核，测试用它替换 lookup 以免依赖真实 DNS。
func assertPublicURL(rawURL string, lookup lookupIPFunc) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("scheme not allowed: %q: %w", u.Scheme, ErrBlocked)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host: %w", ErrBlocked)
	}
	// 先按字面 IP 判定（可能是整数 IP / 直接 IP）
	if literal := net.ParseIP(host); literal != nil && !IsPublicIP(literal) {
		return fmt.Errorf("%w: %s", ErrBlocked, host)
	}
	resolveCtx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
	defer cancel()
	ips, err := lookup(resolveCtx, host)
	if err != nil {
		// 解析失败（超时/NXDOMAIN/网络）——临时性错误，不裹 ErrBlocked，留待调用方重试。
		return fmt.Errorf("resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if !IsPublicIP(ip) {
			return fmt.Errorf("%w %s for %s", ErrBlocked, ip, host)
		}
	}
	return nil
}
