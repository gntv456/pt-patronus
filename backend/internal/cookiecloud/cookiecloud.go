// Package cookiecloud 实现 CookieCloud 协议解密与 host 匹配。
//
// CookieCloud 官方浏览器扩展存在两代加密格式（扩展设置「加密方式」可选，默认 legacy）：
//   - legacy：CryptoJS 兼容 OpenSSL 格式，base64("Salted__" + 8 字节盐 + AES-256-CBC 密文)，
//     key/IV 由 EVP_BytesToKey(MD5, 1 轮) 从 the_key = md5(uuid+"-"+password) hex 前 16
//     字节派生（32 字节 key + 16 字节 IV）。
//   - aes-128-cbc-fixed：AES-128-CBC，key = IV = md5(uuid + "-" + password) 的 hex 前 16
//     字符（即 16 字节 ASCII），PKCS7 填充。部分第三方客户端（如本项目早期版本）用此格式。
//
// 解密按密文前缀自动识别格式，用户无需关心。解密后 JSON 形如
// {"cookie_data": {host: [CookieEntry]}, "update_time": ...}（标准扩展带 cookie_data 包裹）。
//
// 本包只负责「解密 + 解析」纯逻辑，不持有 HTTP 客户端：生产拉取由 chain 层用项目的
// httpclient（带 SSRF 防护）GET 后调 DecodeEnvelope；Fetch 仅供测试/CLI，已兜底加公网校验。
package cookiecloud

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// CookieEntry 是 CookieCloud 负载里的单条 cookie。
type CookieEntry struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	HTTPOnly bool   `json:"httpOnly"`
	Secure   bool   `json:"secure"`
}

// md5hex 返回字符串的 md5 十六进制。
func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// DeriveKey 按 CookieCloud 标准协议派生 16 字节密钥（同时用作 IV）：
// md5(uuid + "-" + password) 的 hex 前 16 字符。
func DeriveKey(uuid, password string) []byte {
	full := md5hex(uuid + "-" + password)
	return []byte(full[:16])
}

// Encrypt 用 AES-128-CBC（IV=密钥）加密明文并做 PKCS7 填充，返回 base64 密文。
// 与 Decrypt 对称，供推送 CookieCloud /update 使用。
func Encrypt(plain []byte, key []byte) (string, error) {
	if len(key) < aes.BlockSize {
		return "", fmt.Errorf("key too short")
	}
	if len(plain) == 0 {
		return "", fmt.Errorf("empty plaintext")
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	if pad == 0 {
		pad = aes.BlockSize
	}
	padded := make([]byte, len(plain)+pad)
	copy(padded, plain)
	for i := len(plain); i < len(padded); i++ {
		padded[i] = byte(pad)
	}
	block, err := aes.NewCipher(key[:aes.BlockSize])
	if err != nil {
		return "", err
	}
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out), nil
}

// EncryptPayload 将 cookie_data 包成标准 CookieCloud JSON 并加密。
func EncryptPayload(cookieData map[string][]CookieEntry, uuid, password string) (string, error) {
	if cookieData == nil {
		cookieData = map[string][]CookieEntry{}
	}
	body := struct {
		CookieData map[string][]CookieEntry `json:"cookie_data"`
		UpdateTime int64                    `json:"update_time"`
	}{CookieData: cookieData, UpdateTime: time.Now().Unix()}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return Encrypt(raw, DeriveKey(uuid, password))
}

// EncodeUpdateRequest 构造 POST /update 的 JSON body：{"uuid","encrypted"}。
func EncodeUpdateRequest(uuid, encrypted string) ([]byte, error) {
	return json.Marshal(map[string]string{
		"uuid":      uuid,
		"encrypted": encrypted,
	})
}

// EntriesFromCookieHeader 把 "k=v; k2=v2" 转为 CookieCloud CookieEntry 列表。
// domain 使用站点 host；path 默认 "/"；Secure 默认 true（PT 站几乎全 HTTPS）。
func EntriesFromCookieHeader(domain, cookieStr string) []CookieEntry {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" || strings.TrimSpace(cookieStr) == "" {
		return nil
	}
	var out []CookieEntry
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		name := strings.TrimSpace(kv[0])
		if name == "" {
			continue
		}
		out = append(out, CookieEntry{
			Name:   name,
			Value:  strings.TrimSpace(kv[1]),
			Domain: domain,
			Path:   "/",
			Secure: true,
		})
	}
	return out
}

// MergeCookieData 把 src 合并进 dst：同 host 整表覆盖（推送端本地为真源）。
func MergeCookieData(dst, src map[string][]CookieEntry) map[string][]CookieEntry {
	if dst == nil {
		dst = map[string][]CookieEntry{}
	}
	for h, entries := range src {
		if h == "" || len(entries) == 0 {
			continue
		}
		dst[h] = entries
	}
	return dst
}

