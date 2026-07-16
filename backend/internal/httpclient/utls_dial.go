package httpclient

import (
	"context"
	"fmt"
	"net"
	"time"

	utls "github.com/refraction-networking/utls"
)

// UTLSDialContext returns an HTTPS dialer using a Chrome-compatible ClientHello.
// 做 TLS 握手，绕过按 TLS 指纹（JA3/JA4）拦截非浏览器流量的 WAF/CDN——典型如 SafeLine（雷池）
// WAF 对签到端点 attendance.php 的拦截：标准 net/http 的指纹非浏览器，被返回 468 拦截页。
//
// 复用 media_fetch.go 的成熟模式：
//   - ALPN 仅 http/1.1（net/http 不能在自管 TLS 连接上说 h2，强制 H1）。
//   - SSRF：allowPrivate=false 时解析 host→公网 IP 直连（防内网探测 + DNS 重绑定），与
//     ssrfDialContext 同策略；allowPrivate=true（本地 httptest 测试）时直连原 addr。
//   - insecure=true 时跳过证书校验（仅自签 httptest 测试用；生产保持 false）。
func UTLSDialContext(allowPrivate, insecure bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		dialAddr := addr
		if !allowPrivate {
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			var dialIP net.IP
			for _, ip := range ips {
				if IsPublicIP(ip.IP) {
					dialIP = ip.IP
					break
				}
			}
			if dialIP == nil {
				return nil, fmt.Errorf("ssrf: blocked private/internal address %s", host)
			}
			dialAddr = net.JoinHostPort(dialIP.String(), port)
		}
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		raw, err := dialer.DialContext(ctx, network, dialAddr)
		if err != nil {
			return nil, err
		}
		cfg := &utls.Config{ServerName: host}
		if insecure {
			cfg.InsecureSkipVerify = true
		}
		uconn := utls.UClient(raw, cfg, utls.HelloCustom)
		spec, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
		if err != nil {
			_ = raw.Close()
			return nil, err
		}
		// 强制 ALPN 只 http/1.1：net/http 不能在自管 TLS 连接上说 h2——否则服务端（如 Cloudflare）
		// 协商 h2 后发 HTTP/2 SETTINGS 帧，H1 transport 报「malformed HTTP response」。
		// Chrome 指纹默认 ALPN 含 h2 优先，须显式覆写为只 http/1.1。
		for i := range spec.Extensions {
			if alpn, ok := spec.Extensions[i].(*utls.ALPNExtension); ok {
				alpn.AlpnProtocols = []string{"http/1.1"}
			}
		}
		if err := uconn.ApplyPreset(&spec); err != nil {
			_ = raw.Close()
			return nil, err
		}
		if err := uconn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, err
		}
		return uconn, nil
	}
}
