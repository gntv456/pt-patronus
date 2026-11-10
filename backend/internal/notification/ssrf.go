// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package notification

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// notifyResolveTimeout 限制单次 DNS 解析耗时：net.DefaultResolver.LookupIPAddr 走系统
// resolver 且无有界超时，上游 DNS 不通时会挂到系统默认的数十秒，整条 Notify 链被一个
// 配错的 webhook host 拖死（manager 并发扇出也救不回——wg.Wait 仍要等最慢通道）。
// 这里施加有界超时，与 httpclient.resolveTimeout 对齐。
const notifyResolveTimeout = 8 * time.Second

// allowPrivateTargets 与站点抓取共用同一个产品开关（ALLOW_PRIVATE_SITES）。
// 从前通知链自己抄了一份守卫判定，只认 loopback 或公网，于是
// 「ntfy/Apprise 装在同一台 NAS 的局域网」「compose 里的兄弟服务（172.17.x）」
// 这两种最常见的自托管布置被永久拦死，而界面给的提示并不提这个开关存在。
// 元数据/链路本地地址不受此开关影响，恒拒。
var allowPrivateTargets atomic.Bool

// SetAllowPrivateTargets 由 app 装配阶段按 cfg.AllowPrivateSites 设定（默认关）。
func SetAllowPrivateTargets(v bool) { allowPrivateTargets.Store(v) }

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
		return fmt.Errorf("%s: 通知地址的协议只支持 http/https（当前填的是 %q），请在「设置→通知」里核对完整 URL", channel, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%s: 通知地址里没有主机名，请在「设置→通知」里填完整 URL（形如 http://192.168.1.20:8000/topic）", channel)
	}
	resolveCtx, cancel := context.WithTimeout(ctx, notifyResolveTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil {
		return fmt.Errorf("%s: 主机 %s 解析失败，通知没有发出：%w", channel, host, err)
	}
	for _, ipa := range ips {
		if !notifyIPAllowed(ipa.IP) {
			return fmt.Errorf("%s: 通知地址 %s 是内网/链路本地地址，已被 SSRF 防护拦下——消息没有发出。若通知服务（ntfy/Apprise/Gotify）确实部署在本机局域网或容器网段，请设环境变量 ALLOW_PRIVATE_SITES=true 后重启程序；公网可达或 127.0.0.1 的填法不受影响", channel, ipa.IP)
		}
	}
	return nil
}

// notifyIPAllowed 放行 loopback（本机通知服务）与公网 IP；
// 开了 ALLOW_PRIVATE_SITES 的管理员另放局域网/容器网段，但链路本地与云元数据地址恒拒
// （那与「内网跑 ntfy」无关，是凭据外泄的口）。
func notifyIPAllowed(ip net.IP) bool {
	if ip.IsLoopback() || httpclient.IsPublicIP(ip) {
		return true
	}
	return allowPrivateTargets.Load() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast()
}
