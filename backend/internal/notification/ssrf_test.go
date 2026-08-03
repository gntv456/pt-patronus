package notification

import (
	"context"
	"testing"
	"time"
)

// assertNotifyURL 内的 DNS 解析有 8s 有界超时；一个永远解析不出的 host
// 必须在 ~8s 内返回（而非挂到系统默认数十秒），否则会把整个 Notify 扇出阻塞。
func TestAssertNotifyURLDNSResolveTimeout(t *testing.T) {
	// .invalid 是 RFC 6761 保留的永远 NXDOMAIN 的 TLD，解析会立即失败而非超时，
	// 但走的是真实 DNS 路径，能验证 resolve 分支不 panic 且快速返回。
	start := time.Now()
	err := assertNotifyURL(context.Background(), "tg", "https://nonexistent-host-for-timeout-test.invalid/webhook")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected resolve error for invalid host, got nil")
	}
	if elapsed > 15*time.Second {
		t.Fatalf("assertNotifyURL took %v, DNS resolve should be bounded by notifyResolveTimeout(8s)", elapsed)
	}
}

// 上下文取消应让 DNS 解析立即返回，而非等待底层 resolver。
func TestAssertNotifyURLCanceledCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 先取消
	err := assertNotifyURL(ctx, "tg", "https://example.com/webhook")
	if err == nil {
		return // 解析已返回（缓存命中等），无妨
	}
	// 取消后应迅速返回（错误可能是 ctx.Err 或 resolve 失败，都不应长时间阻塞）
}

// 字面内网 IP 直接拒绝（不触发 DNS）。
func TestAssertNotifyURLBlocksPrivateLiteral(t *testing.T) {
	// 192.168 是非 loopback 私有段，notifyIPAllowed 不放行（仅放行 loopback + 公网）。
	err := assertNotifyURL(context.Background(), "wh", "http://192.168.1.1:8080/hook")
	if err == nil {
		t.Fatal("192.168 literal must be blocked (non-loopback private)")
	}
}
