package oper

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

type OpsConfigOper struct {
	db *gorm.DB
}

func NewOpsConfigOper(db *gorm.DB) *OpsConfigOper {
	return &OpsConfigOper{db: db}
}

type OpsConfigInput struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Env    string `json:"env"`
	Format string `json:"format"`
	Value  string `json:"value"`
	Tags   string `json:"tags"`
	Note   string `json:"note"`
}

func (o *OpsConfigOper) List() ([]models.OpsConfig, error) {
	var items []models.OpsConfig
	err := o.db.Order("env asc, key asc").Find(&items).Error
	return items, err
}

func (o *OpsConfigOper) Get(id uint) (*models.OpsConfig, error) {
	var item models.OpsConfig
	if err := o.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (o *OpsConfigOper) Create(in OpsConfigInput) (*models.OpsConfig, error) {
	if err := validateOpsConfigInput(in); err != nil {
		return nil, err
	}
	item := &models.OpsConfig{
		Key: strings.TrimSpace(in.Key), Name: strings.TrimSpace(in.Name), Env: strings.TrimSpace(in.Env),
		Format: normOpsConfigFormat(in.Format), Value: in.Value, Tags: strings.TrimSpace(in.Tags),
		Note: in.Note, Version: 1,
	}
	err := o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		return createOpsConfigHistory(tx, item)
	})
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (o *OpsConfigOper) Update(id uint, in OpsConfigInput) (*models.OpsConfig, error) {
	if err := validateOpsConfigInput(in); err != nil {
		return nil, err
	}
	item, err := o.Get(id)
	if err != nil {
		return nil, err
	}
	item.Key = strings.TrimSpace(in.Key)
	item.Name = strings.TrimSpace(in.Name)
	item.Env = strings.TrimSpace(in.Env)
	item.Format = normOpsConfigFormat(in.Format)
	item.Value = in.Value
	item.Tags = strings.TrimSpace(in.Tags)
	item.Note = in.Note
	item.Version++
	err = o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(item).Error; err != nil {
			return err
		}
		return createOpsConfigHistory(tx, item)
	})
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (o *OpsConfigOper) Delete(id uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&models.OpsConfig{}, id).Error; err != nil {
			return err
		}
		return tx.Where("config_id = ?", id).Delete(&models.OpsConfigHistory{}).Error
	})
}

func (o *OpsConfigOper) History(id uint) ([]models.OpsConfigHistory, error) {
	var items []models.OpsConfigHistory
	err := o.db.Where("config_id = ?", id).Order("version desc").Limit(100).Find(&items).Error
	return items, err
}

func createOpsConfigHistory(tx *gorm.DB, item *models.OpsConfig) error {
	return tx.Create(&models.OpsConfigHistory{
		ConfigID: item.ID, Key: item.Key, Version: item.Version, Format: item.Format,
		Value: item.Value, Note: item.Note,
	}).Error
}

func validateOpsConfigInput(in OpsConfigInput) error {
	if strings.TrimSpace(in.Key) == "" {
		return fmt.Errorf("配置 Key 不能为空")
	}
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("配置名称不能为空")
	}
	format := normOpsConfigFormat(in.Format)
	switch format {
	case "kv", "json":
		if strings.TrimSpace(in.Value) == "" {
			return nil
		}
		var v any
		if err := json.Unmarshal([]byte(in.Value), &v); err != nil {
			return fmt.Errorf("%s 格式必须是合法 JSON: %w", format, err)
		}
		if format == "kv" {
			if _, ok := v.(map[string]any); !ok {
				return fmt.Errorf("KV 格式必须是 JSON 对象")
			}
		}
	case "text":
	default:
		return fmt.Errorf("不支持的配置格式：%s", in.Format)
	}
	return nil
}

func normOpsConfigFormat(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "kv", "json", "text":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "text"
	}
}
