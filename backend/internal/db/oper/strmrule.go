package oper

import (
	"errors"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// StrmRuleOper 封装 StrmRule（STRM 生成规则）与 StrmRecord（已生成记录，去重+展示）的 CRUD。
// 规则无秘密字段，纯结构化配置；记录表 DriveID+CloudPath 唯一索引兜底去重（追更不重生成）。
type StrmRuleOper struct {
	db *gorm.DB
}

func NewStrmRuleOper(db *gorm.DB) *StrmRuleOper {
	return &StrmRuleOper{db: db}
}

// StrmRuleInput 创建/更新规则入参。
type StrmRuleInput struct {
	Name      string `json:"name" binding:"max=64"`
	DriveID   uint   `json:"drive_id"`
	CloudPath string `json:"cloud_path" binding:"max=255"`
	OutputDir string `json:"output_dir" binding:"max=255"`
	MountRoot string `json:"mount_root" binding:"max=255"`
	Template  string `json:"template" binding:"max=255"`
	Recursive *bool  `json:"recursive"` // 指针：空=不改（更新时保留），显式 false=关闭递归
	KeepExt   *bool  `json:"keep_ext"`
	Enabled   *bool  `json:"enabled"`
}

func (o *StrmRuleOper) Create(in StrmRuleInput) (*models.StrmRule, error) {
	if in.Name == "" || in.DriveID == 0 || in.CloudPath == "" || in.OutputDir == "" {
		return nil, errors.New("name/drive_id/cloud_path/output_dir 必填")
	}
	m := &models.StrmRule{
		Name: in.Name, DriveID: in.DriveID, CloudPath: in.CloudPath, OutputDir: in.OutputDir,
		MountRoot: in.MountRoot, Template: in.Template,
		Recursive: derefBool(in.Recursive, true), KeepExt: derefBool(in.KeepExt, true), Enabled: derefBool(in.Enabled, true),
	}
	if err := o.db.Create(m).Error; err != nil {
		return nil, err
	}
	return m, nil
}

func (o *StrmRuleOper) Update(id uint, in StrmRuleInput) (*models.StrmRule, error) {
	m, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		m.Name = in.Name
	}
	if in.DriveID != 0 {
		m.DriveID = in.DriveID
	}
	if in.CloudPath != "" {
		m.CloudPath = in.CloudPath
	}
	if in.OutputDir != "" {
		m.OutputDir = in.OutputDir
	}
	if in.MountRoot != "" {
		m.MountRoot = in.MountRoot
	}
	if in.Template != "" {
		m.Template = in.Template
	}
	if in.Recursive != nil {
		m.Recursive = *in.Recursive
	}
	if in.KeepExt != nil {
		m.KeepExt = *in.KeepExt
	}
	if in.Enabled != nil {
		m.Enabled = *in.Enabled
	}
	if err := o.db.Save(m).Error; err != nil {
		return nil, err
	}
	return m, nil
}

func (o *StrmRuleOper) List() ([]models.StrmRule, error) {
	var rs []models.StrmRule
	if err := o.db.Order("id asc").Find(&rs).Error; err != nil {
		return nil, err
	}
	return rs, nil
}

// ListEnabled 列出启用规则（GenerateAll 遍历）。
func (o *StrmRuleOper) ListEnabled() ([]models.StrmRule, error) {
	var rs []models.StrmRule
	if err := o.db.Where("enabled = ?", true).Order("id asc").Find(&rs).Error; err != nil {
		return nil, err
	}
	return rs, nil
}

func (o *StrmRuleOper) Get(id uint) (*models.StrmRule, error) {
	var m models.StrmRule
	if err := o.db.First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (o *StrmRuleOper) Delete(id uint) error {
	return o.db.Delete(&models.StrmRule{}, id).Error
}

// ---- StrmRecord ----

// UpsertRecord 按 DriveID+CloudPath 唯一索引 upsert（存在则更新 strm 路径/大小，不存在则插入）。
// 返回是否新建（true=本次新生成；false=已存在跳过），供生成器统计 Generated/Skipped。
func (o *StrmRuleOper) UpsertRecord(driveID uint, cloudPath, strmPath string, size int64, subID uint) (bool, error) {
	var rec models.StrmRecord
	err := o.db.Where("drive_id = ? AND cloud_path = ?", driveID, cloudPath).First(&rec).Error
	if err == nil {
		rec.StrmPath = strmPath
		rec.Size = size
		if subID > 0 {
			rec.SubscriptionID = subID
		}
		return false, o.db.Save(&rec).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	rec = models.StrmRecord{
		DriveID: driveID, CloudPath: cloudPath, StrmPath: strmPath, Size: size, SubscriptionID: subID,
	}
	if err := o.db.Create(&rec).Error; err != nil {
		// 并发下唯一索引可能竞态：再查一次兜底为「已存在」。
		return false, o.db.Where("drive_id = ? AND cloud_path = ?", driveID, cloudPath).
			First(&rec).Error
	}
	return true, nil
}

func (o *StrmRuleOper) ListRecords(driveID uint) ([]models.StrmRecord, error) {
	var rs []models.StrmRecord
	q := o.db.Order("id desc")
	if driveID > 0 {
		q = q.Where("drive_id = ?", driveID)
	}
	if err := q.Find(&rs).Error; err != nil {
		return nil, err
	}
	return rs, nil
}

// DeleteRecordsByDrive 删除某网盘全部记录（删网盘时级联）。
func (o *StrmRuleOper) DeleteRecordsByDrive(driveID uint) error {
	return o.db.Where("drive_id = ?", driveID).Delete(&models.StrmRecord{}).Error
}

// DeleteRulesByDrive 删除某网盘全部规则（删网盘时级联）。
func (o *StrmRuleOper) DeleteRulesByDrive(driveID uint) error {
	return o.db.Where("drive_id = ?", driveID).Delete(&models.StrmRule{}).Error
}

// LinkRecord 给已存在的 STRM 记录标注订阅来源（追更匹配命中时调用）。
func (o *StrmRuleOper) LinkRecord(id, subID uint) error {
	return o.db.Model(&models.StrmRecord{}).Where("id = ?", id).Update("subscription_id", subID).Error
}

func (o *StrmRuleOper) DeleteRecord(id uint) error {
	return o.db.Delete(&models.StrmRecord{}, id).Error
}

func derefBool(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}
