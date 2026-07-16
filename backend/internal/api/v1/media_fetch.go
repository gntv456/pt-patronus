package v1

import (
	"net/http"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// mediaClient 反代媒体图片用的 HTTP 客户端：用 uTLS 伪装 Chrome 的 ClientHello，
// 绕过豆瓣图片 CDN（*.doubanio.com）按 TLS 指纹（JA3）的反爬——标准 net/http 会被
// 返回一段反爬 JS 挑战页（HTTP 200 + text/html），拿不到真图。
//
// ALPN 仅 http/1.1：net/http 不能在自管 TLS 连接上说 h2，故强制 H1（豆瓣图走 H1 正常）。
// SSRF 防护：拨号前解析 host → IP，校验公网后直连该 IP（复用 httpclient.IsPublicIP），
// 消除「Image 的 hostname 白名单校验」与「实际拨号」之间的 DNS 重绑定窗口。
var mediaClient = newMediaClient(false, false)

func newMediaClient(allowPrivate, insecureTLS bool) *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			ForceAttemptHTTP2: false,
			DialTLSContext:    httpclient.UTLSDialContext(allowPrivate, insecureTLS),
		},
	}
}
