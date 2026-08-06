package oper

import (
	"fmt"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/monitor"
	"gorm.io/gorm"
)

// MonitorOper 封装监控目标/快照/告警的 CRUD 与凭据加解密。
// 同时隐式实现 monitor.Store 接口（供 monitor.Service 依赖，避免循环引用）。
type MonitorOper struct {
	db *gorm.DB
	c  *security.Crypto
}

// NewMonitorOper 构造。
func NewMonitorOper(db *gorm.DB, c *security.Crypto) *MonitorOper {
	return &MonitorOper{db: db, c: c}
}

// WithTx 返回绑定到指定事务的副本（保留 Crypto 句柄），用于跨表原子写。
// 不改变原 oper；调用方负责 tx 的提交/回滚。
func (o *MonitorOper) WithTx(tx *gorm.DB) *MonitorOper {
	return &MonitorOper{db: tx, c: o.c}
}

// TargetInput 创建/更新监控目标入参（凭据明文，由 Oper 加密）。
type TargetInput struct {
	Name         string `json:"name" binding:"max=64"`
	Type         string `json:"type" binding:"max=16"` // host | nas | camera
	Host         string `json:"host" binding:"max=255"`
	Port         int    `json:"port"`
	Vendor       string `json:"vendor" binding:"max=32"`
	Protocol     string `json:"protocol" binding:"max=16"`
	CredentialID uint   `json:"credential_id"`
	RTSPURL      string `json:"rtsp_url" binding:"max=512"`
	IntervalSec  int    `json:"interval_sec"`
	TimeoutSec   int    `json:"timeout_sec"`
	Username     string `json:"username" binding:"max=256"`
	Password     string `json:"password" binding:"max=512"`
	PrivateKey   string `json:"private_key" binding:"max=8192"` // SSH 私钥 PEM（明文，Oper 加密；空=不改/不设）
	Thresholds   string `json:"thresholds" binding:"max=5000"`
	DiskPaths    string `json:"disk_paths" binding:"max=2000"` // host/nas+local：宿主 bind mount 目录（换行/逗号分隔）
	Enabled      bool   `json:"enabled"`
}

// Create 创建监控目标。
func (o *MonitorOper) Create(in TargetInput) (*models.MonitorTarget, error) {
	if err := validateTargetInput(in); err != nil {
		return nil, err
	}
	if in.RTSPURL != "" {
		if err := validateRTSPScheme(in.RTSPURL); err != nil {
			return nil, err
		}
	}
	t := &models.MonitorTarget{
		Name: in.Name, Type: in.Type, Host: in.Host, Port: in.Port,
		Vendor: in.Vendor, Protocol: in.Protocol, CredentialID: in.CredentialID, RTSPURL: in.RTSPURL,
		IntervalSec: normInterval(in.IntervalSec), TimeoutSec: normTimeout(in.TimeoutSec),
		Enabled: in.Enabled, Thresholds: in.Thresholds, DiskPaths: in.DiskPaths,
	}
	if err := o.ensureCredentialExists(in.CredentialID); err != nil {
		return nil, err
	}
	if err := o.setCreds(t, in.Username, in.Password, in.PrivateKey); err != nil {
		return nil, err
	}
	if err := o.db.Create(t).Error; err != nil {
		return nil, err
	}
	// GORM default:true 会把 Enabled=false 当零值吞为 true → 显式写回（同 SiteOper.Create）。
	if !in.Enabled {
		if err := o.db.Model(t).Update("enabled", false).Error; err != nil {
			return nil, err
		}
	}
	o.fillUsername(t)
	return t, nil
}

