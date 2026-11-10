// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// CloudDriveOper 封装 CloudDrive 表的 CRUD 与凭据（cookie/accessToken）加解密。
// 与 MediaServerOper 同构：Token 明文入参，由 Oper 加密落库；空串=保留旧值（UI 不重输秘密时回填）。
type CloudDriveOper struct {
	db *gorm.DB
	c  *security.Crypto
}

func NewCloudDriveOper(db *gorm.DB, c *security.Crypto) *CloudDriveOper {
	return &CloudDriveOper{db: db, c: c}
}

// CloudDriveInput 创建/更新网盘入参（Token 明文，由 Oper 加密）。
type CloudDriveInput struct {
	Name      string `json:"name" binding:"max=64"`
	Type      string `json:"type" binding:"max=16"`    // 115 | 115_open | quark | 123 | aliyun_open | onedrive | baidu | tianyi | mobile
	Token     string `json:"token" binding:"max=8192"` // cookie(115/夸克) 或 accessToken；115_open/aliyun_open 为含 refresh 的 JSON 对，体积较大
	RootPath  string `json:"root_path" binding:"max=255"`
	IsDefault bool   `json:"is_default"`
}

// validCloudDriveType 网盘类型白名单（binding max 不够，需显式枚举校验）。
// 必须与 cloud.NewClient 支持的类型、前端 CloudDialogs 下拉保持一致，
// 否则 UI 可选类型在保存层被 400 拒绝（功能断链）。
func validCloudDriveType(t string) bool {
	switch t {
	case "115", "115_open", "quark", "123", "aliyun_open", "onedrive", "baidu", "tianyi", "mobile":
		return true
	}
	return false
}

func (o *CloudDriveOper) Create(in CloudDriveInput) (*models.CloudDrive, error) {
	if !validCloudDriveType(in.Type) {
		return nil, errInvalidCloudType
	}
	m := &models.CloudDrive{
		Name: in.Name, Type: in.Type, RootPath: in.RootPath, IsDefault: in.IsDefault,
	}
	if in.Token != "" {
		enc, err := o.c.Encrypt(in.Token)
		if err != nil {
			return nil, err
		}
		m.TokenEnc = enc
	}
	if err := o.db.Create(m).Error; err != nil {
		return nil, err
	}
	return m, nil
}

// Update 更新网盘（Token 空串=保留旧值）。
func (o *CloudDriveOper) Update(id uint, in CloudDriveInput) (*models.CloudDrive, error) {
	if !validCloudDriveType(in.Type) {
		return nil, errInvalidCloudType
	}
	m, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	m.Name, m.Type, m.RootPath, m.IsDefault = in.Name, in.Type, in.RootPath, in.IsDefault
	if in.Token != "" {
		enc, err := o.c.Encrypt(in.Token)
		if err != nil {
			return nil, err
		}
		m.TokenEnc = enc
	}
	if err := o.db.Save(m).Error; err != nil {
		return nil, err
	}
	return m, nil
}

func (o *CloudDriveOper) List() ([]models.CloudDrive, error) {
	var ds []models.CloudDrive
	if err := o.db.Order("id asc").Find(&ds).Error; err != nil {
		return nil, err
	}
	return ds, nil
}

func (o *CloudDriveOper) Get(id uint) (*models.CloudDrive, error) {
	var m models.CloudDrive
	if err := o.db.First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// ListEnabled 列出全部网盘（STRM 追更遍历用；当前不区分 enabled，网盘本身无 enabled 开关）。
func (o *CloudDriveOper) ListAll() ([]models.CloudDrive, error) {
	return o.List()
}

func (o *CloudDriveOper) Delete(id uint) error {
	return o.db.Delete(&models.CloudDrive{}, id).Error
}

// WithTx 事务内换绑 db（CloudChain.Delete 级联三删共用，见 chain/cloud.go）。
func (o *CloudDriveOper) WithTx(tx *gorm.DB) *CloudDriveOper { return &CloudDriveOper{db: tx, c: o.c} }

// DB 暴露底层 db 供 chain 层开事务（级联删除原子性）。
func (o *CloudDriveOper) DB() *gorm.DB { return o.db }

// Token 解密返回凭据（cookie/accessToken）。
func (o *CloudDriveOper) Token(m *models.CloudDrive) (string, error) {
	return o.c.Decrypt(m.TokenEnc)
}

// SetToken 加密回写凭据（token 刷新轮换用；不触碰其它字段）。
func (o *CloudDriveOper) SetToken(id uint, token string) error {
	enc, err := o.c.Encrypt(token)
	if err != nil {
		return err
	}
	return o.db.Model(&models.CloudDrive{}).Where("id = ?", id).
		Update("token_enc", enc).Error
}

// errInvalidCloudType 网盘类型不在 115/quark/123 白名单内（包 ErrInvalidInput 供 handler errors.Is 判 400）。
var errInvalidCloudType = invalidInput("无效的网盘类型（仅支持 115/quark/123）")
