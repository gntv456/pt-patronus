// Package cookiecloud 实现 CookieCloud 协议解密与 host 匹配。
// 算法：AES-128-CBC，密钥 = md5(password + "-" + md5(password)) 前 16 字符，IV = 密钥。
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

// DeriveKey 按 CookieCloud 协议派生 16 字节密钥。
func DeriveKey(password string) []byte {
	full := md5hex(password + "-" + md5hex(password))
	return []byte(full[:16])
}

// Decrypt 用 AES-128-CBC（IV=密钥）解密 base64 密文并去 PKCS7 填充。
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

// DecryptPayload 解密并解析为 {host: []CookieEntry}。
func DecryptPayload(encryptedB64, password string) (map[string][]CookieEntry, error) {
	key := DeriveKey(password)
	plain, err := Decrypt(encryptedB64, key)
	if err != nil {
		return nil, err
	}
	var payload map[string][]CookieEntry
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}
	return payload, nil
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
func Fetch(ctx context.Context, server, uuid, password string) (map[string][]CookieEntry, error) {
	u := strings.TrimRight(server, "/") + "/get/" + url.PathEscape(uuid) + "?password=" + url.QueryEscape(password)
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
	var envelope struct {
		Encrypted string `json:"encrypted"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse envelope: %w", err)
	}
	if envelope.Encrypted == "" {
		return nil, fmt.Errorf("empty encrypted payload")
	}
	return DecryptPayload(envelope.Encrypted, password)
}
