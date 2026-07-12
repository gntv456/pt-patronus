package oper

import (
	"fmt"

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
	Type      string `json:"type" binding:"max=16"` // 115 | quark | 123
	Token     string `json:"token" binding:"max=2048"` // cookie(115/夸克) 或 accessToken(123)
	RootPath  string `json:"root_path" binding:"max=255"`
	IsDefault bool   `json:"is_default"`
}

// validCloudDriveType 收紧网盘类型白名单（binding max 不够，需显式枚举校验）。
func validCloudDriveType(t string) bool {
	switch t {
	case "115", "quark", "123":
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

// Token 解密返回凭据（cookie/accessToken）。
func (o *CloudDriveOper) Token(m *models.CloudDrive) (string, error) {
	return o.c.Decrypt(m.TokenEnc)
}

// errInvalidCloudType 网盘类型不在 115/quark/123 白名单内（包 ErrInvalidInput 供 handler errors.Is 判 400）。
var errInvalidCloudType = fmt.Errorf("无效的网盘类型（仅支持 115/quark/123）: %w", ErrInvalidInput)
