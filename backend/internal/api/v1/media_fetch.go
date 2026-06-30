package v1

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	utls "github.com/refraction-networking/utls"
)

// mediaClient 反代媒体图片用的 HTTP 客户端：用 uTLS 伪装 Chrome 的 ClientHello，
// 绕过豆瓣图片 CDN（*.doubanio.com）按 TLS 指纹（JA3）的反爬——标准 net/http 会被
// 返回一段反爬 JS 挑战页（HTTP 200 + text/html），拿不到真图。
//
// ALPN 仅 http/1.1：net/http 不能在自管 TLS 连接上说 h2，故强制 H1（豆瓣图走 H1 正常）。
// SSRF 防护：拨号前解析 host → IP，校验公网后直连该 IP（复用 httpclient.IsPublicIP），
// 消除「Image 的 hostname 白名单校验」与「实际拨号」之间的 DNS 重绑定窗口。
var mediaClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		ForceAttemptHTTP2: false,
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, _ := net.SplitHostPort(addr)
			// SSRF：解析 host→IP，挑一个公网 IP 直连（防内网探测 + DNS 重绑定）。
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			var dialIP net.IP
			for _, ip := range ips {
				if httpclient.IsPublicIP(ip.IP) {
					dialIP = ip.IP
					break
				}
			}
			if dialIP == nil {
				return nil, fmt.Errorf("blocked private/internal address %s", host)
			}
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			raw, err := dialer.DialContext(ctx, network, net.JoinHostPort(dialIP.String(), port))
			if err != nil {
				return nil, err
			}
			uconn := utls.UClient(raw, &utls.Config{ServerName: host, NextProtos: []string{"http/1.1"}}, utls.HelloChrome_Auto)
			if err := uconn.HandshakeContext(ctx); err != nil {
				_ = raw.Close()
				return nil, err
			}
			return uconn, nil
		},
	},
}
