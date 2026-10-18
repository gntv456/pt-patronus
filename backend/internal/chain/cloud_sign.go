package chain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// 直链限时签名（#406 起与 JWT/AES 密钥分域派生）。从 cloud.go 拆出：cloud.go 已到
// 行数门禁上限，签名是自成一体的密码学小节（注入/签发/校验 + legacy 回退）。

// SetAppBaseURL 注入服务对外基址（strm 内容渲染绝对 URL；Emby 远端播放必需）。
func (c *CloudChain) SetAppBaseURL(u string) {
	c.appBaseURL = strings.TrimRight(u, "/")
}

// SetDirectLinkSignKey 注入直链签名密钥。**不再与 JWT/AES 同源**（#406）：
// 现在由 app 侧传 HKDF 按域派生出来的那把；这个单参 Setter 只保留给不跑 app 装配
// 的路径/测试用（它不会注入 legacy 兼容钥）。非空时：
//   - .strm 内容的 {direct} URL 追加 ?sign=<exp>.<hmac>（directLinkSignTTL，7 天）；
//   - 公开 302 端点校验签名，签名缺失/过期/不匹配一律 404。
//
// 收敛「匿名枚举 driveId+路径换全盘直链」面：URL 本身即凭证。过期后播放器 404，
// 由 STRM 重新生成/扫库刷新。兼容：密钥未注入（安全模式/旧库）不签名也不校验。
// #406 兼容：单参 Setter 沿用旧形状（不注入 legacy），双参 SetDirectLinkSignKeys 才会带上。
func (c *CloudChain) SetDirectLinkSignKey(key []byte) { c.directSignKey = key }

// SetDirectLinkSignKeys 注入新派生的签名密钥 + 升级前的旧密钥（legacy 可为 nil）。
//
// 为什么必须带 legacy：.strm 文件里嵌的是**签名后的 URL**，升级改了密钥后，
// 存量 .strm 的签名用新密钥验不过 ⇒ 播放器立刻 404。而 .strm 只在扫库/上传时重写，
// 用户不一定马上重扫。所以校验侧要同时接受旧密钥，等旧签名自然过期（TTL 7 天）。
func (c *CloudChain) SetDirectLinkSignKeys(key, legacy []byte) {
	c.directSignKey = key
	c.directSignLegacy = legacy
}

// 二轮 P0-2 修复：TTL 从 12h 延长到 7 天——上传型 .strm 只在上传瞬间写一次，
// 12h 意味着每天两次全库 404（唯一恢复是管理员手动 repair）。7 天对齐
// cloud-strm cron（默认 6h 一跑 + RepairStrm 兜底），URL 即凭证的安全模型
// 不变：泄露的 .strm URL 本来就在媒体库可读范围内。
const directLinkSignTTL = 7 * 24 * time.Hour

// SignDirectLink 生成 driveId|path|exp 的限时签名「<exp十进制>.<hmac-hex>」。
// 密钥未注入返回空串（不签名，兼容模式）。
func (c *CloudChain) SignDirectLink(driveID uint, cloudPath string) string {
	if len(c.directSignKey) == 0 {
		return ""
	}
	exp := time.Now().Add(directLinkSignTTL).Unix()
	return fmt.Sprintf("%d.%s", exp, c.directSignMAC(driveID, cloudPath, exp))
}

func (c *CloudChain) directSignMAC(driveID uint, cloudPath string, exp int64) string {
	mac := hmac.New(sha256.New, c.directSignKey)
	fmt.Fprintf(mac, "%d|%s|%d", driveID, cloudPath, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

// directLinkMACWith 用给定密钥算签名（#406）。
//
// 拆出来是因为校验侧要试两把密钥：新派生的那把，以及升级前原始 SECRET_KEY 的那把
// （存量 .strm 里的 URL 用旧密钥签名，换密钥后必须仍能验过，否则全库 404）。
func (c *CloudChain) directLinkMACWith(key []byte, driveID uint, cloudPath string, exp int64) string {
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%d|%s|%d", driveID, cloudPath, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyDirectLinkSign 校验「<exp>.<hmac>」签名（exp 未过 + 恒定时间 HMAC 匹配）。
// 密钥未注入时放行（兼容模式——与旧版行为一致）。
func (c *CloudChain) VerifyDirectLinkSign(driveID uint, cloudPath, sign string) bool {
	// 两把都空才是兼容模式（安全模式/旧库）。只注入 legacy 时**不能**因新 key 空而放行——
	// 那等于绕开签名校验，正是这个出口存在的理由。
	if len(c.directSignKey) == 0 && len(c.directSignLegacy) == 0 {
		return true
	}
	dot := strings.IndexByte(sign, '.')
	if dot <= 0 {
		return false
	}
	var exp int64
	if _, err := fmt.Sscanf(sign[:dot], "%d", &exp); err != nil || time.Now().Unix() > exp {
		return false
	}
	got := sign[dot+1:]
	want := c.directLinkMACWith(c.directSignKey, driveID, cloudPath, exp)
	if hmac.Equal([]byte(want), []byte(got)) {
		return true
	}
	// #406 旧密钥回退：存量 .strm 的签名是用升级前的密钥算的，等它们自然过期即可。
	if len(c.directSignLegacy) > 0 {
		want = c.directLinkMACWith(c.directSignLegacy, driveID, cloudPath, exp)
		if hmac.Equal([]byte(want), []byte(got)) {
			return true
		}
	}
	return false
}
