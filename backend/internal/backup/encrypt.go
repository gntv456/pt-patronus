// 备份文件口令加密（安全审计：备份导出含全量明文 cookie/passkey/密码，
// 此前导出文件本身不加密）。格式：JSON 信封
//
//	{"ptpatronus_backup_enc":1,"kdf":"pbkdf2-sha256-310000","salt":…,"iv":…,"data":…}
//
// 口令 PBKDF2-SHA256×310k 派生 AES-256-GCM 密钥（与 TOTP/凭据库同强度）。
// 未带口令导出保持旧行为（明文 JSON），由前端导出弹窗引导用户填口令。
package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

const backupEncVersion = 1

// backupEncEnvelope 加密封包结构。
type backupEncEnvelope struct {
	Magic int    `json:"ptpatronus_backup_enc"`
	KDF   string `json:"kdf"`
	Salt  string `json:"salt"`
	IV    string `json:"iv"`
	Data  string `json:"data"`
}

const kdfID = "pbkdf2-sha256-310000"

// ErrBadPassword 口令错误（GCM 认证失败）。
var ErrBadPassword = errors.New("backup password incorrect")

// Encrypt 用口令加密任意字节（内部 JSON、未来任意格式通用）。
func Encrypt(data []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, errors.New("empty backup password")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := deriveBackupKey(password, salt)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, iv, data, nil)
	b64 := base64.StdEncoding.EncodeToString
	return json.Marshal(backupEncEnvelope{
		Magic: backupEncVersion, KDF: kdfID,
		Salt: b64(salt), IV: b64(iv), Data: b64(ct),
	})
}

// Decrypt 解开封包；非加密 JSON 原样透传（导入路径兼容旧明文备份）。
func Decrypt(data []byte, password string) ([]byte, error) {
	var probe json.RawMessage
	if json.Unmarshal(data, &probe) != nil {
		return data, nil // 非 JSON：原样透传交上层解析报错
	}
	var env backupEncEnvelope
	if json.Unmarshal(probe, &env) != nil || env.Magic != backupEncVersion {
		return data, nil // 非加密信封：明文备份透传
	}
	if password == "" {
		return nil, ErrPasswordRequired
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return nil, fmt.Errorf("bad salt: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(env.IV)
	if err != nil {
		return nil, fmt.Errorf("bad iv: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(env.Data)
	if err != nil {
		return nil, fmt.Errorf("bad data: %w", err)
	}
	key, err := deriveBackupKey(password, salt)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, iv, ct, nil)
	if err != nil {
		return nil, ErrBadPassword
	}
	return pt, nil
}

// NeedsPassword 判断备份体是否为加密信封（导入前路由提示用）。
func NeedsPassword(data []byte) bool {
	var env backupEncEnvelope
	if json.Unmarshal(data, &env) != nil {
		return false
	}
	return env.Magic == backupEncVersion
}

// ErrPasswordRequired 加密备份缺口令。
var ErrPasswordRequired = errors.New("backup password required")

// deriveBackupKey PBKDF2-SHA256（310k 轮）派生 32 字节 AES 密钥——
// 与浏览器扩展 TOTP/凭据库同款参数（跨端一致，便于未来互通）。
func deriveBackupKey(password string, salt []byte) ([]byte, error) {
	if len(salt) == 0 {
		return nil, errors.New("empty salt")
	}
	return pbkdf2.Key([]byte(password), salt, 310000, 32, sha256.New), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// 恒定时间比较（预留：未来加口令校验字段用）。
func ctEqual(a, b []byte) bool { return subtle.ConstantTimeCompare(a, b) == 1 }
