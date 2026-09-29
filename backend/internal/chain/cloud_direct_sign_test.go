package chain

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestDirectLinkSignRoundTrip P1-3 回归：直链签名「<exp>.<hmac>」生成/校验闭环。
// 公开 302 端点以 URL 即凭证——签名缺失/篡改/过期必须拒绝，密钥未注入时兼容放行。
func TestDirectLinkSignRoundTrip(t *testing.T) {
	c := &CloudChain{directSignKey: []byte("test-sign-key")}

	sign := c.SignDirectLink(3, "/媒体/a.mkv")
	if sign == "" || !strings.Contains(sign, ".") {
		t.Fatalf("签名格式应为 <exp>.<hmac>，got %q", sign)
	}
	if !c.VerifyDirectLinkSign(3, "/媒体/a.mkv", sign) {
		t.Fatal("正确签名应通过校验")
	}
	// 路径不匹配（枚举其他路径）→ 拒绝。
	if c.VerifyDirectLinkSign(3, "/媒体/b.mkv", sign) {
		t.Fatal("签名绑定了路径，换路径必须拒绝")
	}
	// driveId 不匹配 → 拒绝。
	if c.VerifyDirectLinkSign(4, "/媒体/a.mkv", sign) {
		t.Fatal("签名绑定了 driveId，换盘必须拒绝")
	}
	// 篡改 exp（延长有效期）→ HMAC 不匹配拒绝。
	dot := strings.IndexByte(sign, '.')
	if c.VerifyDirectLinkSign(3, "/媒体/a.mkv", "9999999999"+sign[dot:]) {
		t.Fatal("篡改 exp 必须拒绝")
	}
	// 缺失/垃圾签名 → 拒绝。
	if c.VerifyDirectLinkSign(3, "/媒体/a.mkv", "") || c.VerifyDirectLinkSign(3, "/媒体/a.mkv", "junk") {
		t.Fatal("缺失/垃圾签名必须拒绝")
	}
}

// TestDirectLinkSignExpired 过期签名拒绝。
func TestDirectLinkSignExpired(t *testing.T) {
	c := &CloudChain{directSignKey: []byte("k")}
	exp := time.Now().Add(-time.Hour).Unix() // 已过期 1h
	mac := c.directSignMAC(1, "/a", exp)
	if c.VerifyDirectLinkSign(1, "/a", strconv.FormatInt(exp, 10)+"."+mac) {
		t.Fatal("过期签名必须拒绝")
	}
}

// TestDirectLinkSignCompatMode 密钥未注入（安全模式/旧库）→ 不签名不校验（兼容）。
func TestDirectLinkSignCompatMode(t *testing.T) {
	c := &CloudChain{}
	if s := c.SignDirectLink(1, "/a"); s != "" {
		t.Fatalf("兼容模式不应生成签名，got %q", s)
	}
	if !c.VerifyDirectLinkSign(1, "/a", "") {
		t.Fatal("兼容模式应放行（与旧版一致）")
	}
}
