package oper

import (
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// FileStorageOper 封装 FileStorage 表的 CRUD 与密码加解密。
type FileStorageOper struct {
	db *gorm.DB
	c  *security.Crypto
}

// NewFileStorageOper 构造。
func NewFileStorageOper(db *gorm.DB, c *security.Crypto) *FileStorageOper {
	return &FileStorageOper{db: db, c: c}
}

// FileStorageInput 创建/更新入参。
type FileStorageInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // webdav | alist | smb | agent
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	BasePath string `json:"base_path"`
	AgentID  uint   `json:"agent_id"` // type=agent 时关联 Agent；其他类型忽略
}

// Create 创建。
func (o *FileStorageOper) Create(in FileStorageInput) (*models.FileStorage, error) {
	if err := validateTypeFields(in); err != nil {
		return nil, err
	}
	if err := validateForType(in.Type, in.URL); err != nil {
		return nil, err
	}
	f := &models.FileStorage{Name: in.Name, Type: in.Type, URL: in.URL, Username: in.Username, BasePath: in.BasePath, AgentID: in.AgentID}
	if in.Password != "" {
		enc, err := o.c.Encrypt(in.Password)
		if err != nil {
			return nil, err
		}
		f.PasswordEnc = enc
	}
	if err := o.db.Create(f).Error; err != nil {
		return nil, err
	}
	return f, nil
}

// Update 更新（密码为空保留）。
func (o *FileStorageOper) Update(id uint, in FileStorageInput) (*models.FileStorage, error) {
	f, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	if err := validateTypeFields(in); err != nil {
		return nil, err
	}
	if err := validateForType(in.Type, in.URL); err != nil {
		return nil, err
	}
	f.Name, f.Type, f.URL, f.Username, f.BasePath, f.AgentID = in.Name, in.Type, in.URL, in.Username, in.BasePath, in.AgentID
	if in.Password != "" {
		enc, err := o.c.Encrypt(in.Password)
		if err != nil {
			return nil, err
		}
		f.PasswordEnc = enc
	}
	if err := o.db.Save(f).Error; err != nil {
		return nil, err
	}
	return f, nil
}

// List 列出全部。
func (o *FileStorageOper) List() ([]models.FileStorage, error) {
	var fs []models.FileStorage
	if err := o.db.Order("id asc").Find(&fs).Error; err != nil {
		return nil, err
	}
	return fs, nil
}

// Get 取单个。
func (o *FileStorageOper) Get(id uint) (*models.FileStorage, error) {
	var f models.FileStorage
	if err := o.db.First(&f, id).Error; err != nil {
		return nil, err
	}
	return &f, nil
}

// Delete 删除。
func (o *FileStorageOper) Delete(id uint) error {
	return o.db.Delete(&models.FileStorage{}, id).Error
}

// Password 解密返回密码。
func (o *FileStorageOper) Password(f *models.FileStorage) (string, error) {
	return o.c.Decrypt(f.PasswordEnc)
}

// validateTypeFields 按存储类型校验必填字段：agent 必须关联 agent_id；webdav/alist/smb 必须填地址。
func validateTypeFields(in FileStorageInput) error {
	switch in.Type {
	case "agent":
		if in.AgentID == 0 {
			return fmt.Errorf("agent 存储必须关联一个 agent")
		}
	case "webdav", "alist", "smb":
		if strings.TrimSpace(in.URL) == "" {
			return fmt.Errorf("%s 存储必须填写地址", in.Type)
		}
	default:
		return fmt.Errorf("未知存储类型: %s", in.Type)
	}
	return nil
}
