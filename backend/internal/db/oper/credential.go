package oper

import (
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// CredentialOper manages shared ops credentials. Passwords and private keys are encrypted at rest.
type CredentialOper struct {
	db *gorm.DB
	c  *security.Crypto
}

func NewCredentialOper(db *gorm.DB, c *security.Crypto) *CredentialOper {
	return &CredentialOper{db: db, c: c}
}

type CredentialInput struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	PrivateKey string `json:"private_key"`
	Tags       string `json:"tags"`
	Note       string `json:"note"`
}

func (o *CredentialOper) List() ([]models.OpsCredential, error) {
	var out []models.OpsCredential
	err := o.db.Order("id asc").Find(&out).Error
	if err == nil {
		for i := range out {
			o.fill(&out[i])
		}
	}
	return out, err
}

func (o *CredentialOper) Get(id uint) (*models.OpsCredential, error) {
	var c models.OpsCredential
	if err := o.db.First(&c, id).Error; err != nil {
		return nil, err
	}
	o.fill(&c)
	return &c, nil
}

func (o *CredentialOper) Create(in CredentialInput) (*models.OpsCredential, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("凭据名称不能为空")
	}
	c := &models.OpsCredential{
		Name: strings.TrimSpace(in.Name),
		Type: normCredentialType(in.Type),
		Tags: strings.TrimSpace(in.Tags),
		Note: strings.TrimSpace(in.Note),
	}
	if err := o.setSecrets(c, in.Username, in.Password, in.PrivateKey, true); err != nil {
		return nil, err
	}
	if err := o.db.Create(c).Error; err != nil {
		return nil, err
	}
	o.fill(c)
	return c, nil
}

func (o *CredentialOper) Update(id uint, in CredentialInput) (*models.OpsCredential, error) {
	c, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("凭据名称不能为空")
	}
	c.Name = strings.TrimSpace(in.Name)
	c.Type = normCredentialType(in.Type)
	c.Tags = strings.TrimSpace(in.Tags)
	c.Note = strings.TrimSpace(in.Note)
	if err := o.setSecrets(c, in.Username, in.Password, in.PrivateKey, false); err != nil {
		return nil, err
	}
	if err := o.db.Save(c).Error; err != nil {
		return nil, err
	}
	o.fill(c)
	return c, nil
}

func (o *CredentialOper) Delete(id uint) error {
	n, err := o.UsageCount(id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("该凭据正被 %d 台设备使用，请先解绑", n)
	}
	return o.db.Delete(&models.OpsCredential{}, id).Error
}

func (o *CredentialOper) UsageCount(id uint) (int64, error) {
	var n int64
	err := o.db.Model(&models.MonitorTarget{}).Where("credential_id = ?", id).Count(&n).Error
	return n, err
}

func (o *CredentialOper) setSecrets(c *models.OpsCredential, username, password, privateKey string, create bool) error {
	if create || username != "" {
		if username == "" {
			c.UsernameEnc = ""
		} else {
			enc, err := o.c.Encrypt(username)
			if err != nil {
				return err
			}
			c.UsernameEnc = enc
		}
	}
	if password != "" {
		enc, err := o.c.Encrypt(password)
		if err != nil {
			return err
		}
		c.PasswordEnc = enc
	}
	if privateKey != "" {
		enc, err := o.c.Encrypt(privateKey)
		if err != nil {
			return err
		}
		c.PrivateKeyEnc = enc
	}
	return nil
}

func (o *CredentialOper) fill(c *models.OpsCredential) {
	if c.UsernameEnc != "" {
		if u, err := o.c.Decrypt(c.UsernameEnc); err == nil {
			c.Username = u
		}
	}
	c.HasPassword = c.PasswordEnc != ""
	c.HasPrivateKey = c.PrivateKeyEnc != ""
}

func normCredentialType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "nas":
		return "nas"
	case "generic":
		return "generic"
	default:
		return "ssh"
	}
}
