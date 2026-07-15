// Package cookiecloud 实现 CookieCloud 协议解密与 host 匹配。
//
// 标准 CookieCloud 协议（easychen/CookieCloud 官方浏览器扩展，与 MoviePilot servcookie
// 一致）：AES-128-CBC，key = IV = md5(uuid + "-" + password) 的 hex 前 16 字符（即 16 字节
// ASCII），PKCS7 填充，无 OpenSSL "Salted__" 头。解密后 JSON 形如
// {"cookie_data": {host: [CookieEntry]}, "update_time": ...}（标准扩展带 cookie_data 包裹）。
//
// 本包只负责「解密 + 解析」纯逻辑，不持有 HTTP 客户端：生产拉取由 chain 层用项目的
// httpclient（带 SSRF 防护）GET 后调 DecodeEnvelope；Fetch 仅供测试/CLI（标准库、无 SSRF）。
package cookiecloud

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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

// Decrypt 用 AES-128-CBC（IV=密钥）解密 base64 密文并去 PKCS7 填充。
// key 需为 DeriveKey 派生的 16 字节。
func Decrypt(b64 string, key []byte) ([]byte, error) {
	if len(key) < aes.BlockSize {
		return nil, fmt.Errorf("key too short")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}
	if len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length invalid")
	}
	block, err := aes.NewCipher(key[:aes.BlockSize])
	if err != nil {
		return nil, err
	}
	iv := key[:aes.BlockSize]
	pt := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, raw)
	// PKCS7 去填充
	pad := int(pt[len(pt)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(pt) {
		return nil, fmt.Errorf("bad pkcs7 padding")
	}
	// 校验填充字节
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
	// 标准 CookieCloud：{"cookie_data": {host: [cookies]}, "update_time": ...}
	var wrapped struct {
		CookieData map[string][]CookieEntry `json:"cookie_data"`
	}
	if err := json.Unmarshal(plain, &wrapped); err == nil && len(wrapped.CookieData) > 0 {
		return wrapped.CookieData, nil
	}
	// 回退：直接的 {host: [cookies]}（部分服务端无包裹层）
	var payload map[string][]CookieEntry
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}
	return payload, nil
}

// DecodeEnvelope 解析服务器响应 {"encrypted": "..."} 并解密为 {host: []CookieEntry}。
// 生产路径：chain 层用项目的 httpclient（SSRF 防护）GET 拿到 body 后调用本函数。
func DecodeEnvelope(rawBody []byte, uuid, password string) (map[string][]CookieEntry, error) {
	var envelope struct {
		Encrypted string `json:"encrypted"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return nil, fmt.Errorf("parse envelope: %w", err)
	}
	if envelope.Encrypted == "" {
		return nil, fmt.Errorf("empty encrypted payload")
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