// CookieCloud 加密格式（官方扩展 crypto_type）：
//   - aes-128-cbc-fixed：key = IV = md5(uuid+"-"+password) hex 前 16 字节，PKCS7。子项目自研
//     客户端按此实现；新扩展亦可选此格式（当前 UI 默认仍为 legacy）。
//   - legacy：CryptoJS OpenSSL 兼容格式（"Salted__" + 8 字节盐 + AES-256-CBC），key/IV 由
//     EvpKDF(MD5, 1 轮) 从 the_key 派生。解密侧按前缀自动识别，无需用户选择。
type CryptoType string

const (
	CryptoTypeLegacy    CryptoType = "legacy"
	CryptoTypeAES128CBC CryptoType = "aes-128-cbc-fixed"
)

// evpBytesToKey 是 OpenSSL EVP_BytesToKey（MD5、1 轮迭代）：
// D1=MD5(password‖salt)，Dn=MD5(D(n-1)‖password‖salt)，串联取前 size 字节。
// 与 CryptoJS EvpKDF 默认参数一致（crypto-js/evpkdf.js）。
func evpBytesToKey(password, salt []byte, size int) []byte {
	var out []byte
	var prev []byte
	for len(out) < size {
		h := md5.Sum(append(append(prev, password...), salt...))
		// md5.Sum 的入参 slice 可能复用底层数组，h 是值拷贝可安全 append
		prev = h[:]
		out = append(out, h[:]...)
	}
	return out[:size]
}

// Decrypt 解密 CookieCloud 密文。ciphertext 以 "Salted__" 开头时按 legacy CryptoJS
// OpenSSL 兼容格式处理（Salted__ + 8 字节盐 + AES-256-CBC，EvpKDF 派生 key/IV）；
// 否则按本端标准格式 AES-128-CBC（IV=key）处理。两者均 PKCS7 去填充。
func Decrypt(b64 string, key []byte) ([]byte, error) {
	if len(key) < aes.BlockSize {
		return nil, fmt.Errorf("key too short")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("ciphertext length invalid")
	}
	// saltedHdrLen 是 "Salted__" + 8 字节盐的长度；legacy 密文 = 头 + AES-256-CBC 块
	const saltedHdrLen = 8 + 8
	if len(raw) > saltedHdrLen && string(raw[:8]) == "Salted__" {
		return decryptSalted(raw, key[:aes.BlockSize])
	}
	if len(raw)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length invalid")
	}
	block, err := aes.NewCipher(key[:aes.BlockSize])
	if err != nil {
		return nil, err
	}
	iv := key[:aes.BlockSize]
	pt := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, raw)
	return unpadPKCS7(pt)
}

// decryptSalted 解密 legacy 格式："Salted__" + 8 字节盐 + AES-256-CBC 密文。
// key 为 DeriveKey 的 16 字节，作 EvpKDF 的 password；派生 32 字节 key + 16 字节 IV。
func decryptSalted(raw, key []byte) ([]byte, error) {
	const hdr = 8 + 8 // "Salted__" + 盐
	if len(raw) <= hdr || (len(raw)-hdr)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length invalid")
	}
	salt := raw[8:hdr]
	dk := evpBytesToKey(key, salt, 32+aes.BlockSize)
	block, err := aes.NewCipher(dk[:32])
	if err != nil {
		return nil, err
	}
	body := raw[hdr:]
	pt := make([]byte, len(body))
	cipher.NewCBCDecrypter(block, dk[32:32+aes.BlockSize]).CryptBlocks(pt, body)
	return unpadPKCS7(pt)
}

