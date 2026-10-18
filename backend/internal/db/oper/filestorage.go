package oper

import (
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
// Password 长度：rclone 场景承载整份 rclone.conf INI 原文（多 remote 段+token，可达数十 KB），
// 上限放宽到 64KB；其余类型沿用 512。操作层另按类型复核（validatePasswordLength），
// 避免"rclone 放宽"被挪用到 webdav 等凭据字段。
type FileStorageInput struct {
	Name     string `json:"name" binding:"max=64"`
	Type     string `json:"type" binding:"max=16"` // webdav | alist | smb | agent | local | rclone | 115 | quark | 123
	URL      string `json:"url" binding:"max=255"`
	Username string `json:"username" binding:"max=64"`
	Password string `json:"password" binding:"max=65536"`
	BasePath string `json:"base_path" binding:"max=255"`
	AgentID  uint   `json:"agent_id"` // type=agent 时关联 Agent；其他类型忽略
}

// validatePasswordLength 按 2 类型复核密码长度：rclone.conf 原文 ≤64KB；其余 ≤512。
// 绑定层 max=65536 仅兜底 rclone，此函数保证 webdav/smb 等凭据仍受 512 约束。
func validatePasswordLength(in FileStorageInput) error {
	max := 512
	if in.Type == "rclone" {
		max = 65536
	}
	if len(in.Password) > max {
		return invalidInput("%s 存储密码/配置过长（>%d 字符）", in.Type, max)
	}
	return nil
}

// Create 创建。
func (o *FileStorageOper) Create(in FileStorageInput) (*models.FileStorage, error) {
	if err := validateTypeFields(in); err != nil {
		return nil, err
	}
	if err := validateForType(in.Type, in.URL); err != nil {
		return nil, err
	}
	if err := validatePasswordLength(in); err != nil {
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
	if err := validatePasswordLength(in); err != nil {
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

// validateTypeFields 按存储类型校验必填字段：agent 必须关联 agent_id；webdav/alist/smb 必须填地址；local 必须填本机根目录；
// rclone 必须填 remote 名（Username 字段承载）。
func validateTypeFields(in FileStorageInput) error {
	switch in.Type {
	case "agent":
		if in.AgentID == 0 {
			return invalidInput("agent 存储必须关联一个 agent")
		}
	case "webdav", "alist", "smb":
		if strings.TrimSpace(in.URL) == "" {
			return invalidInput("%s 存储必须填写地址", in.Type)
		}
	case "local":
		if strings.TrimSpace(in.BasePath) == "" {
			return invalidInput("本机存储必须填写根目录")
		}
	case "115", "quark", "123":
		// 网盘后端（只读浏览）：凭据存 password，浏览根存 basePath；均可在创建后补填/测试。
	case "rclone":
		// 与 NewRclone 构造校验对齐：Username=remote 名（必填，不含 / \ 或冒号）；
		// URL=rclone 二进制路径（可空=用 PATH）、password=rclone.conf INI 原文、BasePath=remote 内根子路径，均可选。
		remote := strings.TrimSpace(strings.TrimRight(in.Username, ":"))
		if remote == "" {
			return invalidInput("rclone 存储必须填写 remote 名")
		}
		if strings.ContainsAny(remote, "/\\:") {
			return invalidInput("rclone remote 名不能包含 / \\ 或冒号: %s", remote)
		}
	default:
		return invalidInput("未知存储类型：%s", in.Type)
	}
	return nil
}
