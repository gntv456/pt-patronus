package notification

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// notifyResolveTimeout 限制单次 DNS 解析耗时：net.DefaultResolver.LookupIPAddr 走系统
// resolver 且无有界超时，上游 DNS 不通时会挂到系统默认的数十秒，整条 Notify 链被一个
// 配错的 webhook host 拖死（manager 并发扇出也救不回——wg.Wait 仍要等最慢通道）。
// 这里施加有界超时，与 httpclient.resolveTimeout 对齐。
const notifyResolveTimeout = 8 * time.Second

// assertNotifyURL 校验通知目标 URL：scheme 限 http/https、host 非空、解析到的 IP
// 仅允许 loopback（本地通知服务如 Apprise/ntfy 容器，合理用例）或公网，
// 拒绝其他内网/CGNAT/link-local/元数据地址（防管理员误配打到 169.254.169.254 等）。
// 通知 URL 由管理员配置（非公网访客输入），故比站点抓取宽松（放行 loopback）。
// 各 HTTP 通知渠道在 Send 开头调用；用请求 ctx 以便取消时中止 DNS 解析。
func assertNotifyURL(ctx context.Context, channel, rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("%s: parse url: %w", channel, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%s: scheme not allowed: %q", channel, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%s: empty host", channel)
	}
	resolveCtx, cancel := context.WithTimeout(ctx, notifyResolveTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil {
		return fmt.Errorf("%s: resolve %s: %w", channel, host, err)
	}
	for _, ipa := range ips {
		if !notifyIPAllowed(ipa.IP) {
			return fmt.Errorf("%s: blocked non-public/non-loopback address %s", channel, ipa.IP)
		}
	}
	return nil
}

// notifyIPAllowed 允许 loopback（本地通知服务）或公网 IP。
func notifyIPAllowed(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	return httpclient.IsPublicIP(ip)
}
