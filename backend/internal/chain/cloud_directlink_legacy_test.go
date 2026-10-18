package chain

// #406 直链签名密钥此前直接吃 []byte(cfg.SecretKey)，与 AES-at-rest / JWT
// 三处共用同一段原始字节。直链 URL 会写进 .strm 文件、被播放器读出去，
// 泄漏面比 JWT 更宽。
//
// 但换密钥不能一刀切：.strm 里嵌的是**签名后的 URL**，而 .strm 只在
// 扫库/上传时重写，用户不一定马上重扫。只认新密钥 ⇒ 升级当天播放器全库 404。
// 所以校验侧必须同时接受升级前的旧密钥，等旧签名按 TTL 自然过期。
//
// 下面钉四件事：
//  1. 存量（旧密钥签的）URL 仍能验过 —— 否则升级即全库 404；
//  2. 新密钥签的 URL 能验过；
//  3. 别的密钥签的必须拒 —— 兼容层不许变成万能钥匙；
//  4. 两把都空才是兼容模式；只注入 legacy 时**不能**因新 key 空而放行。

import (
	"strconv"
	"testing"
	"time"
)

// signWithLegacy 造一份「升级前签发」的存量 URL 签名。
// 直接复用生产里的 MAC 计算，而不是照抄一遍 —— 照抄的话两边一起改错也测不出来。
func signWithLegacy(t *testing.T, key []byte, driveID uint, path string) string {
	t.Helper()
	c := &CloudChain{}
	exp := time.Now().Add(time.Hour).Unix()
	return strconv.FormatInt(exp, 10) + "." + c.directLinkMACWith(key, driveID, path, exp)
}

// TestDirectLinkSignLegacyTokenStillVerifies 存量 .strm 的签名必须仍能过。
func TestDirectLinkSignLegacyTokenStillVerifies(t *testing.T) {
	const driveID = uint(5)
	const path = "/媒体/升级前写入.mkv"

	legacyKey := []byte("raw-secret-key-before-406")
	newKey := []byte("hkdf-derived-directlink-key-406")
	c := &CloudChain{}
	c.SetDirectLinkSignKeys(newKey, legacyKey)

	if !c.VerifyDirectLinkSign(driveID, path, signWithLegacy(t, legacyKey, driveID, path)) {
		t.Fatal("升级前签进 .strm 的直链签名必须仍能验过（否则升级当天播放全库 404）")
	}
	// 新签发的（app 现在传派生密钥）也必须能验过。
	if !c.VerifyDirectLinkSign(driveID, path, signWithLegacy(t, newKey, driveID, path)) {
		t.Fatal("新密钥签的签名必须能验过")
	}
	// 别的密钥：兼容层不是万能钥匙。
	if c.VerifyDirectLinkSign(driveID, path, signWithLegacy(t, []byte("someone-elses-key"), driveID, path)) {
		t.Fatal("无关密钥签的签名必须被拒（兼容层只认新旧两把）")
	}
	// 路径/盘不匹配仍拒（兼容回退不得放松绑定）。
	if c.VerifyDirectLinkSign(driveID, "/媒体/别的.mkv", signWithLegacy(t, legacyKey, driveID, path)) {
		t.Fatal("换路径必须拒（legacy 回退不得放松路径绑定）")
	}
	if c.VerifyDirectLinkSign(driveID+1, path, signWithLegacy(t, legacyKey, driveID, path)) {
		t.Fatal("换 driveId 必须拒")
	}
}

// TestDirectLinkSignLegacyIsExpiredToo 旧密钥回退不豁免 TTL。
//
// 回退的是「用哪把钥匙算 HMAC」，不是「过期也能用」。否则拿到旧签名的人可以
// 永久重放，而这条签名原本就是用来防匿名枚举的。
func TestDirectLinkSignLegacyIsExpiredToo(t *testing.T) {
	legacyKey := []byte("legacy-key")
	c := &CloudChain{}
	c.SetDirectLinkSignKeys([]byte("new-key"), legacyKey)

	exp := time.Now().Add(-time.Hour).Unix()
	stale := strconv.FormatInt(exp, 10) + "." + c.directLinkMACWith(legacyKey, 1, "/a", exp)
	if c.VerifyDirectLinkSign(1, "/a", stale) {
		t.Fatal("过期签名即便用 legacy 密钥签的也必须拒绝 —— 兼容回退只换钥匙，不豁免 TTL")
	}
}

// TestDirectLinkSignNewSignUsesDerivedKeyOnly 新签名只用新密钥，不带 legacy。
//
// 若 SignDirectLink 也回退到 legacy（或反过来，签发用了 legacy），
// 新写进 .strm 的 URL 依旧与 AES 同源，这次改造等于没做。
func TestDirectLinkSignNewSignUsesDerivedKeyOnly(t *testing.T) {
	legacyKey := []byte("legacy-key")
	newKey := []byte("new-key")
	c := &CloudChain{}
	c.SetDirectLinkSignKeys(newKey, legacyKey)

	sign := c.SignDirectLink(2, "/媒体/新写的.mkv")
	if sign == "" {
		t.Fatal("注入新密钥后必须签名")
	}
	if !c.VerifyDirectLinkSign(2, "/媒体/新写的.mkv", sign) {
		t.Fatal("自己签的必须自己验得过")
	}

	dot := indexByte(sign)
	exp, err := strconv.ParseInt(sign[:dot], 10, 64)
	if err != nil {
		t.Fatalf("签名头部不是十进制 exp：%q", sign)
	}
	// 用 legacy 重算一遍：签名必须**不**等于 legacy 算出的 MAC。
	if sign[dot+1:] == c.directLinkMACWith(legacyKey, 2, "/媒体/新写的.mkv", exp) {
		t.Fatal("新签名是用 legacy 密钥算的 ⇒ 域分隔没生效")
	}
	if sign[dot+1:] != c.directLinkMACWith(newKey, 2, "/媒体/新写的.mkv", exp) {
		t.Fatal("新签名必须用新（派生）密钥计算")
	}
}

// TestDirectLinkCompatModeNeedsBothKeysEmpty 只注入 legacy 不能放行。
//
// 这是本条最容易写成假绿的地方：若 VerifyDirectLinkSign 只看新 key 是否为空，
// 「只注入了旧钥」的装配（key 漏传、配置出错、迁移写到一半）会静默变成
// 完全不校验签名 —— 而这个出口的全部意义就是防匿名枚举换全盘直链。
func TestDirectLinkCompatModeNeedsBothKeysEmpty(t *testing.T) {
	legacyKey := []byte("legacy-only-key")
	c := &CloudChain{}
	c.directSignLegacy = legacyKey

	if c.VerifyDirectLinkSign(1, "/a", "") {
		t.Fatal("只注入了 legacy、新 key 为空时不得当作兼容模式放行（会静默关掉签名校验）")
	}
	if c.SignDirectLink(1, "/a") != "" {
		t.Fatal("新 key 为空时不应签名（没有可用的签发密钥）")
	}

	// 两把都空 ⇒ 真正的兼容模式，放行（与旧版行为一致，不能回归）。
	c2 := &CloudChain{}
	if !c2.VerifyDirectLinkSign(1, "/a", "") {
		t.Fatal("两把都空是兼容模式，应放行（与旧版一致）")
	}
}

func indexByte(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}