// unpadPKCS7 校验并去除 PKCS7 填充。
func unpadPKCS7(pt []byte) ([]byte, error) {
	if len(pt) == 0 {
		return nil, fmt.Errorf("bad pkcs7 padding")
	}
	pad := int(pt[len(pt)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(pt) {
		return nil, fmt.Errorf("bad pkcs7 padding")
	}
	for i := len(pt) - pad; i < len(pt); i++ {
		if int(pt[i]) != pad {
			return nil, fmt.Errorf("bad pkcs7 padding")
		}
	}
	return pt[:len(pt)-pad], nil
}

// DecryptPayload 解密 encryptedB64 并解析为 {host: []CookieEntry}。
// 兼容两种格式：标准扩展的 {"cookie_data": {...}} 包裹（优先），以及直接的 {host: [...]}。
func DecryptPayload(encryptedB64, uuid, password string) (map[string][]CookieEntry, error) {
	key := DeriveKey(uuid, password)
	plain, err := Decrypt(encryptedB64, key)
	if err != nil {
		return nil, err
	}
	return ParsePayload(plain)
}

// ParsePayload 解析 CookieCloud 明文为 {host: []CookieEntry}。
// 标准扩展带 {"cookie_data": {host: [cookies]}, "update_time": ...} 包裹；
// cookie_data 存在但为空是合法状态（用户清空了台账），返回空映射而不是报错——
// 报错会让上层以为「云端内容未知」而放弃推送，把「云端是空的」当成了「云端读不到」。
// 无包裹层的服务端（部分自建实现）则直接把顶层当 {host: [cookies]}。
func ParsePayload(plain []byte) (map[string][]CookieEntry, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(plain, &top); err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}
	raw, isWrapped := top["cookie_data"]
	if !isWrapped {
		raw = plain
	}
	var data map[string][]CookieEntry
	if err := json.Unmarshal(raw, &data); err != nil {
		if isWrapped {
			return nil, fmt.Errorf("parse cookie_data: %w", err)
		}
		return nil, fmt.Errorf("parse payload: %w", err)
	}
	return data, nil
}

// VerifyPushProof 校验密文确实由该口令加密：解密成功（PKCS7 填充校验通过）且明文是 JSON 对象。
// 内置服务端 /update 的写入门禁用它——要的是「对端知道口令」这一证明，而不是
// 「我们的 cookie 解析器认得这份负载」：拿 DecryptPayload 把关会把解得开、但 cookie 结构
// 形状陌生的合法客户端一并误判成伪造。要求「是 JSON 对象」是为压掉 CBC+PKCS7 约 1/256
// 的随机数据碰巧通过填充校验。
func VerifyPushProof(encryptedB64, uuid, password string) error {
	plain, err := Decrypt(encryptedB64, DeriveKey(uuid, password))
	if err != nil {
		return err
	}
	s := strings.TrimLeft(string(plain), " \t\r\n")
	if s == "" || s[0] != '{' {
		return fmt.Errorf("payload is not a JSON object")
	}
	return nil
}

// Envelope 是 GET /get/{uuid} 的响应；crypto_type 标记加密格式（老服务端无此字段）。
type Envelope struct {
	Encrypted  string `json:"encrypted"`
	CryptoType string `json:"crypto_type"`
}

// ErrEmptyPayload 云端该 UUID 还没有任何数据（首次推送前的常态）。
// 调用方必须把它和「拉取真失败」区分开：前者可以从空 base 开始推送，
// 后者绝不能——CookieCloud 的 PUT 是整体替换，拿空 base 推等于抹掉云端其它 host。
var ErrEmptyPayload = errors.New("empty encrypted payload")

// DecodeEnvelope 解析服务器响应并解密为 {host: []CookieEntry}。
// crypto_type=aes-128-cbc-fixed 时按标准 fixed-IV 格式解；legacy 时限制为 legacy
// OpenSSL 格式（Salted__）。两者实际上按密文前缀自动识别，crypto_type 仅影响报错语义。
// 生产路径：chain 层用项目的 httpclient（SSRF 防护）GET 拿到 body 后调用本函数。
func DecodeEnvelope(rawBody []byte, uuid, password string) (map[string][]CookieEntry, error) {
	var envelope Envelope
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return nil, fmt.Errorf("parse envelope: %w", err)
	}
	if envelope.Encrypted == "" {
		return nil, fmt.Errorf("%w", ErrEmptyPayload)
	}
	return DecryptPayload(envelope.Encrypted, uuid, password)
}

// CookieStringForHost 返回匹配 host（域名后缀）的 cookie 拼接串 "k=v; k2=v2"。
func CookieStringForHost(payload map[string][]CookieEntry, host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	var parts []string
	for h, cookies := range payload {
		if hostMatches(h, host) {
			for _, c := range cookies {
				if c.Name == "" {
					continue
				}
				parts = append(parts, c.Name+"="+c.Value)
			}
		}
	}
	return strings.Join(parts, "; ")
}

// hostMatches 报告 cookieHost 是否匹配目标 host（域名后缀匹配，去掉前导点）。
func hostMatches(cookieHost, target string) bool {
	ch := strings.TrimPrefix(strings.ToLower(cookieHost), ".")
	return ch == target || strings.HasSuffix(target, "."+ch)
}

// Fetch 从 CookieCloud 服务器拉取并解密。server 形如 https://cc.example.com。
//
// 注意：本函数用标准库 net/http，无 SSRF 防护，仅供测试/CLI。生产路径应在 chain 层用项目的
// httpclient.Client.Get（ssrf=true）拉取后调 DecodeEnvelope，避免用户把 server 配成内网。
func Fetch(ctx context.Context, server, uuid, password string) (map[string][]CookieEntry, error) {
	u := strings.TrimRight(server, "/") + "/get/" + url.PathEscape(uuid)
	// 即便 Fetch 仅用于测试/CLI（生产由 chain 层 httpclient 拉取），也兜底做公网校验，
	// 防止误用拉取内网地址（与 chain 层 SSRF 防护一致）。
	if err := httpclient.AssertPublicURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// 限制 16MB：CookieCloud 响应正常 <1MB，防恶意/被入侵服务器返回超大 body 耗尽内存。
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cookiecloud server returned %d", resp.StatusCode)
	}
	return DecodeEnvelope(body, uuid, password)
}
