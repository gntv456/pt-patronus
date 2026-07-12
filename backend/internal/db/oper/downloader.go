package oper

import (
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// DownloaderOper 封装 Downloader 表的 CRUD 与密码加解密。
type DownloaderOper struct {
	db *gorm.DB
	c  *security.Crypto
}

// NewDownloaderOper 构造。
func NewDownloaderOper(db *gorm.DB, c *security.Crypto) *DownloaderOper {
	return &DownloaderOper{db: db, c: c}
}

// DownloaderInput 创建/更新下载器入参（密码明文，由 Oper 加密）。
type DownloaderInput struct {
	Name      string `json:"name" binding:"max=64"`
	Type      string `json:"type" binding:"max=16"` // qb | tr
	URL       string `json:"url" binding:"max=255"`
	Username  string `json:"username" binding:"max=64"`
	Password  string `json:"password" binding:"max=512"`
	IsDefault bool   `json:"is_default"`
}

// Create 创建下载器。
func (o *DownloaderOper) Create(in DownloaderInput) (*models.Downloader, error) {
	if in.URL != "" {
		if err := validateURLScheme(in.URL); err != nil {
			return nil, err
		}
	}
	d := &models.Downloader{Name: in.Name, Type: in.Type, URL: in.URL, Username: in.Username, IsDefault: in.IsDefault}
	if in.Password != "" {
		enc, err := o.c.Encrypt(in.Password)
		if err != nil {
			return nil, err
		}
		d.PasswordEnc = enc
	}
	// 默认下载器唯一：设新默认时事务内清掉旧的，避免多个 is_default=true 共存。
	if err := o.db.Transaction(func(tx *gorm.DB) error {
		if in.IsDefault {
			if e := tx.Model(&models.Downloader{}).Where("is_default = ?", true).
				Update("is_default", false).Error; e != nil {
				return e
			}
		}
		return tx.Create(d).Error
	}); err != nil {
		return nil, err
	}
	return d, nil
}

// Update 更新下载器（密码为空则保留）。
func (o *DownloaderOper) Update(id uint, in DownloaderInput) (*models.Downloader, error) {
	d, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	if in.URL != "" {
		if err := validateURLScheme(in.URL); err != nil {
			return nil, err
		}
	}
	d.Name, d.Type, d.URL, d.Username, d.IsDefault = in.Name, in.Type, in.URL, in.Username, in.IsDefault
	if in.Password != "" {
		enc, err := o.c.Encrypt(in.Password)
		if err != nil {
			return nil, err
		}
		d.PasswordEnc = enc
	}
	// 默认下载器唯一：设为默认时事务内清掉其他默认（WHERE id<>自身），Save 后自身为 true。
	if err := o.db.Transaction(func(tx *gorm.DB) error {
		if in.IsDefault {
			if e := tx.Model(&models.Downloader{}).Where("is_default = ? AND id <> ?", true, id).
				Update("is_default", false).Error; e != nil {
				return e
			}
		}
		return tx.Save(d).Error
	}); err != nil {
		return nil, err
	}
	return d, nil
}

// List 列出全部下载器。
func (o *DownloaderOper) List() ([]models.Downloader, error) {
	var ds []models.Downloader
	if err := o.db.Order("id asc").Find(&ds).Error; err != nil {
		return nil, err
	}
	return ds, nil
}

// Get 取单个下载器。
func (o *DownloaderOper) Get(id uint) (*models.Downloader, error) {
	var d models.Downloader
	if err := o.db.First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// Delete 删除下载器。
func (o *DownloaderOper) Delete(id uint) error {
	return o.db.Delete(&models.Downloader{}, id).Error
}

// Password 解密返回密码。
func (o *DownloaderOper) Password(d *models.Downloader) (string, error) {
	return o.c.Decrypt(d.PasswordEnc)
}
