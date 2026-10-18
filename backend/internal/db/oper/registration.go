package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// RegistrationOper 封装 RegistrationWatch 表的 CRUD 与状态流转。
type RegistrationOper struct {
	db *gorm.DB
}

// NewRegistrationOper 构造。
func NewRegistrationOper(db *gorm.DB) *RegistrationOper {
	return &RegistrationOper{db: db}
}

// RegistrationInput 是创建/更新监控目标的入参。
type RegistrationInput struct {
	Name       string `json:"name" binding:"max=64"`
	URL        string `json:"url" binding:"max=255"`
	SignupPath string `json:"signup_path" binding:"max=128"`
	OpenText   string `json:"open_text" binding:"max=255"`
	ClosedText string `json:"closed_text" binding:"max=2000"`
	IsActive   bool   `json:"is_active"`
}

// Create 创建监控目标。
func (o *RegistrationOper) Create(in RegistrationInput) (*models.RegistrationWatch, error) {
	w := &models.RegistrationWatch{
		Name: in.Name, URL: in.URL, SignupPath: in.SignupPath,
		OpenText: in.OpenText, ClosedText: in.ClosedText, IsActive: in.IsActive,
	}
	if w.SignupPath == "" {
		w.SignupPath = "signup.php"
	}
	if err := o.db.Create(w).Error; err != nil {
		return nil, err
	}
	// GORM default:true 会把 IsActive=false 当零值吞为 true → 显式写回（同 SiteOper.Create）。
	if !in.IsActive {
		if err := o.db.Model(w).Update("is_active", false).Error; err != nil {
			return nil, err
		}
	}
	return w, nil
}

// Update 更新监控目标。
func (o *RegistrationOper) Update(id uint, in RegistrationInput) (*models.RegistrationWatch, error) {
	w, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	w.Name = in.Name
	w.URL = in.URL
	w.SignupPath = in.SignupPath
	w.OpenText = in.OpenText
	w.ClosedText = in.ClosedText
	w.IsActive = in.IsActive
	if w.SignupPath == "" {
		w.SignupPath = "signup.php"
	}
	if err := o.db.Save(w).Error; err != nil {
		return nil, err
	}
	return w, nil
}

// List 列出全部监控目标。
func (o *RegistrationOper) List() ([]models.RegistrationWatch, error) {
	var ws []models.RegistrationWatch
	if err := o.db.Order("id asc").Find(&ws).Error; err != nil {
		return nil, err
	}
	return ws, nil
}

// ListActive 列出启用的监控目标。
func (o *RegistrationOper) ListActive() ([]models.RegistrationWatch, error) {
	var ws []models.RegistrationWatch
	if err := o.db.Where("is_active = ?", true).Order("id asc").Find(&ws).Error; err != nil {
		return nil, err
	}
	return ws, nil
}

// Get 取单个监控目标。
func (o *RegistrationOper) Get(id uint) (*models.RegistrationWatch, error) {
	var w models.RegistrationWatch
	if err := o.db.First(&w, id).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

// Delete 删除监控目标。
func (o *RegistrationOper) Delete(id uint) error {
	return o.db.Delete(&models.RegistrationWatch{}, id).Error
}

// UpdateStatus 记录最新检测结果与时间戳。返回旧状态便于判定转换（nil=从未检测）。
func (o *RegistrationOper) UpdateStatus(id uint, open bool) (*bool, error) {
	w, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	prev := w.LastOpen
	now := time.Now()
	w.LastOpen = &open
	w.LastCheckedAt = &now
	if err := o.db.Save(w).Error; err != nil {
		return nil, err
	}
	return prev, nil
}

// TouchChecked 只刷新「最近查过」时间戳，保留 last_open。
// 用于本轮无法判定（被拦截 / 页面认不出标记）的情况：状态不动，就不会把已开放的站写成
// 关闭、下一轮读通又重复提醒。
func (o *RegistrationOper) TouchChecked(id uint) error {
	return o.db.Model(&models.RegistrationWatch{}).
		Where("id = ?", id).
		Update("last_checked_at", time.Now()).Error
}
