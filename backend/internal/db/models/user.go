// Code split from models.go. Package models defines GORM models for the persistence layer.
package models

import "time"

// User — phase-0 single admin；登录改造后以好学外部身份(HxptUID)为准，本地密码登录弃用。
type User struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Username      string     `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash  string     `gorm:"size:128" json:"-"` // 本地密码登录弃用，保留列兼容老数据
	IsAdmin       bool       `gorm:"default:false" json:"is_admin"`
	Level         int        `gorm:"default:0" json:"level"`        // 与站点认证等级配合
	HxptUID       string     `gorm:"size:64;index" json:"hxpt_uid"` // 好学用户 ID（外部身份，登录 upsert 依据；非唯一索引避空值约束坑）
	HxptUsername  string     `gorm:"size:64" json:"hxpt_username"`
	VipLevel      int        `gorm:"default:0" json:"vip_level"` // 好学 APP 专属 VIP 等级快照
	VipVerifiedAt *time.Time `json:"vip_verified_at"`            // 上次 VIP 校验成功时间（nil=未验证）
	TokenVersion  uint       `gorm:"default:0" json:"-"`         // 登出 bump → 旧 JWT 失效
	HxptCookieEnc string     `gorm:"type:text" json:"-"`         // AES-GCM 加密的好学 cookie（VIP 重验用）
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
