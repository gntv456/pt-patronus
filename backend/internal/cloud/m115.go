// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
)

// ---- m115 加密契约（115 web/proapi downurl，社区逆向的公开算法族） ----
//
// 协议要点（与 115 网页端 proapi /app/chrome/downurl 一致）：
//  1. 客户端生成临时 ECDH P-256 密钥对，公钥发服务端（hex）；
//  2. 与服务端返回的公钥做 ECDH → 共享秘密；
//  3. 共享秘密 X 坐标 sha256 → 32 字节密钥；前 16 字节为 AES key、后 16 字节为 IV；
//  4. 请求的 data 字段 = AES-CBC(明文 JSON) 再 base64；
//  5. 响应 data 字段 = base64 → AES-CBC 解密 → JSON（含 url.url[0] 直链）。
//
// 说明：ECDH 用标准库 crypto/ecdh（P256），不引第三方依赖；服务端公钥在 downurl
// 响应体同级字段（公开的固定协商流程），密钥派生无 HKDF（原始 X 坐标 sha256，
// 与网页端 JS 一致）。

// m115Keys 一次 downurl 协商的密钥材料。
type m115Keys struct {
	aesKey []byte // 16B
	iv     []byte // 16B
}

// m115GenerateKeyPair 生成临时 P-256 密钥对；返回私钥与 hex 公钥（04||X||Y，服务端要求的形态）。
func m115GenerateKeyPair() (*ecdh.PrivateKey, string, error) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}
	pub := priv.PublicKey().Bytes() // 0x04 || X || Y（65B uncompressed）
	return priv, hex.EncodeToString(pub), nil
}

// m115DeriveKeys 从双方公钥派生 AES key/IV：ECDH → X 坐标 sha256 → 前 16 key / 后 16 iv。
func m115DeriveKeys(priv *ecdh.PrivateKey, serverPubHex string) (*m115Keys, error) {
	serverPubHex = trimHexPrefix(serverPubHex)
	raw, err := hex.DecodeString(serverPubHex)
	if err != nil || len(raw) != 65 || raw[0] != 0x04 {
		return nil, fmt.Errorf("m115: 非法服务端公钥")
	}
	// crypto/ecdh 只吃自己的公钥类型：从 uncompressed 点重建。
	serverPub, err := ecdh.P256().NewPublicKey(raw)
	if err != nil {
		return nil, err
	}
	shared, err := priv.ECDH(serverPub)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(shared)
	return &m115Keys{aesKey: sum[:16], iv: sum[16:32]}, nil
}

// m115Encrypt AES-CBC 加密 + PKCS7 填充（请求 data 字段）。
func m115Encrypt(k *m115Keys, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(k.aesKey)
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, k.iv).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out), nil
}

// m115Decrypt base64 → AES-CBC 解密 → 去 PKCS7（响应 data 字段）。
func m115Decrypt(k *m115Keys, dataB64 string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("m115: 密文长度非法")
	}
	block, err := aes.NewCipher(k.aesKey)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, k.iv).CryptBlocks(out, raw)
	return pkcs7Unpad(out)
}

// pkcs7Pad / pkcs7Unpad 标准 PKCS#7。
func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("pkcs7: 空数据")
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > len(data) || pad > aes.BlockSize {
		return nil, fmt.Errorf("pkcs7: 非法填充")
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return nil, fmt.Errorf("pkcs7: 非法填充字节")
		}
	}
	return data[:len(data)-pad], nil
}

// trimHexPrefix 去掉服务端偶尔带的 "04:" / "0x" 前缀形态。
func trimHexPrefix(s string) string {
	if len(s) > 3 && (s[:3] == "04:") {
		return s[3:]
	}
	if len(s) > 2 && s[:2] == "0x" {
		return s[2:]
	}
	return s
}

// m115EncodeUInt32 LE 写 4 字节（部分契约里 data 前缀长度字段；保留工具）。

// m115Sum 共享秘密 → sha256（测试对拍双端派生用）。
func m115Sum(shared []byte) [32]byte {
	return sha256.Sum256(shared)
}

var _ = big.NewInt // 保留 import 位（曲线标量工具备用）
