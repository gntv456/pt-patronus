package security

// #406 接线护栏（形状层）：三个密钥域必须各挂各的 HKDF info。
//
// 行为测试从结果断言（派生出来的字节是否两两不同），这条从形状断言
// （生产代码是否真的调了 DeriveKey）。两边都退化成旧形状时才会红 ——
// 单留任何一边都会被「改了实现忘了改另一侧」绕过。

import (
	"os"
	"strings"
	"testing"
)

// TestWiring_ThreeKeyDomainsEachUseOwnInfo JWT/AES/直链必须各用各的 info。
func TestWiring_ThreeKeyDomainsEachUseOwnInfo(t *testing.T) {
	jwtSrc := stripSecurityComments(t, "jwt.go")

	// 正向：NewJWT 的签发密钥来自 DeriveKey(secret, hkdfInfoJWT)。
	if !strings.Contains(jwtSrc, "DeriveKey(secret, hkdfInfoJWT)") {
		t.Errorf("NewJWT 必须用 hkdfInfoJWT 域派生签发密钥（#406），实得：\n%s", jwtSrc)
	}
	// 反向（重点）：签发路径上不许再出现裸 []byte(secret) 赋给 secret。
	// 只钉 NewJWT 的函数体，避免把 issue() 里 tok.SignedString(j.secret) 误伤。
	newJWT := braceBalanced(jwtSrc, "func NewJWT")
	if strings.Contains(newJWT, "secret:  []byte(secret)") || strings.Contains(newJWT, "secret: []byte(secret)") {
		t.Errorf("NewJWT 的 secret 仍是原始字节 —— 域分隔被绕回去了（#406）：\n%s", newJWT)
	}
	// 反向：验签必须保留 legacy 回退（否则升级即全站登出）。
	if !strings.Contains(jwtSrc, "j.legacySecret") {
		t.Error("jwt.go 里验签不再回退 legacySecret —— 存量 token 会全部失效（升级即全站登出）")
	}
	// 反向：legacySecret 不许拿去签发。
	issue := braceBalanced(jwtSrc, "func (j *JWT) issue")
	if strings.Contains(issue, "legacySecret") {
		t.Errorf("issue() 里用了 legacySecret 签发 ⇒ 新 token 仍与 AES 同源（#406）：\n%s", issue)
	}
	if !strings.Contains(issue, "j.secret") {
		t.Error("issue() 必须用 j.secret 签发")
	}
}

// TestWiring_CryptoAESDomainUnchanged AES 侧 info 不能被顺手改动。
//
// AES 的 legacyKey 回退（crypto_compat_test）依赖「新派生 + 旧 SHA-256 直取」并存。
// 若有人把 hkdfInfoAES 改掉，存量密文与新密文会互相打不开，而全绿测试看不出原因。
func TestWiring_CryptoAESDomainUnchanged(t *testing.T) {
	src := stripSecurityComments(t, "crypto.go")
	if !strings.Contains(src, `hkdfInfoAES = "ptpatronus:aes-at-rest:v1"`) {
		t.Error("hkdfInfoAES 被改动了 —— 改了等于换密钥，存量密文会解不开（须升 vN 并说明迁移）")
	}
	// 三个 info 串必须互不相同（改到撞车就是域分隔失效）。
	infos := map[string]string{
		"aes":        "ptpatronus:aes-at-rest:v1",
		"jwt":        "ptpatronus:jwt-hs256:v1",
		"directlink": "ptpatronus:directlink-hmac:v1",
	}
	seen := map[string]string{}
	for name, v := range infos {
		if !strings.Contains(src, `"`+v+`"`) {
			t.Errorf("%s 的 info 串在 crypto.go 里找不到了：%q", name, v)
		}
		if prev, dup := seen[v]; dup {
			t.Errorf("info 串撞车：%s 与 %s 同用 %q（域分隔失效）", prev, name, v)
		}
		seen[v] = name
	}
}

// stripSecurityComments 去整行注释（避免注释里的旧形状被当证据）。
func stripSecurityComments(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读不到 %s: %v", file, err)
	}
	lines := strings.Split(string(b), "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if s := strings.TrimSpace(ln); s == "" || strings.HasPrefix(s, "//") {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// braceBalanced 按大括号配平从 from 处截出一个函数体（含签名行）。
func braceBalanced(src, from string) string {
	idx := strings.Index(src, from)
	if idx < 0 {
		return ""
	}
	depth, end := 0, -1
	for i := idx; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return src[idx:]
	}
	return src[idx : end+1]
}