// Update 更新（密码/用户名为空则保留）。
func (o *MonitorOper) Update(id uint, in TargetInput) (*models.MonitorTarget, error) {
	t, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	if err := validateTargetInput(in); err != nil {
		return nil, err
	}
	if in.RTSPURL != "" {
		if err := validateRTSPScheme(in.RTSPURL); err != nil {
			return nil, err
		}
	}
	t.Name, t.Type, t.Host, t.Port = in.Name, in.Type, in.Host, in.Port
	t.Vendor, t.Protocol, t.CredentialID, t.RTSPURL = in.Vendor, in.Protocol, in.CredentialID, in.RTSPURL
	t.IntervalSec = normInterval(in.IntervalSec)
	t.TimeoutSec = normTimeout(in.TimeoutSec)
	t.Enabled, t.Thresholds, t.DiskPaths = in.Enabled, in.Thresholds, in.DiskPaths
	if err := o.ensureCredentialExists(in.CredentialID); err != nil {
		return nil, err
	}
	// 凭据加密失败必须中止（对齐 Create 的 setCreds）：否则保留旧密文、API 误报成功，
	// 用户改密码后监控仍用旧凭证反复鉴权失败。
	if in.Username != "" {
		enc, err := o.c.Encrypt(in.Username)
		if err != nil {
			return nil, err
		}
		t.UsernameEnc = enc
	}
	if in.Password != "" {
		enc, err := o.c.Encrypt(in.Password)
		if err != nil {
			return nil, err
		}
		t.PasswordEnc = enc
	}
	if in.PrivateKey != "" {
		enc, err := o.c.Encrypt(in.PrivateKey)
		if err != nil {
			return nil, err
		}
		t.PrivateKeyEnc = enc
	}
	if err := o.db.Save(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

func (o *MonitorOper) setCreds(t *models.MonitorTarget, username, password, privateKey string) error {
	if username != "" {
		enc, err := o.c.Encrypt(username)
		if err != nil {
			return err
		}
		t.UsernameEnc = enc
	}
	if password != "" {
		enc, err := o.c.Encrypt(password)
		if err != nil {
			return err
		}
		t.PasswordEnc = enc
	}
	if privateKey != "" {
		enc, err := o.c.Encrypt(privateKey)
		if err != nil {
			return err
		}
		t.PrivateKeyEnc = enc
	}
	return nil
}

// List 列出全部。
func (o *MonitorOper) List() ([]models.MonitorTarget, error) {
	var ts []models.MonitorTarget
	err := o.db.Order("id asc").Find(&ts).Error
	if err == nil {
		for i := range ts {
			o.fillUsername(&ts[i])
		}
	}
	return ts, err
}

// ListEnabled 列出启用的。
func (o *MonitorOper) ListEnabled() ([]models.MonitorTarget, error) {
	var ts []models.MonitorTarget
	err := o.db.Where("enabled = ?", true).Order("id asc").Find(&ts).Error
	return ts, err
}

// Get 取单个。
func (o *MonitorOper) Get(id uint) (*models.MonitorTarget, error) {
	var t models.MonitorTarget
	err := o.db.First(&t, id).Error
	if err == nil {
		o.fillUsername(&t)
	}
	return &t, err
}

// Delete 删除（连带删快照/告警，单事务保证不残留孤儿行；对齐 RssOper/WorkflowOper 写法）。
func (o *MonitorOper) Delete(id uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&models.MonitorTarget{}, id).Error; err != nil {
			return err
		}
		// 级联删快照/告警：错误必须返回，否则级联失败仍提交 → 孤儿行（外键破坏）。
		if err := tx.Where("target_id = ?", id).Delete(&models.MetricSnapshot{}).Error; err != nil {
			return err
		}
		return tx.Where("target_id = ?", id).Delete(&models.MonitorAlert{}).Error
	})
}

// Credentials 解密返回凭据（无凭据时字段为空，不报错）。
func (o *MonitorOper) Credentials(t *models.MonitorTarget) (monitor.Credentials, error) {
	creds := monitor.Credentials{}
	if t.CredentialID > 0 {
		var c models.OpsCredential
		if err := o.db.First(&c, t.CredentialID).Error; err != nil {
			return creds, err
		}
		if c.UsernameEnc != "" {
			if u, err := o.c.Decrypt(c.UsernameEnc); err == nil {
				creds.Username = u
			} else {
				return creds, err
			}
		}
		if c.PasswordEnc != "" {
			if p, err := o.c.Decrypt(c.PasswordEnc); err == nil {
				creds.Password = p
			} else {
				return creds, err
			}
		}
		if c.PrivateKeyEnc != "" {
			if k, err := o.c.Decrypt(c.PrivateKeyEnc); err == nil {
				creds.PrivateKey = k
			} else {
				return creds, err
			}
		}
		return creds, nil
	}
	if t.UsernameEnc != "" {
		if u, err := o.c.Decrypt(t.UsernameEnc); err == nil {
			creds.Username = u
		} else {
			return creds, err
		}
	}
	if t.PasswordEnc != "" {
		if p, err := o.c.Decrypt(t.PasswordEnc); err == nil {
			creds.Password = p
		} else {
			return creds, err
		}
	}
	if t.PrivateKeyEnc != "" {
		if k, err := o.c.Decrypt(t.PrivateKeyEnc); err == nil {
			creds.PrivateKey = k
		} else {
			return creds, err
		}
	}
	return creds, nil
}

// fillUsername 解密回填 transient Username 与 HasPrivateKey（仅供 JSON 回显，编辑表单用）；
// 明文密码/私钥不回传，编辑时留空保留。
func (o *MonitorOper) fillUsername(t *models.MonitorTarget) {
	if t.CredentialID > 0 {
		var c models.OpsCredential
		if err := o.db.First(&c, t.CredentialID).Error; err == nil {
			t.CredentialName = c.Name
			if c.UsernameEnc != "" {
				if u, err := o.c.Decrypt(c.UsernameEnc); err == nil {
					t.Username = u
				}
			}
			t.HasPrivateKey = c.PrivateKeyEnc != ""
			return
		}
	}
	if t.UsernameEnc != "" {
		if u, err := o.c.Decrypt(t.UsernameEnc); err == nil {
			t.Username = u
		}
	}
	t.HasPrivateKey = t.PrivateKeyEnc != ""
}

func (o *MonitorOper) ensureCredentialExists(id uint) error {
	if id == 0 {
		return nil
	}
	var n int64
	if err := o.db.Model(&models.OpsCredential{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("凭据不存在")
	}
	return nil
}
