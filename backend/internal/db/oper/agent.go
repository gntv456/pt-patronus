package oper

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// AgentOper 封装 Agent 的 CRUD 与 per-agent secret 鉴权（push 架构：agent WS 上报）。
type AgentOper struct {
	db *gorm.DB
}

// NewAgentOper 构造。
func NewAgentOper(db *gorm.DB) *AgentOper { return &AgentOper{db: db} }

// WithTx 返回绑定到指定事务的副本，用于跨表原子写（如 Agent 与 MonitorTarget 同生共死）。
// 不改变原 oper；调用方负责 tx 的提交/回滚（通常包在 db.Transaction 回调里）。
func (o *AgentOper) WithTx(tx *gorm.DB) *AgentOper { return &AgentOper{db: tx} }

// AgentSecretResult 创建/重置结果：明文 secret 仅此一次返回给前端展示，之后服务端不留存。
type AgentSecretResult struct {
	Agent       *models.Agent
	PlainSecret string
}

// Create 新建 agent + 关联 target，生成随机 secret（bcrypt 哈希存储），返回明文一次。
func (o *AgentOper) Create(name string, targetID uint) (AgentSecretResult, error) {
	secret, err := generateAgentSecret()
	if err != nil {
		return AgentSecretResult{}, err
	}
	hash, err := security.HashPassword(secret)
	if err != nil {
		return AgentSecretResult{}, err
	}
	a := &models.Agent{
		Name: name, SecretHash: hash, SecretHint: secretHint(secret),
		Enabled: true, TargetID: targetID,
	}
	if err := o.db.Create(a).Error; err != nil {
		return AgentSecretResult{}, err
	}
	return AgentSecretResult{Agent: a, PlainSecret: secret}, nil
}

// ResetSecret 重置 secret（泄露后应急），返回新明文。
func (o *AgentOper) ResetSecret(id uint) (string, error) {
	secret, err := generateAgentSecret()
	if err != nil {
		return "", err
	}
	hash, err := security.HashPassword(secret)
	if err != nil {
		return "", err
	}
	res := o.db.Model(&models.Agent{}).Where("id = ?", id).
		Updates(map[string]any{"secret_hash": hash, "secret_hint": secretHint(secret)})
	if res.Error != nil {
		return "", res.Error
	}
	// 一行都没改 = 这个被控端不存在：必须失败，不能把明文密钥当「轮转成功」发出去。
	// 否则界面显示已轮转、用户复制的是个谁也用不上的密钥，而真正那台 agent 的密钥纹丝未动。
	if res.RowsAffected == 0 {
		return "", gorm.ErrRecordNotFound
	}
	return secret, nil
}

// List 列出全部 agent（SecretHash 标 json:"-" 不会泄露）。
func (o *AgentOper) List() ([]models.Agent, error) {
	var as []models.Agent
	err := o.db.Order("id asc").Find(&as).Error
	return as, err
}

// Get 取单个。
func (o *AgentOper) Get(id uint) (*models.Agent, error) {
	var a models.Agent
	err := o.db.First(&a, id).Error
	return &a, err
}

// Delete 删除（不级联 target；target 是否一并删由调用方决定）。
func (o *AgentOper) Delete(id uint) error {
	return o.db.Delete(&models.Agent{}, id).Error
}

// SetEnabled 启用/禁用（禁用后 Authenticate 即拒）。
func (o *AgentOper) SetEnabled(id uint, enabled bool) error {
	return o.db.Model(&models.Agent{}).Where("id = ?", id).Update("enabled", enabled).Error
}

// Authenticate 按 agent_id + secret 双因子校验（bcrypt 比对，防越权枚举）。失败返回错误。
func (o *AgentOper) Authenticate(agentID uint, secret string) (*models.Agent, error) {
	var a models.Agent
	if err := o.db.First(&a, agentID).Error; err != nil {
		return nil, err
	}
	if !a.Enabled {
		return nil, errors.New("agent disabled")
	}
	if !security.VerifyPassword(a.SecretHash, secret) {
		return nil, errors.New("bad agent secret")
	}
	return &a, nil
}

// TouchSeen 更新最后在线时间与在线状态；version/hostname/ip 非空才更新（WS 收到消息/断开时调用）。
func (o *AgentOper) TouchSeen(id uint, online bool, version, hostname, ip string) error {
	updates := map[string]any{"last_seen_at": time.Now(), "online": online}
	if version != "" {
		updates["version"] = version
	}
	if hostname != "" {
		updates["hostname"] = hostname
	}
	if ip != "" {
		updates["ip"] = ip
	}
	return o.db.Model(&models.Agent{}).Where("id = ?", id).Updates(updates).Error
}

// generateAgentSecret 生成 32 字节随机 secret（base64.RawURLEncoding，无填充），仿 auth/ticket.go:37。
func generateAgentSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// secretHint 取明文前 8 位作提示（不泄露完整 secret）。
func secretHint(secret string) string {
	if len(secret) > 8 {
		return secret[:8]
	}
	return secret
}
