// Package cookiecloud 内置服务端（PTPatronus 自身充当 CookieCloud 服务器）。
//
// 官方协议语义（easychen/CookieCloud api/app.js）：
//   - POST /update：body {"uuid","encrypted","crypto_type"?}，成功返回 {"action":"done"}。
//     服务端只存密文（端到端加密，服务端无密码不解密），可附带 crypto_type 元数据。
//   - GET /get/{uuid}：返回 {"encrypted":"...","crypto_type":"..."}；未注册 uuid 404。
//
// 本文件是纯逻辑层：store + 请求解析/校验。HTTP/CORS/gzip 由 API handler 负责。
package cookiecloud

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// maxEncryptedLen 是单条 encrypted 密文上限（base64 字符数）。CookieCloud 正常负载
// <1MB；16MB 防异常大上传占内存（多设备全量 cookie 也在量级内）。
const maxEncryptedLen = 16 << 20

// ServerEnvelope 是一次存储的完整 envelope（密文 + 可选算法标记）。
type ServerEnvelope struct {
	Encrypted  string `json:"encrypted"`
	CryptoType string `json:"crypto_type,omitempty"`
}

// ServerStore 抽象密文 envelope 的存取（内存实现即可：重启后浏览器扩展会重推，
// 无需持久化；接口留给将来换持久实现）。
type ServerStore interface {
	Get(uuid string) (ServerEnvelope, bool)
	Put(uuid string, env ServerEnvelope)
}

// MemServerStore 是 ServerStore 的内存实现（mutex + map）。
type MemServerStore struct {
	mu   sync.RWMutex
	data map[string]ServerEnvelope
}

// NewMemServerStore 构造。
func NewMemServerStore() *MemServerStore {
	return &MemServerStore{data: map[string]ServerEnvelope{}}
}

// Get 取 envelope。
func (m *MemServerStore) Get(uuid string) (ServerEnvelope, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.data[uuid]
	return e, ok
}

// Put 存 envelope。
func (m *MemServerStore) Put(uuid string, env ServerEnvelope) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[uuid] = env
}

// UpdateRequest 是 POST /update 的 body。
type UpdateRequest struct {
	UUID       string `json:"uuid"`
	Encrypted  string `json:"encrypted"`
	CryptoType string `json:"crypto_type"`
}

// ParseUpdateRequest 解析并校验 /update body（rawBody 需已解压）。
// 校验：uuid/encrypted 非空、encrypted 是合法 base64、长度不超上限。
// 不解密内容——服务端无密码，解密在拉取端（ Decrypt 双格式自动识别）。
func ParseUpdateRequest(rawBody []byte) (UpdateRequest, error) {
	var req UpdateRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		return req, fmt.Errorf("invalid json: %w", err)
	}
	req.UUID = strings.TrimSpace(req.UUID)
	if req.UUID == "" {
		return req, fmt.Errorf("uuid is required")
	}
	req.Encrypted = strings.TrimSpace(req.Encrypted)
	if req.Encrypted == "" {
		return req, fmt.Errorf("encrypted is required")
	}
	if len(req.Encrypted) > maxEncryptedLen {
		return req, fmt.Errorf("encrypted too large")
	}
	if _, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.Encrypted)); err != nil {
		return req, fmt.Errorf("encrypted is not valid base64")
	}
	return req, nil
}

// MarshalEnvelope 把 envelope 序列化为 GET 响应 body。
func MarshalEnvelope(env ServerEnvelope) ([]byte, error) {
	return json.Marshal(env)
}
