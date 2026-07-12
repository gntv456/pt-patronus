package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ContractOper 持久化保种契约规则。
type ContractOper struct{ db *gorm.DB }

func NewContractOper(db *gorm.DB) *ContractOper { return &ContractOper{db: db} }

func (o *ContractOper) List() ([]models.ContractRule, error) {
	var rs []models.ContractRule
	err := o.db.Order("id DESC").Find(&rs).Error
	return rs, err
}

func (o *ContractOper) ListEnabled() ([]models.ContractRule, error) {
	var rs []models.ContractRule
	err := o.db.Where("enabled = ?", true).Find(&rs).Error
	return rs, err
}

func (o *ContractOper) Get(id uint) (*models.ContractRule, error) {
	var r models.ContractRule
	if err := o.db.First(&r, id).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// Upsert 按 site_id（uniqueIndex）新增或更新一条契约。
func (o *ContractOper) Upsert(r *models.ContractRule) error {
	r.UpdatedAt = time.Now()
	return o.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "site_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "min_ratio", "min_seeding", "min_upload_gb", "updated_at"}),
	}).Create(r).Error
}

func (o *ContractOper) Delete(id uint) error {
	return o.db.Delete(&models.ContractRule{}, id).Error
}

// MarkChecked 记录最近一次检查结果。
func (o *ContractOper) MarkChecked(id uint, status, detail string) error {
	now := time.Now()
	return o.db.Model(&models.ContractRule{}).Where("id = ?", id).Updates(map[string]any{
		"last_checked_at": &now, "last_status": status, "last_detail": detail,
	}).Error
}
