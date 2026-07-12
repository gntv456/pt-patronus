package oper

import (
	"fmt"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// MediaServerOper 封装 MediaServer 表的 CRUD 与凭据（API Key / X-Plex-Token）加解密。
// 与 DownloaderOper 同构：Token 明文入参，由 Oper 加密落库；空串=保留旧值（UI 不重输秘密时回填）。
type MediaServerOper struct {
	db *gorm.DB
	c  *security.Crypto
}

func NewMediaServerOper(db *gorm.DB, c *security.Crypto) *MediaServerOper {
	return &MediaServerOper{db: db, c: c}
}

// MediaServerInput 创建/更新媒体服务器入参（Token 明文，由 Oper 加密）。
type MediaServerInput struct {
	Name              string `json:"name" binding:"max=64"`
	Type              string `json:"type" binding:"max=16"` // emby | jellyfin | plex
	URL               string `json:"url" binding:"max=255"`
	Username          string `json:"username" binding:"max=64"`
	Token             string `json:"token" binding:"max=512"` // API Key(Emby/JF) 或 X-Plex-Token
	IsDefault         bool   `json:"is_default"`
	RefreshOnDownload bool   `json:"refresh_on_download"`
	NotifyOnPlay      bool   `json:"notify_on_play"`
	DirectLinkEnabled bool   `json:"direct_link_enabled"`
	DirectLinkBase    string `json:"direct_link_base" binding:"max=255"`
}

// validMediaServerType 收紧服务端类型白名单（inspectReq 复查 + binding max 不够）。
func validMediaServerType(t string) bool {
	switch t {
	case "emby", "jellyfin", "plex":
		return true
	}
	return false
}

func (o *MediaServerOper) Create(in MediaServerInput) (*models.MediaServer, error) {
	if !validMediaServerType(in.Type) {
		return nil, errInvalidServerType
	}
	if err := validateURLScheme(in.URL); err != nil {
		return nil, err
	}
	m := &models.MediaServer{
		Name: in.Name, Type: in.Type, URL: in.URL, Username: in.Username,
		IsDefault: in.IsDefault, RefreshOnDownload: in.RefreshOnDownload, NotifyOnPlay: in.NotifyOnPlay,
		DirectLinkEnabled: in.DirectLinkEnabled, DirectLinkBase: in.DirectLinkBase,
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

// Update 更新媒体服务器（Token 空串=保留旧值）。
func (o *MediaServerOper) Update(id uint, in MediaServerInput) (*models.MediaServer, error) {
	if !validMediaServerType(in.Type) {
		return nil, errInvalidServerType
	}
	if err := validateURLScheme(in.URL); err != nil {
		return nil, err
	}
	m, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	m.Name, m.Type, m.URL, m.Username = in.Name, in.Type, in.URL, in.Username
	m.IsDefault, m.RefreshOnDownload, m.NotifyOnPlay = in.IsDefault, in.RefreshOnDownload, in.NotifyOnPlay
	m.DirectLinkEnabled, m.DirectLinkBase = in.DirectLinkEnabled, in.DirectLinkBase
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

func (o *MediaServerOper) List() ([]models.MediaServer, error) {
	var ms []models.MediaServer
	if err := o.db.Order("id asc").Find(&ms).Error; err != nil {
		return nil, err
	}
	return ms, nil
}

func (o *MediaServerOper) Get(id uint) (*models.MediaServer, error) {
	var m models.MediaServer
	if err := o.db.First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// ListRefreshOnDownload 列出开启「下载完成刷新」的服务器（下载完成事件遍历刷新）。
func (o *MediaServerOper) ListRefreshOnDownload() ([]models.MediaServer, error) {
	var ms []models.MediaServer
	if err := o.db.Where("refresh_on_download = ?", true).Find(&ms).Error; err != nil {
		return nil, err
	}
	return ms, nil
}

func (o *MediaServerOper) Delete(id uint) error {
	return o.db.Delete(&models.MediaServer{}, id).Error
}

// Token 解密返回凭据。
func (o *MediaServerOper) Token(m *models.MediaServer) (string, error) {
	return o.c.Decrypt(m.TokenEnc)
}

// errInvalidServerType 媒体服务器类型不在 emby/jellyfin/plex 白名单内（包 ErrInvalidInput 供 handler errors.Is 判 400）。
var errInvalidServerType = fmt.Errorf("无效的媒体服务器类型（仅支持 emby/jellyfin/plex）: %w", ErrInvalidInput)
