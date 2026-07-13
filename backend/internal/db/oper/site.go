// Package oper 实现数据访问层（Oper 模式）：每个领域一个操作类，封装 GORM 事务
// 与敏感字段的加解密。借鉴 MoviePilot 的 Oper 模式。
package oper

import (
	"errors"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// ErrDuplicateSite 同名 + 同 base_url 的站点已存在（防重复添加致仪表盘聚合双计）。
var ErrDuplicateSite = errors.New("同名同地址的站点已存在")

// SiteOper 封装 Site 表的 CRUD 与密钥加解密。
type SiteOper struct {
	db *gorm.DB
	c  *security.Crypto
}

// NewSiteOper 构造 SiteOper。
func NewSiteOper(db *gorm.DB, c *security.Crypto) *SiteOper {
	return &SiteOper{db: db, c: c}
}

// DecryptedPasskey 解密站点 PasskeyEnc；空或解密失败返回 ""（辅种拼下载链接用）。
func (o *SiteOper) DecryptedPasskey(s *models.Site) string {
	if s == nil || s.PasskeyEnc == "" {
		return ""
	}
	pk, err := o.c.Decrypt(s.PasskeyEnc)
	if err != nil {
		return ""
	}
	return pk
}

// SiteInput 是创建/更新站点的入参（密钥为明文，由 Oper 加密入库）。
type SiteInput struct {
	Name          string `json:"name" binding:"required,min=1,max=64"`
	Platform      string `json:"platform" binding:"required,min=1,max=32"`
	Definition    string `json:"definition" binding:"max=64"`
	BaseURL       string `json:"base_url" binding:"required,min=1,max=512"`
	Cookie        string `json:"cookie" binding:"max=65536"`
	APIKey        string `json:"api_key" binding:"max=256"`
	Passkey       string `json:"passkey" binding:"max=128"`
	Username      string `json:"username" binding:"max=64"`  // 站点账号（财神等 API 登录用）
	Password      string `json:"password" binding:"max=128"` // 站点密码（明文，由 Oper 加密入库）
	IsActive      bool   `json:"is_active"`
	Priority      int    `json:"priority"`
	LimitInterval int    `json:"limit_interval"`
}

// Create 创建站点。
func (o *SiteOper) Create(in SiteInput) (*models.Site, error) {
	// 防重复添加（同名 + 同 base_url）：重复站点会在仪表盘聚合里被双计（上传/做种翻倍）。
	// 批量导入 CreateMany 不走此校验，保留其「跳过单条错误」的恢复语义。
	var cnt int64
	if err := o.db.Model(&models.Site{}).Where("name = ? AND base_url = ?", in.Name, in.BaseURL).Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt > 0 {
		return nil, ErrDuplicateSite
	}
	return o.createOnDB(o.db, in)
}

// createOnDB 在给定 db（o.db 或事务 tx）上创建站点，复用加密 + IsActive 兜底逻辑。
func (o *SiteOper) createOnDB(db *gorm.DB, in SiteInput) (*models.Site, error) {
	s := &models.Site{
		Name: in.Name, Platform: in.Platform, Definition: in.Definition,
		BaseURL: in.BaseURL, Username: in.Username, IsActive: in.IsActive, Priority: in.Priority,
		LimitInterval: in.LimitInterval,
	}
	if err := o.encryptSecrets(s, in); err != nil {
		return nil, err
	}
	if err := db.Create(s).Error; err != nil {
		return nil, err
	}
	// GORM default:true 会把 IsActive=false 当零值吞为 true（导入 PT-Depiler isOffline
	// 站时本应停用却变启用）→ 显式写回。Update 走 Save 写全字段，不受此影响。
	if !in.IsActive {
		if err := db.Model(s).Update("is_active", false).Error; err != nil {
			return nil, err
		}
	}
	return s, nil
}

// CreateMany 单事务批量创建站点。逐条 INSERT 以保留「单条失败跳过、其余继续」语义，
// 但全部包在一个事务内——把 N 次 fsync 压成 1 次（慢盘上 200 站从数十秒降到 <1s）。
// 单条错误（极罕见，Site 无唯一约束）跳过不计；仅事务级失败（ctx 取消 / 磁盘满）整体回滚。
// 返回成功创建的条数。
func (o *SiteOper) CreateMany(ins []SiteInput) (int, error) {
	if len(ins) == 0 {
		return 0, nil
	}
	n := 0
	err := o.db.Transaction(func(tx *gorm.DB) error {
		for _, in := range ins {
			if _, e := o.createOnDB(tx, in); e != nil {
				continue
			}
			n++
		}
		return nil
	})
	return n, err
}

// Update 更新站点（密钥为空则保留原值）。
func (o *SiteOper) Update(id uint, in SiteInput) (*models.Site, error) {
	s, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	s.Name = in.Name
	s.Platform = in.Platform
	s.Definition = in.Definition
	s.BaseURL = in.BaseURL
	s.Username = in.Username
	s.IsActive = in.IsActive
	s.Priority = in.Priority
	s.LimitInterval = in.LimitInterval
	if err := o.encryptSecrets(s, in); err != nil {
		return nil, err
	}
	if err := o.db.Save(s).Error; err != nil {
		return nil, err
	}
	return s, nil
}

// List 列出全部站点（密钥字段 json:"-" 不外泄）。
func (o *SiteOper) List() ([]models.Site, error) {
	var sites []models.Site
	if err := o.db.Order("priority desc, id asc").Find(&sites).Error; err != nil {
		return nil, err
	}
	return sites, nil
}

// ListActive 列出启用的站点。
func (o *SiteOper) ListActive() ([]models.Site, error) {
	var sites []models.Site
	if err := o.db.Where("is_active = ?", true).Order("priority desc, id asc").Find(&sites).Error; err != nil {
		return nil, err
	}
	return sites, nil
}

// Get 取单个站点。
func (o *SiteOper) Get(id uint) (*models.Site, error) {
	var s models.Site
	if err := o.db.First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// Delete 删除站点。
func (o *SiteOper) Delete(id uint) error {
	return o.db.Delete(&models.Site{}, id).Error
}

// Cookie 解密返回站点 cookie。
func (o *SiteOper) Cookie(s *models.Site) (string, error) {
	return o.c.Decrypt(s.CookieEnc)
}

// UpdateCookie 仅更新站点 cookie（WebView 登录后上传用）。
func (o *SiteOper) UpdateCookie(id uint, cookie string) error {
	s, err := o.Get(id)
	if err != nil {
		return err
	}
	enc, err := o.c.Encrypt(cookie)
	if err != nil {
		return err
	}
	s.CookieEnc = enc
	return o.db.Save(s).Error
}

// APIKey 解密返回站点 API key。
func (o *SiteOper) APIKey(s *models.Site) (string, error) {
	return o.c.Decrypt(s.APIKeyEnc)
}

// Password 解密返回站点密码（财神 API 登录用）。
func (o *SiteOper) Password(s *models.Site) (string, error) {
	return o.c.Decrypt(s.PasswordEnc)
}

// encryptSecrets 把明文密钥加密写入 s；空值保留原密文（用于更新）。
func (o *SiteOper) encryptSecrets(s *models.Site, in SiteInput) error {
	if in.Cookie != "" {
		enc, err := o.c.Encrypt(in.Cookie)
		if err != nil {
			return err
		}
		s.CookieEnc = enc
	}
	if in.APIKey != "" {
		enc, err := o.c.Encrypt(in.APIKey)
		if err != nil {
			return err
		}
		s.APIKeyEnc = enc
	}
	if in.Passkey != "" {
		enc, err := o.c.Encrypt(in.Passkey)
		if err != nil {
			return err
		}
		s.PasskeyEnc = enc
	}
	if in.Password != "" {
		enc, err := o.c.Encrypt(in.Password)
		if err != nil {
			return err
		}
		s.PasswordEnc = enc
	}
	return nil
}
