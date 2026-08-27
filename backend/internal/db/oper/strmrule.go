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
	Name            string `json:"name" binding:"max=64"`
	DriveID         uint   `json:"drive_id"`
	CloudPath       string `json:"cloud_path" binding:"max=255"`
	OutputDir       string `json:"output_dir" binding:"max=255"`
	MountRoot       string `json:"mount_root" binding:"max=255"`
	Template        string `json:"template" binding:"max=255"`
	Recursive       *bool  `json:"recursive"` // 指针：空=不改（更新时保留），显式 false=关闭递归
	KeepExt         *bool  `json:"keep_ext"`
	Incremental     *bool  `json:"incremental"` // 增量生成：本地 .strm 已存在即跳过（大库定期生成）
	Enabled         *bool  `json:"enabled"`
	OutputTag       string `json:"output_tag" binding:"max=64"`        // 输出目录绑定目录标签（优先于 OutputDir）
	IncludeExts     string `json:"include_exts" binding:"max=255"`     // 扩展名白名单（逗号分隔；空=全部媒体）
	ExcludeKeywords string `json:"exclude_keywords" binding:"max=512"` // 文件名排除关键词（逗号分隔）
	LocalBaseURL    string `json:"local_base_url" binding:"max=255"`   // 内网基址（{direct} 渲染优先用）
	PublicBaseURL   string `json:"public_base_url" binding:"max=255"`  // 外网基址（{directPublic} 渲染用）
}

func (o *StrmRuleOper) Create(in StrmRuleInput) (*models.StrmRule, error) {
	if in.Name == "" || in.DriveID == 0 || in.CloudPath == "" || in.OutputDir == "" {
		return nil, errors.New("name/drive_id/cloud_path/output_dir 必填")
	}
	// Recursive/KeepExt/Enabled 均带 gorm:"default:true"；GORM 对带 default 标签的零值 bool(false)
	// 即使 Select 强制列名，仍会用 DB 默认 true 覆盖（RETURNING 回填）——实测 struct+Create 无法写入 false。
	// 用 map[string]any 绕开 default 机制，确保显式 false 正确落库（对标 keepseed.SaveSetting 的同款修复）。
	row := map[string]any{
		"name": in.Name, "drive_id": in.DriveID, "cloud_path": in.CloudPath, "output_dir": in.OutputDir,
		"mount_root": in.MountRoot, "template": in.Template,
		"recursive": derefBool(in.Recursive, true), "keep_ext": derefBool(in.KeepExt, true), "incremental": derefBool(in.Incremental, false), "enabled": derefBool(in.Enabled, true),
		"output_tag": in.OutputTag, "include_exts": in.IncludeExts, "exclude_keywords": in.ExcludeKeywords,
		"local_base_url": in.LocalBaseURL, "public_base_url": in.PublicBaseURL,
	}
	if err := o.db.Model(&models.StrmRule{}).Create(row).Error; err != nil {
		return nil, err
	}
	// glebarez/sqlite（pure-Go）回填到 map 的 id 类型不固定（int/int64/float64 均见过），
	// 全覆盖断言；取不到 id 时用 LastInsertID 兜底。
	var id uint
	switch v := row["id"].(type) {
	case int:
		id = uint(v)
	case int64:
		id = uint(v)
	case float64:
		id = uint(v)
	}
	if id == 0 {
		var last models.StrmRule
		if err := o.db.Order("id DESC").First(&last).Error; err == nil {
			id = last.ID
		}
	}
	return o.Get(id)
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
	if in.Incremental != nil {
		m.Incremental = *in.Incremental
	}
	if in.Enabled != nil {
		m.Enabled = *in.Enabled
	}
	// 字符串字段允许显式清空（除唯一键字段外用指针语义太啰嗦，这里以「入参非 nil 即覆盖」
	// 不适用——binding 零值无法区分未传/清空，采用与旧字段一致的「非空才改」，标签/过滤的
	// 清空通过传空格 hack 不可取，故增 tag:Update 总是覆盖这几个新字段（传 "" 即清空）。
	m.OutputTag = in.OutputTag
	m.IncludeExts = in.IncludeExts
	m.ExcludeKeywords = in.ExcludeKeywords
	m.LocalBaseURL = in.LocalBaseURL
	m.PublicBaseURL = in.PublicBaseURL
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

// GetRecordByCloudPath 按 DriveID+CloudPath 精确取单条记录（302 直链端点用）。
func (o *StrmRuleOper) GetRecordByCloudPath(driveID uint, cloudPath string) (*models.StrmRecord, error) {
	var rec models.StrmRecord
	if err := o.db.Where("drive_id = ? AND cloud_path = ?", driveID, cloudPath).
		First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
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
