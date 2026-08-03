package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func newContractDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/contract.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ContractRule{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// ContractRule.Enabled 带 gorm:"default:true"。Upsert 新建 Enabled=false 的规则，
// DB 必须存 false（修复后回写），否则会被吞为 true，导致禁用的契约仍被定时检查执行。
func TestContractUpsertEnabledFalseNotSwallowed(t *testing.T) {
	op := &ContractOper{db: newContractDB(t)}

	if err := op.Upsert(&models.ContractRule{SiteID: 1, Enabled: false, MinRatio: 1}); err != nil {
		t.Fatal(err)
	}
	var off models.ContractRule
	if err := op.db.Where("site_id = ?", 1).First(&off).Error; err != nil {
		t.Fatal(err)
	}
	if off.Enabled {
		t.Errorf("Enabled=true：被 gorm default:true 吞了，应保持 false（禁用契约会被错误执行）")
	}

	// 更新分支：先建 true，再 upsert false
	op.Upsert(&models.ContractRule{SiteID: 2, Enabled: true, MinRatio: 1})
	if err := op.Upsert(&models.ContractRule{SiteID: 2, Enabled: false, MinRatio: 2}); err != nil {
		t.Fatal(err)
	}
	var off2 models.ContractRule
	op.db.Where("site_id = ?", 2).First(&off2)
	if off2.Enabled {
		t.Errorf("更新分支 Enabled=true：OnConflict DoUpdates 应把 enabled 写为 false")
	}

	enabled, _ := op.ListEnabled()
	for _, e := range enabled {
		if e.SiteID == 1 || e.SiteID == 2 {
			t.Errorf("禁用契约出现在 ListEnabled 里")
		}
	}
}
