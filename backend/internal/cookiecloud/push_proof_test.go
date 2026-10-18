package cookiecloud

import "testing"

// 下面三条密文是浏览器扩展 extension/backup.js 的 cookieCloudEncrypt 真实产出的
// （node 跑扩展代码得到，Salted__ + AES-256-CBC + EVP_BytesToKey(MD5) 派生 key32+iv16），
// uuid/口令固定为 extPinUUID/extPinPass。钉第三方实现的密文而不是本包 Encrypt 的输出，
// 才能防「实现自洽」——扩展曾经只派生 32 字节（退化成 AES-128），本包根本解不开它推的东西。
const (
	extPinUUID = "ext-pin-uuid"
	extPinPass = "ext-pin-pass"
	// {"cookie_data":{"demo.pt":[{"name":"sess","value":"abc","domain":"demo.pt","path":"/"}]},"local_storage_data":{}}
	extPinArray = "U2FsdGVkX18QS0L56AU4GgPtE1cpW0EbdPv1maQePnJz4MXznw17mZ3573dDFpTGx1EDXlaLIQdWgEevtRNV6hKIp2tLNq8PKZZH1MOTwWPiLgFOkUYiVfqCfyAskh1IpZ/VdYM3vdwuWRzBr+54znX+qF0MRrYugiv3hKbT1N4gIU78lBWWl+bjNwpVlyz4"
	// {"cookie_data":{"demo.pt":{"sess":"abc","uid":"1"}},"local_storage_data":{}}——映射式负载（旧扩展写法）
	extPinMap = "U2FsdGVkX1/twncjEN2YOPG/Pxh3HT3bcRXvqlTbKUBQl1eKLELR1YltssBPiHif4AviaZTm1CF+03TxQ2ng3KhQ4TgLX9/eXIJGvsJ6FTwrcCR/pFjK3kz4zEYFnBHq"
	// {"cookie_data":{},"update_time":1700000000}——用户清空台账后的推送
	extPinEmpty = "U2FsdGVkX1+xVDrjwH/QgfJ+tBAZly4xdd45NA3+F3PCmgsRLMf2SHw277V63Uo2qLFqouEpb9JIkXqKRCB08w=="
)

func extPinCiphers() map[string]string {
	return map[string]string{"array": extPinArray, "map": extPinMap, "empty": extPinEmpty}
}

// TestVerifyPushProofAcceptsExtensionCiphertext 扩展推的三种负载都算「对端知道口令」。
func TestVerifyPushProofAcceptsExtensionCiphertext(t *testing.T) {
	for name, cipher := range extPinCiphers() {
		if err := VerifyPushProof(cipher, extPinUUID, extPinPass); err != nil {
			t.Errorf("%s 负载口令正确，应通过证明：%v", name, err)
		}
		if err := VerifyPushProof(cipher, extPinUUID, extPinPass+"-wrong"); err == nil {
			t.Errorf("%s 负载换了口令仍通过证明 → 门禁形同虚设", name)
		}
	}
}

// TestVerifyPushProofIsShapeAgnostic 关键对照：映射式负载 DecryptPayload 解不开，
// 但口令证明确实成立。写入门禁若继续用 DecryptPayload 把关，就会把这种推送 404 掉。
func TestVerifyPushProofIsShapeAgnostic(t *testing.T) {
	if err := VerifyPushProof(extPinMap, extPinUUID, extPinPass); err != nil {
		t.Fatalf("映射式负载的口令证明应成立：%v", err)
	}
	if _, err := DecryptPayload(extPinMap, extPinUUID, extPinPass); err == nil {
		t.Error("用例前提失效：映射式 cookie_data 现在能被解析，本对照不再承重")
	}
	// 正确口令下的非 JSON 垃圾不算证明（CBC+PKCS7 约 1/256 会碰巧通过填充校验）
	garbage, err := Encrypt([]byte("not json at all"), DeriveKey(extPinUUID, extPinPass))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPushProof(garbage, extPinUUID, extPinPass); err == nil {
		t.Error("明文不是 JSON 对象时不应算作口令证明")
	}
}

// TestParsePayloadExtShapes 扩展密文解开后的解析口径：数组形状正常、空台账是合法空值、
// 映射形状仍然解析失败（这一点是刻意的——协议是数组，但要让它去当写入门禁就会误拒）。
func TestParsePayloadExtShapes(t *testing.T) {
	key := DeriveKey(extPinUUID, extPinPass)
	plainOf := func(name, cipher string) []byte {
		t.Helper()
		plain, err := Decrypt(cipher, key)
		if err != nil {
			t.Fatalf("%s 解密失败（格式不合规？）：%v", name, err)
		}
		return plain
	}

	got, err := ParsePayload(plainOf("array", extPinArray))
	if err != nil {
		t.Fatalf("array 解析失败：%v", err)
	}
	if len(got) != 1 || len(got["demo.pt"]) != 1 || got["demo.pt"][0].Domain != "demo.pt" {
		t.Errorf("array 解析结果 = %v", got)
	}
	if s := CookieStringForHost(got, "demo.pt"); s != "sess=abc" {
		t.Errorf("cookie 串 = %q", s)
	}

	empty, err := ParsePayload(plainOf("empty", extPinEmpty))
	if err != nil {
		t.Errorf("清空台账（cookie_data:{}）应解析成空值而不是报错：%v", err)
	}
	if len(empty) != 0 {
		t.Errorf("空台账 = %v", empty)
	}

	if _, err := ParsePayload(plainOf("map", extPinMap)); err == nil {
		t.Error("映射式 cookie_data 现在能被解析，本用例的对照前提失效")
	}
}
